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
	nodeName         string
	client           kubernetes.Interface
	podSubnet        *net.IPNet
	device           *wireguard.Device
	nodeWatcher      *kube.NodeWatcher
	peers            map[string]Peer
	canSync          bool // Flag to allow updating wireguard peers only after initial node watcher sync
	annotations      RunnerAnnotations
	fullResyncPeriod time.Duration
	sync             chan struct{}
	stop             chan struct{}
}

func newRunner(client, watchClient kubernetes.Interface, nodeName, wgDeviceName, wgKeyPath, localClusterName, remoteClusterName string, wgDeviceMTU, wgListenPort int, podSubnet *net.IPNet, watcherResyncPeriod, fullPeerResyncPeriod time.Duration) *Runner {
	runner := &Runner{
		nodeName:         nodeName,
		client:           client,
		podSubnet:        podSubnet,
		peers:            make(map[string]Peer),
		canSync:          false,
		annotations:      constructRunnerAnnotations(localClusterName, remoteClusterName),
		fullResyncPeriod: fullPeerResyncPeriod,
		sync:             make(chan struct{}),
		stop:             make(chan struct{}),
	}
	runner.device = wireguard.NewDevice(wgDeviceName, wgKeyPath, wgDeviceMTU, wgListenPort)
	nw := kube.NewNodeWatcher(
		watchClient,
		watcherResyncPeriod,
		runner.nodeEventHandler,
	)
	runner.nodeWatcher = nw
	runner.nodeWatcher.Init()
	go runner.syncLoop()

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
	r.canSync = true
	r.enqueuePeersSync()
	return nil
}

func (r *Runner) syncLoop() {
	ticker := time.NewTicker(r.fullResyncPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-r.sync:
			r.doSyncPeers()
		case <-ticker.C:
			log.Logger.Info("Full sync ticker expired, attempting a peers sync")
			r.doSyncPeers()
		case <-r.stop:
			log.Logger.Debug("Stopping sync loop")
			return
		}
	}
}

func (r *Runner) doSyncPeers() {
	if !r.canSync {
		log.Logger.Warn("Cannot sync peers while canSync flag is not set")
		return
	}
	err := r.syncPeers()
	MetricsSyncPeerAttempt(r.device.Name(), err)
	if err != nil {
		log.Logger.Warn("Failed to sync wg peers", "err", err)
		r.requeuePeersSync()
	}
}

// syncPeers will try to get a list of peers based on the nodes list and set wg
// peers based on the nodes annotations. It also updates the runner's peer
// variable.
func (r *Runner) syncPeers() error {
	peers, err := r.calculatePeersFromNodeList()
	if err != nil {
		return fmt.Errorf("Failed to get peers list: %v", err)
	}
	var peersConfig []wgtypes.PeerConfig
	for pubKey, peer := range peers {
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
	r.peers = peers
	return nil
}

func (r *Runner) enqueuePeersSync() {
	select {
	case r.sync <- struct{}{}:
		log.Logger.Debug("Sync task queued")
	case <-time.After(5 * time.Second):
		log.Logger.Error("Timed out trying to queue a sync action for netset, sync queue is full")
		MetricsIncSyncQueueFullFailures(r.device.Name())
		r.requeuePeersSync()
	}
}

func (r *Runner) requeuePeersSync() {
	log.Logger.Debug("Requeueing peers sync task")
	go func() {
		time.Sleep(1)
		MetricsIncSyncRequeue(r.device.Name())
		r.enqueuePeersSync()
	}()
}

func (r *Runner) calculatePeersFromNodeList() (map[string]Peer, error) {
	nodes, err := r.nodeWatcher.List()
	if err != nil {
		return nil, err
	}
	peers := map[string]Peer{}
	for _, node := range nodes {
		if r.checkWSAnnotationsExist(node.Annotations) {
			pubKey := node.Annotations[r.annotations.watchAnnotationWGPublicKey]
			peer := Peer{
				allowedIPs: []string{node.Spec.PodCIDR},
				endpoint:   node.Annotations[r.annotations.watchAnnotationWGEndpoint],
			}
			peers[pubKey] = peer
		}
	}
	return peers, nil
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
	r.enqueuePeersSync()
}

func (r *Runner) onPeerNodeDelete(node *v1.Node) {
	log.Logger.Debug("On peer node delete", "namename", node.Name)
	pubKey := node.Annotations[r.annotations.watchAnnotationWGPublicKey]
	if _, ok := r.peers[pubKey]; !ok {
		// if peer is not in the list we do not need to update anything
		return
	}
	r.enqueuePeersSync()
}

// Healthy is true if the node watcher is reporting healthy.
func (r *Runner) Healthy() bool {
	return r.nodeWatcher.Healthy()
}
