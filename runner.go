package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/utilitywarehouse/semaphore-wireguard/kube"
	"github.com/utilitywarehouse/semaphore-wireguard/log"
	"github.com/utilitywarehouse/semaphore-wireguard/wireguard"
)

// Peer keeps the config for a wireguard peer.
type Peer struct {
	allowedIPs []string
	endpoint   string
}

// RunnerAnnotations contains the annotations that each runner should use for
// updating its local node and watching a remote cluster.
type RunnerAnnotations struct {
	watchAnnotationWGPublicKey      string
	watchAnnotationWGEndpoint       string
	advertisedAnnotationWGPublicKey string
	advertisedAnnotationWGEndpoint  string
}

func constructRunnerAnnotations(localClusterName, remoteClusterName string) RunnerAnnotations {
	return RunnerAnnotations{
		watchAnnotationWGPublicKey:      fmt.Sprintf(annotationWGPublicKeyPattern, localClusterName),
		watchAnnotationWGEndpoint:       fmt.Sprintf(annotationWGEndpointPattern, localClusterName),
		advertisedAnnotationWGPublicKey: fmt.Sprintf(annotationWGPublicKeyPattern, remoteClusterName),
		advertisedAnnotationWGEndpoint:  fmt.Sprintf(annotationWGEndpointPattern, remoteClusterName),
	}
}

// Runner is the main runner that keeps a watch on the remote cluster's nodes
// and adds/removes local peers.
type Runner struct {
	nodeName    string
	client      kubernetes.Interface
	podSubnet   *net.IPNet
	device      *wireguard.Device
	nodeWatcher *kube.NodeWatcher
	peers       map[string]Peer
	canUpdate   bool // Flag to allow updating wireguard peers only after initial node watcher sync
	annotations RunnerAnnotations
}

func newRunner(client, watchClient kubernetes.Interface, nodeName, wgDeviceName, wgKeyPath, localClusterName, remoteClusterName string, wgDeviceMTU, wgListenPort int, podSubnet *net.IPNet, resyncPeriod time.Duration) *Runner {
	runner := &Runner{
		nodeName:    nodeName,
		client:      client,
		podSubnet:   podSubnet,
		peers:       make(map[string]Peer),
		canUpdate:   false,
		annotations: constructRunnerAnnotations(localClusterName, remoteClusterName),
	}
	runner.device = wireguard.NewDevice(wgDeviceName, wgKeyPath, wgDeviceMTU, wgListenPort)
	nw := kube.NewNodeWatcher(
		watchClient,
		resyncPeriod,
		runner.nodeEventHandler,
	)
	runner.nodeWatcher = nw
	runner.nodeWatcher.Init()

	return runner
}

// Run will set up local interface and route, and start the nodes watcher.
func (r *Runner) Run() error {
	if err := r.device.Run(); err != nil {
		return err
	}
	if err := r.device.Configure(); err != nil {
		return err
	}
	if err := r.patchLocalNode(); err != nil {
		return err
	}
	// TODO: see if we need to set an address on the wg interface, seems
	// that everything can work without it
	//if err := r.setLocalDeviceAddress(); err != nil {
	//	return err
	//}
	if err := r.device.FlushAddresses(); err != nil {
		return err
	}
	if err := r.device.EnsureLinkUp(); err != nil {
		return err
	}
	go r.nodeWatcher.Run()
	// wait for node watcher to sync. TODO: atm dummy and could run forever
	// if node cache fails to sync
	stopCh := make(chan struct{})
	if ok := cache.WaitForNamedCacheSync("nodeWatcher", stopCh, r.nodeWatcher.HasSynced); !ok {
		return fmt.Errorf("failed to wait for nodes cache to sync")
	}
	// Static route to the whole subnet cidr
	if err := r.device.AddRouteToNet(r.podSubnet); err != nil {
		return err
	}
	r.canUpdate = true
	if err := r.Update(); err != nil {
		log.Logger.Warn("Update failed", "err", err)
	}
	return nil
}

// Update is responsible for updating local wireguard device peers and routes
func (r *Runner) Update() error {
	if !r.canUpdate {
		return fmt.Errorf("Cannot update while canUpdate flag is not set")
	}
	var peersConfig []wgtypes.PeerConfig
	for pubKey, peer := range r.peers {
		pc, err := wireguard.NewPeerConfig(pubKey, "", peer.endpoint, peer.allowedIPs)
		if err != nil {
			return err
		}
		peersConfig = append(peersConfig, *pc)
	}
	log.Logger.Debug("Updating wg peers", "peers", peersConfig)
	if err := wireguard.SetPeers(r.device.Name(), peersConfig); err != nil {
		return err
	}
	return nil
}

