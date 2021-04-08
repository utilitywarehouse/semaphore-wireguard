package main

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mdlayher/promtest"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestCollector(t *testing.T) {
	// Fake public keys used to identify devices and peers.
	var (
		pubDevA  = newWgKey()
		pubPeerA = newWgKey()
		pubPeerB = newWgKey()
	)

	tests := []struct {
		name    string
		devices func() ([]*wgtypes.Device, error)
		metrics []string
	}{
		{
			name: "ok",
			devices: func() ([]*wgtypes.Device, error) {
				return []*wgtypes.Device{
					&wgtypes.Device{
						Name:      "wg0",
						PublicKey: pubDevA,
						Peers: []wgtypes.Peer{{
							PublicKey: pubPeerA,
							Endpoint: &net.UDPAddr{
								IP:   net.ParseIP("1.1.1.1"),
								Port: 51820,
							},
							LastHandshakeTime: time.Unix(10, 0),
							ReceiveBytes:      1,
							TransmitBytes:     2,
							AllowedIPs: []net.IPNet{
								net.IPNet{
									IP:   net.ParseIP("10.0.0.1"),
									Mask: net.CIDRMask(32, 32),
								},
								net.IPNet{
									IP:   net.ParseIP("10.0.0.2"),
									Mask: net.CIDRMask(32, 32),
								},
							}},
							{
								PublicKey: pubPeerB,
								AllowedIPs: []net.IPNet{
									net.IPNet{
										IP:   net.ParseIP("10.0.0.3"),
										Mask: net.CIDRMask(32, 32),
									},
								},
							},
						},
					}}, nil
			},
			metrics: []string{
				fmt.Sprintf(`wiresteward_wg_device_info{device="wg0",public_key="%v"} 1`, pubDevA.String()),
				fmt.Sprintf(`wiresteward_wg_peer_info{device="wg0",endpoint="1.1.1.1:51820",public_key="%v"} 1`, pubPeerA.String()),
				fmt.Sprintf(`wiresteward_wg_peer_info{device="wg0",endpoint="",public_key="%v"} 1`, pubPeerB.String()),
				fmt.Sprintf(`wiresteward_wg_peer_allowed_ips_info{allowed_ips="10.0.0.1/32",device="wg0",public_key="%v"} 1`, pubPeerA.String()),
				fmt.Sprintf(`wiresteward_wg_peer_allowed_ips_info{allowed_ips="10.0.0.2/32",device="wg0",public_key="%v"} 1`, pubPeerA.String()),
				fmt.Sprintf(`wiresteward_wg_peer_allowed_ips_info{allowed_ips="10.0.0.3/32",device="wg0",public_key="%v"} 1`, pubPeerB.String()),
				fmt.Sprintf(`wiresteward_wg_peer_last_handshake_seconds{device="wg0",public_key="%v"} 10`, pubPeerA.String()),
				fmt.Sprintf(`wiresteward_wg_peer_last_handshake_seconds{device="wg0",public_key="%v"} 0`, pubPeerB.String()),
				fmt.Sprintf(`wiresteward_wg_peer_receive_bytes_total{device="wg0",public_key="%v"} 1`, pubPeerA.String()),
				fmt.Sprintf(`wiresteward_wg_peer_receive_bytes_total{device="wg0",public_key="%v"} 0`, pubPeerB.String()),
				fmt.Sprintf(`wiresteward_wg_peer_transmit_bytes_total{device="wg0",public_key="%v"} 2`, pubPeerA.String()),
				fmt.Sprintf(`wiresteward_wg_peer_transmit_bytes_total{device="wg0",public_key="%v"} 0`, pubPeerB.String()),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := promtest.Collect(t, newMetricsCollector(tt.devices))

			if !promtest.Lint(t, body) {
				t.Fatal("one or more promlint errors found")
			}

			if !promtest.Match(t, body, tt.metrics) {
				t.Fatal("metrics did not match whitelist")
			}
		})
	}
}

// return a wg key or panic
func newWgKey() wgtypes.Key {
	key, err := wgtypes.GenerateKey()
	if err != nil {
		panic(fmt.Sprintf("Cannot generate new wg key: %v", err))
	}
	return key
}
