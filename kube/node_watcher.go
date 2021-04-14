package kube

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/utilitywarehouse/semaphore-wireguard/log"
)

// NodeEventHandler is the function to handle new events
type NodeEventHandler = func(eventType watch.EventType, old *v1.Node, new *v1.Node)

// NodeWatcher has a watch on the clients nodes
type NodeWatcher struct {
	ctx          context.Context
	client       kubernetes.Interface
	resyncPeriod time.Duration
	stopChannel  chan struct{}
	store        cache.Store
	controller   cache.Controller
	eventHandler NodeEventHandler
	ListHealthy  bool
	WatchHealthy bool
}

// NewNodeWatcher returns a new node wathcer.
func NewNodeWatcher(client kubernetes.Interface, resyncPeriod time.Duration, handler NodeEventHandler) *NodeWatcher {
	return &NodeWatcher{
		ctx:          context.Background(),
		client:       client,
		resyncPeriod: resyncPeriod,
		stopChannel:  make(chan struct{}),
		eventHandler: handler,
	}
}

// Init sets up the list, watch functions and the cache.
func (nw *NodeWatcher) Init() {
	listWatch := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			l, err := nw.client.CoreV1().Nodes().List(nw.ctx, options)
			if err != nil {
				log.Logger.Error("nw: list error", "err", err)
				nw.ListHealthy = false
			} else {
				nw.ListHealthy = true
			}
			return l, err
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			w, err := nw.client.CoreV1().Nodes().Watch(nw.ctx, options)
			if err != nil {
				log.Logger.Error("nw: watch error", "err", err)
				nw.WatchHealthy = false
			} else {
				nw.WatchHealthy = true
			}
			return w, err
		},
	}
	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			nw.eventHandler(watch.Added, nil, obj.(*v1.Node))
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			nw.eventHandler(watch.Modified, oldObj.(*v1.Node), newObj.(*v1.Node))
		},
		DeleteFunc: func(obj interface{}) {
			nw.eventHandler(watch.Deleted, obj.(*v1.Node), nil)
		},
	}
	nw.store, nw.controller = cache.NewInformer(listWatch, &v1.Node{}, nw.resyncPeriod, eventHandler)
}

// Run will not return unless writting in the stop channel
func (nw *NodeWatcher) Run() {
	log.Logger.Info("starting node watcher")
	// Running controller will block until writing on the stop channel.
	nw.controller.Run(nw.stopChannel)
	log.Logger.Info("stopped node watcher")
}

// Stop stop the watcher via the respective channel
func (nw *NodeWatcher) Stop() {
	log.Logger.Info("stopping node watcher")
	close(nw.stopChannel)
}

// HasSynced calls controllers HasSync method to determine whether the watcher
// cache is synced.
func (nw *NodeWatcher) HasSynced() bool {
	return nw.controller.HasSynced()
}

// List lists all nodes from the store
func (nw *NodeWatcher) List() ([]*v1.Node, error) {
	var svcs []*v1.Node
	for _, obj := range nw.store.List() {
		svc, ok := obj.(*v1.Node)
		if !ok {
			return nil, fmt.Errorf("unexpected object in store: %+v", obj)
		}
		svcs = append(svcs, svc)
	}
	return svcs, nil
}

// Healthy is true when both list and watch handlers are running without errors.
func (nw *NodeWatcher) Healthy() bool {
	return nw.ListHealthy && nw.WatchHealthy
}
