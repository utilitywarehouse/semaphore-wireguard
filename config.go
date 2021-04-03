package main

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	defaultWGDeviceMTU  = 1420
	defaultWGListenPort = 51820
)

// https://stackoverflow.com/questions/48050945/how-to-unmarshal-json-into-durations/54571600#54571600
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		tmp, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		d.Duration = tmp
		return nil
	default:
		return fmt.Errorf("Invalid duration of type %v", value)
	}
}

var (
	defaultWatcherResyncPeriod = Duration{1 * time.Hour}
	zeroDuration               = Duration{0}
)

type localClusterConfig struct {
	Name           string `json:"name"`
	KubeConfigPath string `json:"kubeConfigPath"`
}

type remoteClusterConfig struct {
	Name              string   `json:"name"`
	KubeConfigPath    string   `json:"kubeConfigPath"`
	RemoteAPIURL      string   `json:"remoteAPIURL"`
	RemoteCAURL       string   `json:"remoteCAURL"`
	RemoteSATokenPath string   `json:"remoteSATokenPath"`
	WGDeviceMTU       int      `json:"wgDeviceMTU"`
	WGListenPort      int      `json:"wgListenPort"`
	PodSubnet         string   `json:"podSubnet"`
	ResyncPeriod      Duration `json:"resyncPeriod"`
}

type Config struct {
	Local   localClusterConfig     `json:"local"`
	Remotes []*remoteClusterConfig `json:"remotes"`
}

func parseConfig(rawConfig []byte) (*Config, error) {
	conf := &Config{}
	if err := json.Unmarshal(rawConfig, conf); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %v", err)
	}
	// Check for mandatory local config.
	if conf.Local.Name == "" {
		return nil, fmt.Errorf("Configuration is missing local cluster name")
	}
	if len(conf.Remotes) < 1 {
		return nil, fmt.Errorf("No remote cluster configuration defined")
	}
	// Check for mandatory remote config.
	for _, r := range conf.Remotes {
		if r.Name == "" {
			return nil, fmt.Errorf("Configuration is missing remote cluster name")
		}
		if (r.RemoteAPIURL == "" || r.RemoteCAURL == "" || r.RemoteSATokenPath == "") && r.KubeConfigPath == "" {
			return nil, fmt.Errorf("Insufficient configuration to create remote cluster client. Set kubeConfigPath or remoteAPIURL and remoteCAURL and remoteSATokenPath")
		}
		if r.PodSubnet == "" {
			return nil, fmt.Errorf("No pod subnet defined for remote cluster")
		}
		if r.WGDeviceMTU == 0 {
			r.WGDeviceMTU = defaultWGDeviceMTU
		}
		if r.WGListenPort == 0 {
			r.WGListenPort = defaultWGListenPort
		}
		if r.ResyncPeriod == zeroDuration {
			r.ResyncPeriod = defaultWatcherResyncPeriod
		}
	}
	return conf, nil
}
