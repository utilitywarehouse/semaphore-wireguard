# semaphore-wireguard

Borrows from [semaphore-service-mirror](https://github.com/utilitywarehouse/semaphore-service-mirror) and [wiresteward](https://github.com/utilitywarehouse/wiresteward).

It is an experimental wireguard peer manager that is meant to run as a
kubernetes daemonset and peer local cluster nodes with a remote cluster's nodes
that also run semaphore-wireguard.

It creates a single wireguard interface per remote cluster on the host it is
running and add all the remote wireguard peers it can discover, thus node to
node connectivity is needed between clusters. All routing is done using the
wireguard interface and a single route created on the host for the whole remote
pod subnet. It does not clean up network configuration on teardown, so restarts
can go unnoticed but syncs on startup.

### Usage

```
Usage of ./semaphore-wireguard:
  -clusters-config string
        Path to the clusters' json config file
  -listen-address string
        Listen address to serve health and metrics (default ":7773")
  -log-level string
        Log level (default "info")
  -node-name string
        (Required) The node on which semaphore-wireguard is running
  -wg-key-path string
        Path to store and look for wg private key (default "/var/lib/semaphore-wireguard")
```

# Config

A json config is expected to define all the needed information regarding the
local and the remote clusters that semaphore-wireguard operates on.

## Local
- name: the name of the local cluster, it will be used on the created nodes
  annotations (<name>.wireguard.semaphore.uw.io)

## Remotes
Is a list of remote clusters that may define the following:
- name: The name of the remote cluster. This will be used when creating the
  local wireguard interfaces (wireguard.<name>) and for watching annotations on
  remote clusters nodes (annotation pattern as the local config above). Thus,
  a remote deployment that uses the same name as local should exist.

- remoteAPIURL: The kube apiserver url for the remote cluster.

- remoteCAURL: A public endpoint to fetch the remote clusters CA.

- remoteSATokenPath: Path to the service account token that would allow watching
  the remote cluster's nodes.

- kubeConfigPath: The path to a kube config file. This is an alternative for the
  3 above configuration options.

- podSubnet: The cluster's pod subnet. Will be used to configure a static route
  to the subnet via the created wg interface

- wgDeviceMTU: MTU for the created wireguard interface.

- wgListenPort: WG listen port.

- fullPeerResyncPeriod: Period to attempt a full wg peers resync based on the
  remote cluster's node list.

- watcherResyncPeriod: Resync period for kube watcher cache.

## Example
```
{
  "local": {
    "name": "local"
  },
  "remotes": [
    {
      "name": "remote1",
      "remoteAPIURL": "https://lb.master.k8s.uw.systems",
      "remoteCAURL": "https://kube-ca-cert.uw.systems",
      "remoteSATokenPath": "/etc/semaphore-wireguard/tokens/gcp/token",
      "podSubnet": "10.4.0.0/16",
      "wgDeviceMTU": 1380,
      "wgListenPort": 51821
    }
  ]
}
```

# Deployment

See example kube manifests to deploy under [example dir](./deploy/exmple/).
