/*
Copyright 2019 Intel Corporation

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

package agent

import (
	"sync"
	"time"

	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	k8sclient "k8s.io/client-go/kubernetes"

	"github.com/intel/cri-resource-manager/pkg/log"
)

type cachedConfig struct {
	sync.RWMutex
	config resmgrConfig
}

// k8sWatcher is our interface to K8s control plane watcher
type k8sWatcher interface {
	// Start the watcher instance
	Start() error
	// Stop the watcher instance
	Stop()
	// Get a chan through which to receive configuration updates
	ConfigChan() <-chan resmgrConfig
	// Get up-to-date config
	GetConfig() resmgrConfig
}

// watcher implements k8sWatcher
type watcher struct {
	log.Logger
	stop          chan struct{}        // Flag to stop the watcher
	k8sCli        *k8sclient.Clientset // Client interface for kubernetes control plane
	currentConfig cachedConfig         // Current cri-resmgr config, cached
	configChan    chan resmgrConfig    // Chan for sending config updates
}

// newK8sWatcher creates a new K8sWatcher instance
func newK8sWatcher(k8sCli *k8sclient.Clientset) (k8sWatcher, error) {
	w := &watcher{
		Logger:        log.NewLogger("watcher"),
		k8sCli:        k8sCli,
		stop:          make(chan struct{}, 1),
		currentConfig: cachedConfig{config: resmgrConfig{}},
		configChan:    make(chan resmgrConfig, 1),
	}

	return w, nil
}

// Start runs a k8sWatcher instance
func (w *watcher) Start() error {
	// First, get and cache initial configuration
	w.Debug("getting ConfigMap %q in ns %q", opts.configMapName, opts.configNs)
	config, err := w.k8sCli.CoreV1().ConfigMaps(opts.configNs).Get(opts.configMapName, meta_v1.GetOptions{})
	if err != nil {
		w.Info("empty initial config, failed to get configmap: %v", err)
	} else {
		w.currentConfig.set(config.Data)
	}
	// Send initial config
	w.configChan <- w.currentConfig.get()

	// Then, start to watch for changes in configuration
	w.Info("starting watcher...")
	go func() {
		w.watch()
	}()
	return nil
}

// Stop stops a running k8sWatcher instance
func (w *watcher) Stop() {
	select {
	case w.stop <- struct{}{}:
	default:
		w.Debug("stop already sent")
	}
}

// ConfigChan returns the chan for config updates
func (w *watcher) ConfigChan() <-chan resmgrConfig {
	return w.configChan
}

// GetConfig returns the current cri-resmgr configuration
func (w *watcher) GetConfig() resmgrConfig {
	return w.currentConfig.get()
}

func (w *watcher) watch() error {
	var cmw watch.Interface
	// Start with a closed channel. We want this so that we end up in creating
	// a new watch in the main event loop below
	eventChan := func() <-chan watch.Event {
		c := make(chan watch.Event)
		close(c)
		return c
	}()

	w.Info("watcher running")
	for {
		select {
		case _ = <-w.stop:
			if cmw != nil {
				cmw.Stop()
			}
			w.Info("watcher stopped")
			return nil
		case event, ok := <-eventChan:
			if ok {
				w.Debug("received %s event", event.Type)
				w.handleEvent(event)
			} else {
				var err error
				w.Debug("creating watch for ConfigMap %q in ns %q", opts.configMapName, opts.configNs)
				cmw, err = watchConfigMap(w.k8sCli, opts.configNs, opts.configMapName)
				if err != nil {
					w.Error("failed to create watch: %v", err)
					time.Sleep(1 * time.Second)
				} else {
					eventChan = cmw.ResultChan()
				}
			}
		}
	}
}

func (w *watcher) handleEvent(event watch.Event) error {
	switch event.Object.(type) {
	case *core_v1.ConfigMap:
		switch t := event.Type; t {
		case watch.Added, watch.Modified:
			configMap := event.Object.(*core_v1.ConfigMap)
			w.currentConfig.set(configMap.Data)
		case watch.Deleted:
			w.currentConfig.set(resmgrConfig{})
		default:
			w.Debug("Ignoring event %q", t)
			return nil
		}

		// Pop possible outdated config from the chan
		select {
		case <-w.configChan:
			w.Debug("trashed outdated config update event")
		default:
		}

		// Send in new config
		w.configChan <- w.currentConfig.get()
	default:
		w.Error("BUG: object type %T instead of *core_v1.ConfigMap", event.Object)
	}
	return nil
}

// get is a helper method for getting the config data
func (c *cachedConfig) get() resmgrConfig {
	c.RLock()
	defer c.RUnlock()
	return c.config
}

// set is a helper method for setting the config data
func (c *cachedConfig) set(newConfig resmgrConfig) {
	c.Lock()
	c.config = newConfig
	c.Unlock()
}
