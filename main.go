package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/utilitywarehouse/kube-wiresteward/kube"
	"github.com/utilitywarehouse/kube-wiresteward/log"
	"k8s.io/client-go/kubernetes"
)

const (
	watchAnnotationWGPublicKeyPattern = "wiresteward.%s.uw.io/pubKey"
	watchAnnotationWGEndpointPattern  = "wiresteward.%s.uw.io/endpoint"
	localAnnotationWGPublicKeyPattern = "wiresteward.%s.uw.io/pubKey"
	localAnnotationWGEndpointPattern  = "wiresteward.%s.uw.io/endpoint"
	wgDeviceNamePattern               = "wireguard.%s"
)

var (
	flagLogLevel         = flag.String("log-level", getEnv("WS_LOG_LEVEL", "info"), "Log level")
	flagWSNodeName       = flag.String("ws-node-name", getEnv("WS_NODE_NAME", ""), "(Required) The node on which wiresteward is running")
	flagWGKeyPath        = flag.String("wg-key-path", getEnv("WS_WG_KEY_PATH", "/var/lib/wiresteward"), "Path to store and look for wg private key")
	flagWSListenAddr     = flag.String("listen-address", getEnv("WS_LISTEN_ADDRESS", ":7773"), "Listen address to serve health and metrics")
	flagWSClustersConfig = flag.String("clusters-config", getEnv("WS_CLUSTERS_CONFIG", ""), "Path to the wiresteward clusters' json config file")

	bearerRe = regexp.MustCompile(`[A-Z|a-z0-9\-\._~\+\/]+=*`)
)

func usage() {
	flag.Usage()
	os.Exit(1)
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return defaultValue
	}
	return value
}

func main() {
	flag.Parse()
	log.InitLogger("kube-wiresteward", *flagLogLevel)

	if *flagWSNodeName == "" {
		log.Logger.Error("Must specify the kube node that wiresteward runs on")
		usage()
	}
	if *flagWSClustersConfig == "" {
		log.Logger.Error("Must specify a clusters config file location")
		usage()
	}
	fileContent, err := os.ReadFile(*flagWSClustersConfig)
	if err != nil {
		log.Logger.Error("Cannot read clusters config file", "err", err)
		os.Exit(1)
	}
	config, err := parseConfig(fileContent)
	if err != nil {
		log.Logger.Error("Cannot parse clusters config", "err", err)
		os.Exit(1)
	}

	homeClient, err := kube.ClientFromConfig(config.Local.KubeConfigPath)
	if err != nil {
		log.Logger.Error(
			"cannot create kube client for homecluster",
			"err", err,
		)
		os.Exit(1)
	}

	var runners []*Runner
	var wgDeviceNames []string
	for _, rConf := range config.Remotes {
		r, wgDeviceName, err := makeRunner(homeClient, config.Local.Name, rConf)
		if err != nil {
			log.Logger.Error("Failed to create runner", "err", err)
			os.Exit(1)
		}
		wgDeviceNames = append(wgDeviceNames, wgDeviceName)
		runners = append(runners, r)
		if err := r.Run(); err != nil {
			log.Logger.Error("Failed to start runner", "err", err)
			os.Exit(1)
		}
	}

	wgMetricsClient, err := wgctrl.New()
	if err != nil {
		log.Logger.Error("Failed to start wg client for metrics collection", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := wgMetricsClient.Close(); err != nil {
			log.Logger.Error("Failed to close metrics collection wg client", "err", err)
		}
	}()

	makeMetricsCollector(wgMetricsClient, wgDeviceNames)
	listenAndServe(runners)
}

func makeRunner(homeClient kubernetes.Interface, localName string, rConf *remoteClusterConfig) (*Runner, string, error) {
	data, err := os.ReadFile(rConf.RemoteSATokenPath)
	if err != nil {
		return nil, "", fmt.Errorf("Cannot read file: %s: %v", rConf.RemoteSATokenPath, err)
	}
	saToken := string(data)
	if saToken != "" {
		saToken = strings.TrimSpace(saToken)
		if !bearerRe.Match([]byte(saToken)) {
			return nil, "", fmt.Errorf("The provided token does not match regex: %s", bearerRe.String())
		}
	}
	var remoteClient *kubernetes.Clientset
	if rConf.KubeConfigPath != "" {
		remoteClient, err = kube.ClientFromConfig(rConf.KubeConfigPath)
	} else {
		remoteClient, err = kube.Client(saToken, rConf.RemoteAPIURL, rConf.RemoteCAURL)
	}
	if err != nil {
		return nil, "", fmt.Errorf("cannot create kube client for remotecluster %v", err)
	}
	_, podSubnet, err := net.ParseCIDR(rConf.PodSubnet)
	if err != nil {
		return nil, "", fmt.Errorf("Cannot parse remote pod subnet: %s", err)
	}
	wgDeviceName := fmt.Sprintf(wgDeviceNamePattern, rConf.Name)
	r := newRunner(
		homeClient,
		remoteClient,
		*flagWSNodeName,
		wgDeviceName,
		fmt.Sprintf("%s/%s.key", *flagWGKeyPath, wgDeviceName),
		localName,
		rConf.Name,
		rConf.WGDeviceMTU,
		rConf.WGListenPort,
		podSubnet,
		rConf.ResyncPeriod.Duration,
	)
	return r, wgDeviceName, nil
}

func makeMetricsCollector(wgMetricsClient *wgctrl.Client, wgDeviceNames []string) {
	mc := newMetricsCollector(func() ([]*wgtypes.Device, error) {
		var devices []*wgtypes.Device
		for _, name := range wgDeviceNames {
			device, err := wgMetricsClient.Device(name)
			if err != nil {
				return nil, err
			}
			devices = append(devices, device)
		}
		return devices, nil
	})
	prometheus.MustRegister(mc)

}

func listenAndServe(runners []*Runner) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		for _, r := range runners {
			if !r.Healthy() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	})
	server := http.Server{
		Addr:    *flagWSListenAddr,
		Handler: mux,
	}
	log.Logger.Error(
		"Listen and Serve",
		"err", server.ListenAndServe(),
	)
}
