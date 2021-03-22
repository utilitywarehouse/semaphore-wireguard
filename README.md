# kube-wiresteward

Borrows heavily from [kube-service-mirror](https://github.com/utilitywarehouse/kube-service-mirror) and [wiresteward](https://github.com/utilitywarehouse/wiresteward).

It is an experimental wireguard peer manager that is meant to run as a
kubernetes daemonset and peer local cluster nodes with a remote cluster's nodes
that also run kube-wiresteward.

Atm, it creates a single wireguard interface on the host it is running and add
all the remote wireguard peers it can discover, thus node to node connectivity
is needed between clusters. All routing is done using the wireguard interface
and a single route created on the host for the whole remote pod subnet.
It does not clean up network configuration on teardown, so restarts can go
unnoticed but syncs on startup.

### Usage

```
Usage of ./kube-wiresteward:
  -local-kube-config string
        Path of the local kube cluster config file, if not provided the app will try to get in cluster config
  -log-level string
        Log level (default "info")
  -remote-api-url string
        Remote Kubernetes API server URL
  -remote-ca-url string
        Remote Kubernetes CA certificate URL
  -remote-pod-subnet string
        Subnet to route via the created wg interface
  -remote-sa-token-path string
        Remote Kubernetes cluster token path
  -resync-period duration
        Node watcher cache resync period (default 1h0m0s)
  -target-kube-config string
        (Required) Path of the target cluster kube config file to add wg peers from
  -wg-device-mtu string
        MTU for wg device (default "1420")
  -wg-device-name string
        (Required) The name of the wireguard device to be created
  -wg-key-path string
        Path to store and look for wg private key (default "/var/lib/wiresteward")
  -wg-listen-port string
        Wg listen port (default "51820")
  -ws-node-name string
        (Required) The node on which wiresteward is running
```

# Deployment

See example kube manifests to deploy under [example dir](./deploy/exmple/).
