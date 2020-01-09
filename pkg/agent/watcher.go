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
	k8swatch "k8s.io/apimachinery/pkg/watch"
	k8sclient "k8s.io/client-go/kubernetes"

	"github.com/intel/cri-resource-manager/pkg/log"
)

type cachedConfig struct {
	sync.RWMutex
	nodeCfg  *resmgrConfig // node-specific configuration
	groupCfg *resmgrConfig // group-specific configuration
	group    string        // group name, "" for default
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

	configChan chan resmgrConfig // Chan for sending config updates
}

// newK8sWatcher creates a new K8sWatcher instance
func newK8sWatcher(k8sCli *k8sclient.Clientset) (k8sWatcher, error) {
	w := &watcher{
		Logger:        log.NewLogger("watcher"),
		k8sCli:        k8sCli,
		stop:          make(chan struct{}, 1),
		currentConfig: cachedConfig{},
		configChan:    make(chan resmgrConfig, 1),
	}

	return w, nil
}

// Start runs a k8sWatcher instance
func (w *watcher) Start() error {
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
	cfg, kind := w.currentConfig.get()
	w.Info("giving %s configuration in reply to query", kind)
	return cfg
}

// sendConfig sends the current configuration.
func (w *watcher) sendConfig() {
	cfg, kind := w.currentConfig.get()
	w.Info("pushing %s configuration to client", kind)
	w.configChan <- cfg
}

func (w *watcher) watch() error {
	nodew := newNodeWatch(w)
	group := ""

	if node, err := nodew.Query(); err != nil {
		w.Warn("failed to query node %q: %v", nodeName, err)
	} else if node == nil {
		w.Warn("failed to query node %q, make sure that NODE_NAME is correctly set", nodeName)
	} else {
		group = node.(*core_v1.Node).Labels[opts.labelName]
		w.Info("configuration group is set to '%s'", group)
	}

	cfgw := newConfigMapWatch(w, opts.configMapName+".node."+nodeName, namespace(opts.configNs))
	grpw := newConfigMapWatch(w, groupMapName(group), namespace(opts.configNs))

	w.Info("watcher running")
	w.sendConfig()

	for {
		select {
		case _ = <-w.stop:
			w.Info("stopping configuration watcher")
			nodew.Stop()
			cfgw.Stop()
			grpw.Stop()
			return nil

		case e, ok := <-nodew.ResultChan():
			if ok {
				switch e.Type {
				case k8swatch.Added, k8swatch.Modified:
					w.Info("node (%s) configuration updated", nodeName)
					label, _ := e.Object.(*core_v1.Node).Labels[opts.labelName]
					if group != label {
						group = label
						w.Info("configuration group is set to '%s'", group)
						grpw.Start(groupMapName(group))
					}
				case k8swatch.Deleted:
					w.Warn("Hmm, our node got removed...")
				}
				continue
			}

		case e, ok := <-cfgw.ResultChan():
			if ok {
				switch e.Type {
				case k8swatch.Added, k8swatch.Modified:
					w.Info("node ConfigMap updated")
					cm := e.Object.(*core_v1.ConfigMap)
					w.currentConfig.setNode(&cm.Data)
					w.sendConfig()

				case k8swatch.Deleted, SyntheticMissing:
					w.Info("node ConfigMap deleted")
					w.currentConfig.setNode(nil)
					w.sendConfig()
				}
				continue
			}

		case e, ok := <-grpw.ResultChan():
			if ok {
				switch e.Type {
				case k8swatch.Added, k8swatch.Modified:
					w.Info("group/default ConfigMap updated")
					cm := e.Object.(*core_v1.ConfigMap)
					if w.currentConfig.setGroup(group, &cm.Data) {
						w.sendConfig()
					}
				case k8swatch.Deleted, SyntheticMissing:
					w.Info("group/default ConfigMap deleted")
					if w.currentConfig.setGroup(group, nil) {
						w.sendConfig()
					}
				}
				continue
			}
		}

		// shouln't be necessary, but just in case avoid spinning on a closed channel
		time.Sleep(1 * time.Second)
	}
}

// groupMapName returns the our group ConfigMap, or the default one is we have no group.
func groupMapName(group string) string {
	if group == "" {
		return opts.configMapName + ".default"
	}
	return opts.configMapName + ".group." + group
}

// get is a helper method for getting the config data
func (c *cachedConfig) get() (resmgrConfig, string) {
	c.RLock()
	defer c.RUnlock()

	var cfg *resmgrConfig
	var kind string

	switch {
	case c.nodeCfg != nil:
		kind = "node"
		cfg = c.nodeCfg
	case c.group != "":
		kind = "group " + c.group
		cfg = c.groupCfg
	case c.groupCfg != nil:
		kind = "default"
		cfg = c.groupCfg
	default:
		kind = "fallback"
	}

	if cfg == nil {
		kind = "empty " + kind
		cfg = &resmgrConfig{}
	}

	return *cfg, kind
}

// set node-specific configuration
func (c *cachedConfig) setNode(data *map[string]string) bool {
	c.Lock()
	defer c.Unlock()

	c.nodeCfg = (*resmgrConfig)(data)
	return true
}

// set group-specific or default configuration
func (c *cachedConfig) setGroup(group string, data *map[string]string) bool {
	c.Lock()
	defer c.Unlock()

	c.groupCfg = (*resmgrConfig)(data)
	c.group = group
	return c.nodeCfg == nil
}
