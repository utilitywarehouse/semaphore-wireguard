package wireguard

import (
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/utilitywarehouse/kube-wiresteward/log"
)

const (
	defaultPersistentKeepaliveInterval = 25 * time.Second
	defaultWireguardDeviceName         = "wiresteward"
)

// NewPeerConfig constructs and returns a wgtypes PeerConfig object.
func NewPeerConfig(publicKey string, presharedKey string, endpoint string, allowedIPs []string) (*wgtypes.PeerConfig, error) {
	key, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return nil, err
	}
	t := defaultPersistentKeepaliveInterval
	peer := &wgtypes.PeerConfig{PublicKey: key, PersistentKeepaliveInterval: &t}
	if presharedKey != "" {
		key, err := wgtypes.ParseKey(presharedKey)
		if err != nil {
			return nil, err
		}
		peer.PresharedKey = &key
	}
	if endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp4", endpoint)
		if err != nil {
			return nil, err
		}
		peer.Endpoint = addr
	}
	for _, ai := range allowedIPs {
		_, network, err := net.ParseCIDR(ai)
		if err != nil {
			return nil, err
		}
		peer.AllowedIPs = append(peer.AllowedIPs, *network)
	}
	return peer, nil
}

// SetPeers takes a device name and a list of peers and updates the device's
// peers list to match the passed one.
func SetPeers(deviceName string, peers []wgtypes.PeerConfig) error {
	wg, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer func() {
		if err := wg.Close(); err != nil {
			log.Logger.Error(
				"Failed to close wireguard client", "err", err)
		}
	}()
	if deviceName == "" {
		deviceName = defaultWireguardDeviceName
	}
	device, err := wg.Device(deviceName)
	if err != nil {
		return err
	}
	for _, ep := range device.Peers {
		found := false
		for i, np := range peers {
			peers[i].ReplaceAllowedIPs = true
			if ep.PublicKey.String() == np.PublicKey.String() {
				found = true
				break
			}
		}
		if !found {
			peers = append(peers, wgtypes.PeerConfig{PublicKey: ep.PublicKey, Remove: true})
		}
	}
	return wg.ConfigureDevice(deviceName, wgtypes.Config{Peers: peers})
}
