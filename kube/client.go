package kube

import (
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
)

type certMan struct {
	caURL string
}

func (cm *certMan) verifyConn(cs tls.ConnectionState) error {
	resp, err := http.Get(cm.caURL)
	if err != nil {
		return fmt.Errorf("error getting remote CA from %s: %v", cm.caURL, err)
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected %d response from %s, got %d", http.StatusOK, cm.caURL, resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body from %s: %v", cm.caURL, err)
	}
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(body)
	if !ok {
		return fmt.Errorf("failed to parse root certificate from %s", cm.caURL)
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