// patchLocalNode will make sure we set the needed annotations on the node and
// should be called after the local wg device is set.
func (r *Runner) patchLocalNode() error {
	ctx := context.Background()
	node, err := r.client.CoreV1().Nodes().Get(ctx, r.nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	var wgEndpoint string
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			wgEndpoint = fmt.Sprintf("%s:%d", addr.Address, r.device.ListenPort())
			break
		}
	}
	if wgEndpoint == "" {
		return fmt.Errorf("Could not calculate wg endpoint, node internal address not found")
	}
	annotations := map[string]string{
		r.annotations.advertisedAnnotationWGPublicKey: r.device.PublicKey(),
		r.annotations.advertisedAnnotationWGEndpoint:  wgEndpoint,
	}
	if err := kube.PatchNodeAnnotation(r.client, r.nodeName, annotations); err != nil {
		return err
	}
	return nil
}

// setLocalDeviceAddress gets the pod cidr from the local node spec and assignes
// the first address to the wireguard interface.
func (r *Runner) setLocalDeviceAddress() error {
	ctx := context.Background()
	node, err := r.client.CoreV1().Nodes().Get(ctx, r.nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// TODO: derive an ip from node's spec pod cidr
	// For now let's rely on calico
	calicoIP, ok := node.Annotations["projectcalico.org/IPv4IPIPTunnelAddr"]
	if !ok {
		return fmt.Errorf("Cannot get ip from calico annotations, is calico running on the node?")
	}
	return r.device.UpdateAddress(&net.IPNet{
		IP:   net.ParseIP(calicoIP),
		Mask: net.CIDRMask(32, 32),
	})
}

func (r *Runner) checkWSAnnotationsExist(annotations map[string]string) bool {
	_, ok := annotations[r.annotations.watchAnnotationWGPublicKey]
	if !ok {
		return false
	}
	_, ok = annotations[r.annotations.watchAnnotationWGEndpoint]
	if !ok {
		return false
	}
	return true
}

func (r *Runner) nodeEventHandler(eventType watch.EventType, old *v1.Node, new *v1.Node) {
	switch eventType {
	case watch.Added:
		if r.checkWSAnnotationsExist(new.Annotations) {
			r.onPeerNodeUpdate(new)
		} else {
			log.Logger.Debug("Added node missing the needed ws annotations", "node", new.Name)
		}
	case watch.Modified:
		if r.checkWSAnnotationsExist(new.Annotations) {
			r.onPeerNodeUpdate(new)
		} else {
			log.Logger.Debug("Modified node missing the needed ws annotations", "node", new.Name)
		}
	case watch.Deleted:
		if _, ok := old.Annotations[r.annotations.watchAnnotationWGPublicKey]; ok {
			r.onPeerNodeDelete(old)
		} else {
			log.Logger.Debug("Deleted node missing the needed ws annotations", "node", old.Name)
		}
	default:
		log.Logger.Info(
			"Unknown service event received: %v",
			eventType,
		)
	}
}

func equalSlices(a, b []string) bool {
	// If one is nil, the other must also be nil.
	if (a == nil) != (b == nil) {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func (r *Runner) onPeerNodeUpdate(node *v1.Node) {
	log.Logger.Debug("On peer node update", "namename", node.Name)
	pubKey := node.Annotations[r.annotations.watchAnnotationWGPublicKey]
	peer := Peer{
		allowedIPs: []string{node.Spec.PodCIDR},
		endpoint:   node.Annotations[r.annotations.watchAnnotationWGEndpoint],
	}
	// Check if peer needs to be updated
	if oldPeer, ok := r.peers[pubKey]; ok {
		if equalSlices(oldPeer.allowedIPs, peer.allowedIPs) && oldPeer.endpoint == peer.endpoint {
			return
		}
	}
	r.peers[pubKey] = peer
	if err := r.Update(); err != nil {
		log.Logger.Warn("Update failed", "err", err)
	}
}

func (r *Runner) onPeerNodeDelete(node *v1.Node) {
	log.Logger.Debug("On peer node delete", "namename", node.Name)
	pubKey := node.Annotations[r.annotations.watchAnnotationWGPublicKey]
	if _, ok := r.peers[pubKey]; ok {
		delete(r.peers, pubKey)
	}
	if err := r.Update(); err != nil {
		log.Logger.Warn("Update failed", "err", err)
	}
}

// Healthy is true if the node watcher is reporting healthy.
func (r *Runner) Healthy() bool {
	return r.nodeWatcher.Healthy()
}
