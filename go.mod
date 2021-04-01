module github.com/utilitywarehouse/kube-wiresteward

go 1.16

require (
	github.com/hashicorp/go-hclog v0.14.1
	github.com/mdlayher/promtest v0.0.0-20200528141414-3c8577d47d5c
	github.com/prometheus/client_golang v1.9.0
	github.com/stretchr/testify v1.6.1 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20200802231818-98629f7ffc4b
	golang.zx2c4.com/wireguard v0.0.20200121
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20200609130330-bd2cb7843e1b
	inet.af/netaddr v0.0.0-20210317195617-2d42ec05f8a1
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.4
)
