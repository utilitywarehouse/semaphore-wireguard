package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/utilitywarehouse/kube-wiresteward/kube"
	"github.com/utilitywarehouse/kube-wiresteward/log"
	"k8s.io/client-go/kubernetes"
)

const (
	annotationWGPublicKey = "wiresteward.uw.io/pubKey"
	annotationWGEndpoint  = "wiresteward.uw.io/endpoint"
)

var (
	flagKubeConfigPath       = flag.String("local-kube-config", getEnv("WS_LOCAL_KUBE_CONFIG", ""), "Path of the local kube cluster config file, if not provided the app will try to get in cluster config")
	flagTargetKubeConfigPath = flag.String("target-kube-config", getEnv("WS_TARGET_KUBE_CONFIG", ""), "(Required) Path of the target cluster kube config file to add wg peers from")
	flagLogLevel             = flag.String("log-level", getEnv("WS_LOG_LEVEL", "info"), "Log level")
	flagRemoteAPIURL         = flag.String("remote-api-url", getEnv("WS_REMOTE_API_URL", ""), "Remote Kubernetes API server URL")
	flagRemoteCAURL          = flag.String("remote-ca-url", getEnv("WS_REMOTE_CA_URL", ""), "Remote Kubernetes CA certificate URL")
	flagRemoteSATokenPath    = flag.String("remote-sa-token-path", getEnv("WS_REMOTE_SERVICE_ACCOUNT_TOKEN_PATH", ""), "Remote Kubernetes cluster token path")
	flagWGDeviceName         = flag.String("wg-device-name", getEnv("WS_WG_DEVICE_NAME", ""), "(Required) The name of the wireguard device to be created")
	flagWSNodeName           = flag.String("ws-node-name", getEnv("WS_NODE_NAME", ""), "(Required) The node on which wiresteward is running")
	flagWGKeyPath            = flag.String("wg-key-path", getEnv("WS_WG_KEY_PATH", "/var/lib/wiresteward"), "Path to store and look for wg private key")
	flagWGDeviceMTU          = flag.String("wg-device-mtu", getEnv("WS_WG_DEVICE_MTU", "1420"), "MTU for wg device")
	flagWGListenPort         = flag.String("wg-listen-port", getEnv("WS_WG_LISTEN_PORT", "51820"), "WG listen port")
	flagRemotePodSubnet      = flag.String("remote-pod-subnet", getEnv("WS_REMOTE_POD_SUBNET", ""), "Subnet to route via the created wg interface")
	flagResyncPeriod         = flag.Duration("resync-period", 60*time.Minute, "Node watcher cache resync period")

	saToken  = os.Getenv("WS_REMOTE_SERVICE_ACCOUNT_TOKEN")
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
	if *flagWGDeviceName == "" {
		log.Logger.Error("Must specify a name for the wg device")
		usage()
	}
	if *flagWSNodeName == "" {
		log.Logger.Error("Must specify the kube node that wiresteward runs on")
		usage()
	}
	if *flagRemotePodSubnet == "" {
		log.Logger.Error("Must specify remote cluster's pod subnet")
		usage()
	}
	_, podSubnet, err := net.ParseCIDR(*flagRemotePodSubnet)
	if err != nil {
		log.Logger.Error("Cannot parse remote pod subnet", "err", err)
		os.Exit(1)
	}
	wgDeviceMTU, err := strconv.Atoi(*flagWGDeviceMTU)
	if err != nil {
		log.Logger.Error("Cannot parse mtu flag to int", "mtu", *flagWGDeviceMTU, "err", err)
		usage()
	}
	wgListenPort, err := strconv.Atoi(*flagWGListenPort)
	if err != nil {
		log.Logger.Error("Cannot parse listen port flag to int", "listen port", *flagWGListenPort, "err", err)
		usage()
	}
	if *flagRemoteSATokenPath != "" {
		data, err := os.ReadFile(*flagRemoteSATokenPath)
		if err != nil {
			fmt.Printf("Cannot read file: %s", *flagRemoteSATokenPath)
			os.Exit(1)
		}
		saToken = string(data)
	}

	if saToken != "" {
		saToken = strings.TrimSpace(saToken)
		if !bearerRe.Match([]byte(saToken)) {
			log.Logger.Error(
				"The provided token does not match regex",
				"regex", bearerRe.String)
			os.Exit(1)
		}
	}

	// Get a kube client to use with the watchers
	homeClient, err := kube.ClientFromConfig(*flagKubeConfigPath)
	if err != nil {
		log.Logger.Error(
			"cannot create kube client for homecluster",
			"err", err,
		)
		usage()
	}

	var remoteClient *kubernetes.Clientset
	if *flagTargetKubeConfigPath != "" {
		remoteClient, err = kube.ClientFromConfig(*flagTargetKubeConfigPath)
	} else {
		remoteClient, err = kube.Client(saToken, *flagRemoteAPIURL, *flagRemoteCAURL)
	}
	if err != nil {
		log.Logger.Error(
			"cannot create kube client for remotecluster",
			"err", err,
		)
		usage()
	}

	r := newRunner(
		homeClient,
		remoteClient,
		*flagWSNodeName,
		*flagWGDeviceName,
		fmt.Sprintf("%s/%s.key", *flagWGKeyPath, *flagWGDeviceName),
		wgDeviceMTU,
		wgListenPort,
		podSubnet,
		*flagResyncPeriod,
	)
	if err := r.Run(); err != nil {
		log.Logger.Error("Failed to start runner", "err", err)
		os.Exit(1)
	}

	sm := http.NewServeMux()
	sm.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if r.Healthy() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	log.Logger.Error(
		"Listen and Serve",
		"err", http.ListenAndServe(":51880", sm),
	)

}
