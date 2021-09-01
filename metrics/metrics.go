package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/utilitywarehouse/semaphore-wireguard/log"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var (
	syncPeersAttempt = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "semaphore_wg_sync_peers_total",
			Help: "Counts runners' attempts to sync peers.",
		},
		[]string{"device", "success"},
	)
	syncQueueFullFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "semaphore_wg_sync_queue_full_failures_total",
			Help: "Number of times a sync task was not added to the sync queue in time because the queue was full.",
		},
		[]string{"device"},
	)
	syncRequeue = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "semaphore_wg_sync_requeue_total",
			Help: "Number of attempts to requeue a sync.",
		},
		[]string{"device"},
	)
	nodeWatcherFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "semaphore_wg_node_watcher_failures_total",
			Help: "Number of times the node wathcer list/watch functions errored.",
		},
		[]string{"cluster", "type"},
	)
)

// Register registers all the prometheus collectors
func Register(wgMetricsClient *wgctrl.Client, wgDeviceNames, rClusterNames []string) {
	// Initialize counters with a value of 0
	for _, d := range wgDeviceNames {
		for _, s := range []string{"0", "1"} {
			syncPeersAttempt.With(prometheus.Labels{"device": d, "success": s})
		}
		syncQueueFullFailures.With(prometheus.Labels{"device": d})
		syncRequeue.With(prometheus.Labels{"device": d})
	}

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

	// Retrieving a Counter from a CounterVec will initialize it with a 0 value if it
	// doesn't already have a value. This ensures that all possible counters
	// start with a 0 value.
	for _, c := range rClusterNames {
		for _, t := range []string{"get", "list", "create", "update", "patch", "watch", "delete"} {
			nodeWatcherFailures.With(prometheus.Labels{"cluster": c, "type": t})
		}
	}

	prometheus.MustRegister(
		mc,
		syncPeersAttempt,
		syncQueueFullFailures,
		syncRequeue,
		nodeWatcherFailures,
	)
}

// A collector is a prometheus.Collector for a WireGuard device.
type collector struct {
	DeviceInfo         *prometheus.Desc
	PeerInfo           *prometheus.Desc
	PeerAllowedIPsInfo *prometheus.Desc
	PeerReceiveBytes   *prometheus.Desc
	PeerTransmitBytes  *prometheus.Desc
	PeerLastHandshake  *prometheus.Desc

	devices func() ([]*wgtypes.Device, error) // to allow testing
}

// newMetricsCollector constructs a prometheus.Collector to collect metrics for
// all present wg devices and correlate with user if possible
func newMetricsCollector(devices func() ([]*wgtypes.Device, error)) prometheus.Collector {
	// common labels for all metrics
	labels := []string{"device", "public_key"}

	return &collector{
		DeviceInfo: prometheus.NewDesc(
			"semaphore_wg_device_info",
			"Metadata about a device.",
			labels,
			nil,
		),
		PeerInfo: prometheus.NewDesc(
			"semaphore_wg_peer_info",
			"Metadata about a peer. The public_key label on peer metrics refers to the peer's public key; not the device's public key.",
			append(labels, []string{"endpoint"}...),
			nil,
		),
		PeerAllowedIPsInfo: prometheus.NewDesc(
			"semaphore_wg_peer_allowed_ips_info",
			"Metadata about each of a peer's allowed IP subnets for a given device.",
			append(labels, []string{"allowed_ips"}...),
			nil,
		),
		PeerReceiveBytes: prometheus.NewDesc(
			"semaphore_wg_peer_receive_bytes_total",
			"Number of bytes received from a given peer.",
			labels,
			nil,
		),
		PeerTransmitBytes: prometheus.NewDesc(
			"semaphore_wg_peer_transmit_bytes_total",
			"Number of bytes transmitted to a given peer.",
			labels,
			nil,
		),
		PeerLastHandshake: prometheus.NewDesc(
			"semaphore_wg_peer_last_handshake_seconds",
			"UNIX timestamp for the last handshake with a given peer.",
			labels,
			nil,
		),
		devices: devices,
	}
}

// Describe implements prometheus.Collector.
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ds := []*prometheus.Desc{
		c.DeviceInfo,
		c.PeerInfo,
		c.PeerAllowedIPsInfo,
		c.PeerReceiveBytes,
		c.PeerTransmitBytes,
		c.PeerLastHandshake,
	}

	for _, d := range ds {
		ch <- d
	}
}

// Collect implements prometheus.Collector.
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	devices, err := c.devices()
	if err != nil {
		log.Logger.Error("Failed to get wg device for metrics collection", "err", err)
		ch <- prometheus.NewInvalidMetric(c.DeviceInfo, err)
		return
	}

	for _, d := range devices {
		ch <- prometheus.MustNewConstMetric(
			c.DeviceInfo,
			prometheus.GaugeValue,
			1,
			d.Name, d.PublicKey.String(),
		)

		for _, p := range d.Peers {
			pub := p.PublicKey.String()
			// Use empty string instead of special Go <nil> syntax for no endpoint.
			var endpoint string
			if p.Endpoint != nil {
				endpoint = p.Endpoint.String()
			}

			ch <- prometheus.MustNewConstMetric(
				c.PeerInfo,
				prometheus.GaugeValue,
				1,
				d.Name, pub, endpoint,
			)

			for _, ip := range p.AllowedIPs {
				ch <- prometheus.MustNewConstMetric(
					c.PeerAllowedIPsInfo,
					prometheus.GaugeValue,
					1,
					d.Name, pub, ip.String(),
				)
			}

			ch <- prometheus.MustNewConstMetric(
				c.PeerReceiveBytes,
				prometheus.CounterValue,
				float64(p.ReceiveBytes),
				d.Name, pub,
			)

			ch <- prometheus.MustNewConstMetric(
				c.PeerTransmitBytes,
				prometheus.CounterValue,
				float64(p.TransmitBytes),
				d.Name, pub,
			)

			// Expose last handshake of 0 unless a last handshake time is set.
			var last float64
			if !p.LastHandshakeTime.IsZero() {
				last = float64(p.LastHandshakeTime.Unix())
			}

			ch <- prometheus.MustNewConstMetric(
				c.PeerLastHandshake,
				prometheus.GaugeValue,
				last,
				d.Name, pub,
			)
		}
	}
}

func SyncPeerAttempt(device string, err error) {
	s := "1"
	if err != nil {
		s = "0"
	}
	syncPeersAttempt.With(prometheus.Labels{
		"device":  device,
		"success": s,
	}).Inc()
}

func IncSyncQueueFullFailures(device string) {
	syncQueueFullFailures.With(prometheus.Labels{
		"device": device,
	}).Inc()
}

func IncSyncRequeue(device string) {
	syncRequeue.With(prometheus.Labels{
		"device": device,
	}).Inc()
}

func IncNodeWatcherFailures(c, t string) {
	nodeWatcherFailures.With(prometheus.Labels{
		"cluster": c,
		"type":    t,
	}).Inc()
}
