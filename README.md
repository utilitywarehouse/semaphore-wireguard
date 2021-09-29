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
- name: the name of the local cluster. This will be used by the controller to
  look for WireGuard configuration in remote nodes' annotations, based on the
  pattern: <name>.wireguard.semaphore.uw.io.

## Remotes
Is a list of remote clusters that may define the following:
- name: The name of the remote cluster. This will be used when creating the
  local wireguard interfaces (wireguard.<name>) and annotations to expose the
  needed configuration for remote clusters controllers.

- remoteAPIURL: The kube apiserver url for the remote cluster.

- remoteCAURL: A public endpoint to fetch the remote clusters CA.

- remoteSATokenPath: Path to the service account token that would allow watching
  the remote cluster's nodes.

- kubeConfigPath: The path to a kube config file. This is an alternative for the
  3 above configuration options.

- podSubnet: The cluster's pod subnet. Will be used to configure a static route
  to the subnet via the created wg interface. Pod subnets should be unique
  across the configuration, so that routes to different clusters pods do not
  overlap. As a result, clusters which use the same subnet for pods cannot be
  paired.

- wgDeviceMTU: MTU for the created wireguard interface.

- wgListenPort: WG listen port.

- resyncPeriod: Kubernetes watcher resync period. It should yield update events
  for everything that is stored in the cache. Default `0` value disables it.

## Cluster Naming Consistency

Cluster names should be unique and consistent across configuration of different
deployments that live in different clusters. For example, if we pick `cluster1`
as our local cluster name for a particular deployment, the same name should be
set as the remote cluster name in other deployments that will try to pair with
the local cluster.

## Example

`cluster1` configuration:

```
{
  "local": {
    "name": "cluster1"
  },
  "remotes": [
    {
      "name": "cluster2",
      "remoteAPIURL": "https://lb.cluster2.k8s.uw.systems",
      "remoteCAURL": "https://kube-ca-cert.cluster2.uw.systems",
      "remoteSATokenPath": "/etc/semaphore-wireguard/tokens/cluster2/token",
      "podSubnet": "10.4.0.0/16",
      "wgDeviceMTU": 1380,
      "wgListenPort": 51821
    }
  ]
}
```

`cluster2` configuration:

```
{
  "local": {
    "name": "cluster2"
  },
  "remotes": [
    {
      "name": "cluster1",
      "remoteAPIURL": "https://lb.cluster1.k8s.uw.systems",
      "remoteCAURL": "https://kube-ca-cert.cluster1.uw.systems",
      "remoteSATokenPath": "/etc/semaphore-wireguard/tokens/cluster1/token",
      "podSubnet": "10.2.0.0/16",
      "wgDeviceMTU": 1380,
      "wgListenPort": 51821
    }
  ]
}
```

# Deployment

See example kube manifests to deploy under [example dir](./deploy/exmple/).
