// pkg/watch/info.md         /go/src/k8s.io/kubernetes/staging/src/k8s.io/client-go/pkg/watch/info.md

/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package watch

import (
	"fmt"
	"time"

	"encoding/json"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	// meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	mgr "github.com/davidwalter0/llb/manager"
	"github.com/davidwalter0/llb/share"
)

var EnvCfg *share.ServerCfg

func SetConfig(e *share.ServerCfg) {
	EnvCfg = e
}

type EType string

const (
	ADD    = "ADD"
	DELETE = "DELETE"
	UPDATE = "UPDATE"
)

type Event struct {
	Key string
	EType
	*v1.Service
}

var Events chan Event = make(chan Event, 100)

type Controller struct {
	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller
}

func NewController(queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller) *Controller {
	return &Controller{
		informer: informer,
		indexer:  indexer,
		queue:    queue,
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	event, quit := c.queue.Get()
	if quit {
		return false
	}

	// Tell the queue that we are done with processing this event. This
	// unblocks the event for other workers This allows safe parallel
	// processing because two services with the same event are never
	// processed in parallel.
	defer c.queue.Done(event)

	// Invoke the method containing the business logic
	err := c.Publish(event.(Event))
	// Handle the error if something went wrong during the execution of
	// the business logic
	c.handleErr(err, event)
	return true
}

// Publish is the business logic of the controller publishing to the
// mgr's service channel.
func (c *Controller) Publish(event Event) error {
	key := event.Key
	etype := event.EType
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		fmt.Printf("Service %s does not exist anymore\n", key)
		mgr.RemovedServices <- key
	} else {
		// Note that you also have to check the uid if you have a local
		// controlled resource, which is dependent on the actual instance,
		// to detect that a Service was recreated with the same name
		Service := obj.(*v1.Service)
		fmt.Printf("Sync/Add/Update for Service %s\n", key)
		// fmt.Printf("Sync/Add/Update for Service %s\n", Service.GetName())
		// Publish to manager for load balancer types
		if Service.Spec.Type == "LoadBalancer" {
			name := Service.ObjectMeta.Name
			ns := Service.ObjectMeta.Namespace
			fmt.Printf("Load balancer found notify mgr ns/name %s/%s\n", ns, name)
			mgr.Services <- Service
		}
		if false && EnvCfg.Debug {
			var jsonbytes []byte
			if jsonbytes, err = json.MarshalIndent(Service, "", "  "); err == nil {
				fmt.Printf("JSON for Service\nName: %s Key: %s Event: %s\n%s\n%v\n",
					Service.GetName(),
					key,
					etype,
					string(jsonbytes),
					Service.Spec)

			} else {
				fmt.Println(err)
			}
		}
	}
	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, event interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the event on every
		// successful synchronization.  This ensures that future
		// processing of updates for this event is not delayed because of
		// an outdated error history.
		c.queue.Forget(event)
		return
	}

	// This controller retries 5 times if something goes wrong. After
	// that, it stops trying.
	if c.queue.NumRequeues(event) < 5 {
		glog.Infof("Error syncing service %v: %v", event, err)

		// Re-enqueue the event rate limited. Based on the rate limiter on
		// the queue and the re-enqueue history, the event will be
		// processed later again.
		c.queue.AddRateLimited(event)
		return
	}

	c.queue.Forget(event)
	// Report to an external entity that, even after several retries, we
	// could not successfully process this event
	runtime.HandleError(err)
	glog.Infof("Dropping service %q out of the queue: %v", event, err)
}

func (c *Controller) Run(threadiness int, stopCh chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	glog.Info("Starting Service controller")

	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing
	// items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	glog.Info("Stopping Service controller")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func RunWatcher(clientset *kubernetes.Clientset) {
	serviceListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())

	// create the workqueue
	ratelimiter := workqueue.DefaultControllerRateLimiter()
	queue := workqueue.NewRateLimitingQueue(ratelimiter)

	// Bind the workqueue to a cache with the help of an informer. This
	// way we make sure that whenever the cache is updated, the service
	// event is added to the workqueue.  Note that when we finally
	// process the item from the workqueue, we might see a newer version
	// of the Service than the version which was responsible for
	// triggering the update.
	indexer, informer := cache.NewIndexerInformer(serviceListWatcher, &v1.Service{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(Event{Key: key, EType: ADD})
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				queue.Add(Event{Key: key, EType: UPDATE})
			}
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// event function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(Event{Key: key, EType: DELETE})
			}
		},
	}, cache.Indexers{})

	controller := NewController(queue, indexer, informer)

	// Now let's start the controller
	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(1, stop)

	// Wait forever
	select {}
}
