package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/metrics"
)

var (
	kubeClientRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "semaphore_wg_kube_http_request_total",
		Help: "Total number of HTTP requests to the Kubernetes API by host, code and method",
	},
		[]string{"host", "code", "method"},
	)
)

func init() {
	(&kubeClientRequestAdapter{}).Register()
}

// kubeClientRequestAdapter implements metrics interfaces provided by client-go
type kubeClientRequestAdapter struct{}

// Register registers the adapter
func (a *kubeClientRequestAdapter) Register() {
	metrics.Register(
		metrics.RegisterOpts{
			RequestResult: a,
		},
	)
	prometheus.MustRegister(
		kubeClientRequests,
	)

}

// Increment implements metrics.ResultMetric
func (a kubeClientRequestAdapter) Increment(ctx context.Context, code string, method string, host string) {
	kubeClientRequests.With(prometheus.Labels{
		"code":   code,
		"method": method,
		"host":   host,
	}).Inc()
}
