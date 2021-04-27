package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	emptyConfig := []byte(`
{
  "local": {
  }
}
`)
	_, err := parseConfig(emptyConfig)
	assert.Equal(t, fmt.Errorf("Configuration is missing local cluster name"), err)

	localConfigOnly := []byte(`
{
  "local": {
    "name": "local_cluster",
    "kubeConfigPath": "/path/to/kube/config"
  }
}
`)
	_, err = parseConfig(localConfigOnly)
	assert.Equal(t, fmt.Errorf("No remote cluster configuration defined"), err)

	emptyRemoteConfigName := []byte(`
{
  "local": {
    "name": "local_cluster",
    "kubeConfigPath": "/path/to/kube/config"
  },
  "remotes": [
    {
      "name": ""
    }
  ]
}
`)
	_, err = parseConfig(emptyRemoteConfigName)
	assert.Equal(t, fmt.Errorf("Configuration is missing remote cluster name"), err)
	insufficientRemoteKubeConfigPath := []byte(`
{
  "local": {
    "name": "local_cluster",
    "kubeConfigPath": "/path/to/kube/config"
  },
  "remotes": [
    {
      "name": "remote_cluster_1",
      "remoteCAURL": "remote_ca_url",
      "remoteAPIURL": "remote_api_url"
    }
  ]
}
`)
	_, err = parseConfig(insufficientRemoteKubeConfigPath)
	assert.Equal(t, fmt.Errorf("Insufficient configuration to create remote cluster client. Set kubeConfigPath or remoteAPIURL and remoteCAURL and remoteSATokenPath"), err)

	rawFullConfig := []byte(`
{
  "local": {
    "name": "local_cluster",
    "kubeConfigPath": "/path/to/kube/config"
  },
  "remotes": [
    {
      "name": "remote_cluster_1",
      "remoteCAURL": "remote_ca_url",
      "remoteAPIURL": "remote_api_url",
      "remoteSATokenPath": "/path/to/token",
      "podSubnet": "10.0.0.0/16",
      "wgDeviceMTU": 1500,
      "wgListenPort": 51821,
      "resyncPeriod": "10s"
    },
    {
      "name": "remote_cluster_2",
      "kubeConfigPath": "/path/to/kube/config",
      "podSubnet": "10.0.1.0/16"
    }
  ]
}
`)
	config, err := parseConfig(rawFullConfig)
	assert.Equal(t, nil, err)
	assert.Equal(t, "local_cluster", config.Local.Name)
	assert.Equal(t, "/path/to/kube/config", config.Local.KubeConfigPath)
	assert.Equal(t, 2, len(config.Remotes))
	assert.Equal(t, "remote_cluster_1", config.Remotes[0].Name)
	assert.Equal(t, "remote_ca_url", config.Remotes[0].RemoteCAURL)
	assert.Equal(t, "remote_api_url", config.Remotes[0].RemoteAPIURL)
	assert.Equal(t, "/path/to/token", config.Remotes[0].RemoteSATokenPath)
	assert.Equal(t, "", config.Remotes[0].KubeConfigPath)
	assert.Equal(t, "10.0.0.0/16", config.Remotes[0].PodSubnet)
	assert.Equal(t, 1500, config.Remotes[0].WGDeviceMTU)
	assert.Equal(t, 51821, config.Remotes[0].WGListenPort)
	assert.Equal(t, Duration{10 * time.Second}, config.Remotes[0].ResyncPeriod)
	assert.Equal(t, "remote_cluster_2", config.Remotes[1].Name)
	assert.Equal(t, "", config.Remotes[1].RemoteCAURL)
	assert.Equal(t, "", config.Remotes[1].RemoteAPIURL)
	assert.Equal(t, "", config.Remotes[1].RemoteSATokenPath)
	assert.Equal(t, "/path/to/kube/config", config.Remotes[1].KubeConfigPath)
	assert.Equal(t, "10.0.1.0/16", config.Remotes[1].PodSubnet)
	assert.Equal(t, defaultWGDeviceMTU, config.Remotes[1].WGDeviceMTU)
	assert.Equal(t, defaultWGListenPort, config.Remotes[1].WGListenPort)
	assert.Equal(t, Duration{0}, config.Remotes[1].ResyncPeriod)

}
