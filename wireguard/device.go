package wireguard

import (
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/utilitywarehouse/semaphore-wireguard/log"
)

// Device is the struct to hold the link device and the wireguard attributes we
// need.
type Device struct {
	deviceName  string
	link        netlink.Link
	keyFilename string
	listenPort  int
	pubKey      string
}

// NewDevice returns a new device struct.
func NewDevice(name string, keyFilename string, mtu, listenPort int) *Device {
	if mtu == 0 {
		mtu = device.DefaultMTU
	}
	return &Device{
		deviceName: name,
		link: &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{
			MTU:    mtu,
			Name:   name,
			TxQLen: 1000,
		}},
		keyFilename: keyFilename,
		listenPort:  listenPort,
	}
}

// Name returns the name of the device.
func (d *Device) Name() string {
	return d.deviceName
}

// PublicKey returns the device's wg public key
func (d *Device) PublicKey() string {
	return d.pubKey
}

// ListenPort returns the wg listen port
func (d *Device) ListenPort() int {
	return d.listenPort
}

// Run creates the wireguard device or sets mtu and txqlen if the device exists.
func (d *Device) Run() error {
	h := netlink.Handle{}
	defer h.Delete()
	l, err := h.LinkByName(d.deviceName)
	if err != nil {
		log.Logger.Info(
			"Could not get wg device by name, will try creating",
			"name", d.deviceName,
			"err", err,
		)
		if err := h.LinkAdd(d.link); err != nil {
			return err
		}
	} else {
		if err := h.LinkSetMTU(l, d.link.Attrs().MTU); err != nil {
			return err
		}
		if err := h.LinkSetTxQLen(l, d.link.Attrs().TxQLen); err != nil {
			return err
		}
	}
	return nil
}

// Configure configures wireguard keys and listen port on the device.
func (d *Device) Configure() error {
	wg, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer func() {
		if err := wg.Close(); err != nil {
			log.Logger.Error("Failed to close wireguard client", "err", err)
		}
	}()
	key, err := d.privateKey()
	if err != nil {
		return err
	}
	log.Logger.Info(
		"Configuring wireguard",
		"device", d.deviceName,
		"port", d.listenPort,
		"pubKey", key.PublicKey(),
	)
	d.pubKey = key.PublicKey().String()
	return wg.ConfigureDevice(d.deviceName, wgtypes.Config{
		PrivateKey: &key,
		ListenPort: &d.listenPort,
	})
}

func (d *Device) privateKey() (wgtypes.Key, error) {
	kd, err := os.ReadFile(d.keyFilename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Logger.Info(
				"No key found, generating a new private key",
				"path", d.keyFilename,
			)
			keyDir := filepath.Dir(d.keyFilename)
			err := os.MkdirAll(keyDir, 0755)
			if err != nil {
				log.Logger.Error(
					"Unable to create directory=%s",
					keyDir,
				)
				return wgtypes.Key{}, err
			}
			key, err := wgtypes.GeneratePrivateKey()
			if err != nil {
				return wgtypes.Key{}, err
			}
			if err := os.WriteFile(d.keyFilename, []byte(key.String()), 0600); err != nil {
				return wgtypes.Key{}, err
			}
			return key, nil
		}
		return wgtypes.Key{}, err
	}
	return wgtypes.ParseKey(string(kd))
}

// UpdateAddress will patch the device interface so it is assigned only the
// given address.
func (d *Device) UpdateAddress(address *net.IPNet) error {
	h := netlink.Handle{}
	defer h.Delete()
	link, err := h.LinkByName(d.deviceName)
	if err != nil {
		return err
	}
	d.FlushAddresses()
	if err := h.AddrAdd(link, &netlink.Addr{IPNet: address}); err != nil {
		return err
	}
	return nil
}

// FlushAddresses deletes all ips from the device network interface
func (d *Device) FlushAddresses() error {
	h := netlink.Handle{}
	defer h.Delete()
	link, err := h.LinkByName(d.deviceName)
	if err != nil {
		return err
	}
	ips, err := h.AddrList(link, 2)
	for _, ip := range ips {
		if err := h.AddrDel(link, &ip); err != nil {
			return err
		}
	}
	return nil
}

// EnsureLinkUp brings up the wireguard device.
func (d *Device) EnsureLinkUp() error {
	h := netlink.Handle{}
	defer h.Delete()
	link, err := h.LinkByName(d.deviceName)
	if err != nil {
		return err
	}
	return h.LinkSetUp(link)
}

// AddRouteToNet adds a route to the passed subnet via the device
func (d *Device) AddRouteToNet(subnet *net.IPNet) error {
	h := netlink.Handle{}
	defer h.Delete()
	link, err := h.LinkByName(d.deviceName)
	if err != nil {
		return err
	}
	return h.RouteReplace(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       subnet,
		Scope:     netlink.SCOPE_LINK,
	})
}
