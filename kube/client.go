package kube

import (
	"errors"
	"fmt"
	"io"

	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	// in case of local kube config
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"github.com/utilitywarehouse/semaphore-wireguard/log"
)

type certMan struct {
	caURL string
}

func (cm *certMan) verifyConn(cs tls.ConnectionState) error {
	resp, err := http.Get(cm.caURL)
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()
	if err != nil {
		log.Logger.Error(
			"error getting remote CA",
			"err", err)
		return err
	}
	body, err := ioutil.ReadAll(resp.Body)
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(body)
	if !ok {
		return errors.New("failed to parse root certificate")
	}
	opts := x509.VerifyOptions{
		DNSName: cs.ServerName,
		Roots:   roots,
	}
	_, err = cs.PeerCertificates[0].Verify(opts)
	return err
}

// Client returns a Kubernetes client (clientset) from token, apiURL and caURL
func Client(token, apiURL, caURL string) (*kubernetes.Clientset, error) {
	cm := &certMan{caURL}
	conf := &rest.Config{
		Host: apiURL,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				VerifyConnection:   cm.verifyConn}},
		BearerToken: token,
	}
	return kubernetes.NewForConfig(conf)
}

// ClientFromConfig returns a Kubernetes client (clientset) from the kubeconfig
// path or from the in-cluster service account environment.
func ClientFromConfig(path string) (*kubernetes.Clientset, error) {
	conf, err := getClientConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client config: %v", err)
	}
	return kubernetes.NewForConfig(conf)
}

// getClientConfig returns a Kubernetes client Config.
func getClientConfig(path string) (*rest.Config, error) {
	if path != "" {
		// build Config from a kubeconfig filepath
		return clientcmd.BuildConfigFromFlags("", path)
	}
	// uses pod's service account to get a Config
	return rest.InClusterConfig()
}
