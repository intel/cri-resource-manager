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
	core_v1 "k8s.io/api/core/v1"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	k8sclient "k8s.io/client-go/kubernetes"
	"sync"
	"time"

	"encoding/json"
	patch "github.com/evanphx/json-patch"
	pkgtypes "k8s.io/apimachinery/pkg/types"

	resmgrcli "github.com/intel/cri-resource-manager/pkg/apis/resmgr/generated/clientset/versioned/typed/resmgr/v1alpha1"
	resmgr "github.com/intel/cri-resource-manager/pkg/apis/resmgr/v1alpha1"

	"github.com/intel/cri-resource-manager/pkg/log"
)

type cachedConfig struct {
	sync.RWMutex
	nodeCfg  *resmgrConfig    // node-specific configuration
	groupCfg *resmgrConfig    // group-specific configuration
	group    string           // group name, "" for default
	inscope  resmgrAdjustment // external adjustments that apply to this node
	ignored  resmgrAdjustment // external adjustments that do not apply to this node
	status   *resmgrStatus    // latest adjustment update status
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
	// Get a chan through which to receive adjustment updates
	AdjustmentChan() <-chan resmgrAdjustment
	// Update the node Status for adjustment updates.
	UpdateStatus(*resmgrStatus) error
}

// watcher implements k8sWatcher
type watcher struct {
	log.Logger
	stop           chan struct{}                      // channel to stop watcher goroutine
	k8sCli         *k8sclient.Clientset               // k8s client interface
	resmgrCli      *resmgrcli.CriresmgrV1alpha1Client // adjustment CRD interface
	currentConfig  cachedConfig                       // current configuration, cached
	configChan     chan resmgrConfig                  // channel for config updates
	adjustmentChan chan resmgrAdjustment              // channel for adjustment updates
}

// newK8sWatcher creates a new K8sWatcher instance
func newK8sWatcher(k8sCli *k8sclient.Clientset, resmgrCli *resmgrcli.CriresmgrV1alpha1Client) (k8sWatcher, error) {
	w := &watcher{
		Logger:         log.NewLogger("watcher"),
		k8sCli:         k8sCli,
		resmgrCli:      resmgrCli,
		stop:           make(chan struct{}, 1),
		currentConfig:  newCachedConfig(),
		configChan:     make(chan resmgrConfig, 1),
		adjustmentChan: make(chan resmgrAdjustment, 1),
	}

	return w, nil
}

// Start runs a k8sWatcher instance
func (w *watcher) Start() error {
	w.Info("starting watcher...")
	if nodeName == "" {
		return agentError("node name not set, NODE_NAME env variable should be set to match the name of this k8s Node")
	}

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

// AdjustmentChan returns the chan for adjustment updates
func (w *watcher) AdjustmentChan() <-chan resmgrAdjustment {
	return w.adjustmentChan
}

// GetConfig returns the current cri-resmgr configuration
func (w *watcher) GetConfig() resmgrConfig {
	cfg, kind := w.currentConfig.getConfig()
	w.Info("giving %s configuration in reply to query", kind)
	return cfg
}

// UpdateStatus updates the node status for adjustment updates.
func (w *watcher) UpdateStatus(status *resmgrStatus) error {
	w.currentConfig.setStatus(status)
	return w.PatchAdjustmentStatus(status)
}

// PatchAdjustmentStatus updates the node status for adjustment updates.
func (w *watcher) PatchAdjustmentStatus(status *resmgrStatus) error {
	errors := status.errors
	if errors == nil {
		errors = map[string]string{}
	}
	if status.request != nil {
		errors["request"] = status.request.Error()
	}

	inscope, ignored := w.currentConfig.getAdjustment()

	w.currentConfig.Lock()
	defer w.currentConfig.Unlock()

	errCnt := 0
	for _, adjust := range inscope {
		if err := w.patchAdjustment(adjust, true, errors); err != nil {
			w.Error("%v", err)
			errCnt++
		}
	}
	for _, adjust := range ignored {
		if err := w.patchAdjustment(adjust, false, errors); err != nil {
			w.Error("%v", err)
			errCnt++
		}
	}
	if errCnt > 0 {
		return agentError("some adjustment status updates failed")
	}

	return nil
}

// patchAdjustment patches the status of an update to the given adjustment.
func (w *watcher) patchAdjustment(adjust *resmgr.Adjustment, inscope bool, errors map[string]string) error {
	var pdata []byte
	var err error

	old, ok := adjust.Status.Nodes[nodeName]

	if !inscope {
		if !ok {
			w.Debug("adjustment %s does not need status patching...", adjust.Name)
			return nil
		}
		current := &resmgr.Adjustment{
			Status: resmgr.AdjustmentStatus{
				Nodes: map[string]resmgr.AdjustmentNodeStatus{
					nodeName: old,
				},
			},
		}
		updated := &resmgr.Adjustment{
			Status: resmgr.AdjustmentStatus{
				Nodes: map[string]resmgr.AdjustmentNodeStatus{},
			},
		}
		oldData, _ := json.Marshal(current)
		newData, _ := json.Marshal(updated)
		pdata, err = patch.CreateMergePatch(oldData, newData)
		if err != nil {
			return agentError("failed to adjustment status patch: %v", err)
		}
	} else {
		current := &resmgr.Adjustment{
			Status: resmgr.AdjustmentStatus{
				Nodes: map[string]resmgr.AdjustmentNodeStatus{},
			},
		}
		if ok {
			current.Status.Nodes[nodeName] = old
		}
		updated := &resmgr.Adjustment{
			Status: resmgr.AdjustmentStatus{
				Nodes: map[string]resmgr.AdjustmentNodeStatus{
					nodeName: {Errors: errors},
				},
			},
		}
		oldData, _ := json.Marshal(current)
		newData, _ := json.Marshal(updated)
		pdata, err = patch.CreateMergePatch(oldData, newData)
		if err != nil {
			return agentError("failed to adjustment status patch: %v", err)
		}
	}

	ptype := pkgtypes.MergePatchType

	w.Debug("patching status of adjustment %s status with %v...", adjust.Name, string(pdata))

	if _, err := w.resmgrCli.Adjustments(opts.configNs).Patch(adjust.Name, ptype, pdata); err != nil {
		return agentError("failed to patch Adjustment CRD %q: %v", adjust.Name, err)
	}

	if inscope {
		if adjust.Status.Nodes == nil {
			adjust.Status.Nodes = make(map[string]resmgr.AdjustmentNodeStatus)
		}
		adjust.Status.Nodes[nodeName] = resmgr.AdjustmentNodeStatus{Errors: errors}
	} else {
		delete(adjust.Status.Nodes, nodeName)
	}

	return nil
}

// sendConfig sends the current configuration.
func (w *watcher) sendConfig() {
	cfg, kind := w.currentConfig.getConfig()
	w.Info("pushing %s configuration to client", kind)
	w.configChan <- cfg
}

// sendAdjustment sends the current overridden policies.
func (w *watcher) sendAdjustment() {
	inscope, _ := w.currentConfig.getAdjustment()
	w.adjustmentChan <- inscope
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
	crdw := newAdjustmentCRDWatch(w, namespace(opts.configNs))

	w.Info("watcher running")
	w.sendConfig()

	for {
		select {
		case _ = <-w.stop:
			w.Info("stopping configuration watcher")
			nodew.Stop()
			cfgw.Stop()
			grpw.Stop()
			crdw.Stop()
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

		case e, ok := <-crdw.ResultChan():
			if ok {
				switch e.Type {
				case k8swatch.Added, k8swatch.Modified:
					w.Info("Adjustment CRD(s) updated: %T, %+v", e.Object, e.Object)
					w.Info("Adjustment CRD(s): %+v", e.Object.(*resmgr.Adjustment).Spec)
					if w.currentConfig.setAdjustment(e.Object.(*resmgr.Adjustment)) {
						w.sendAdjustment()
					}

				case k8swatch.Deleted:
					w.Info("Adjustment CRD(s) (%T) deleted", e.Object)
					if w.currentConfig.deleteAdjustment(e.Object.(*resmgr.Adjustment)) {
						w.sendAdjustment()
					}

				case SyntheticMissing:
					w.Info("No Adjustment CRD(s)")
					w.sendAdjustment()
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

// newCacheConfig creates a new cachedConfig instance.
func newCachedConfig() cachedConfig {
	return cachedConfig{
		inscope: resmgrAdjustment{},
		ignored: resmgrAdjustment{},
	}
}

// getConfig is a helper method for getting the config data
func (c *cachedConfig) getConfig() (resmgrConfig, string) {
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

// getAdjustment is a helper method for getting a copy of external adjustments
func (c *cachedConfig) getAdjustment() (resmgrAdjustment, resmgrAdjustment) {
	c.RLock()
	defer c.RUnlock()

	inscope := resmgrAdjustment{}
	for name, value := range c.inscope {
		inscope[name] = value
	}
	ignored := resmgrAdjustment{}
	for name, value := range c.ignored {
		ignored[name] = value
	}

	return inscope, ignored
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

// setAdjustment is a helper method for updating external adjustments
func (c *cachedConfig) setAdjustment(adjust *resmgr.Adjustment) bool {
	var inscope, ignored bool
	var updated *resmgr.Adjustment

	c.Lock()
	defer c.Unlock()

	//
	// filter out updates
	//   - for expired watches being recreated
	//   - without any Spec changes (Status updates)
	//

	if updated, inscope = c.inscope[adjust.Name]; inscope {
		if adjust.HasSameVersion(updated) || adjust.Spec.Compare(&updated.Spec) {
			c.inscope[adjust.Name] = adjust
			return false
		}
	} else if updated, ignored = c.ignored[adjust.Name]; ignored {
		if adjust.HasSameVersion(updated) || adjust.Spec.Compare(&updated.Spec) {
			c.ignored[adjust.Name] = adjust
			return false
		}
	}

	//
	// we need to notify cri-resmgr if
	//   - the adjustment applies to this node
	//   - the adjustment used to apply to this node before the update
	//

	notify := false
	if adjust.Spec.IsNodeInScope(nodeName) {
		c.inscope[adjust.Name] = adjust
		if ignored {
			delete(c.ignored, adjust.Name)
		}
		notify = true
	} else {
		c.ignored[adjust.Name] = adjust
		if inscope {
			delete(c.inscope, adjust.Name)
			notify = true
		}
	}

	return notify
}

// deleteAdjustment is a helper method for updating external adjustments
func (c *cachedConfig) deleteAdjustment(o *resmgr.Adjustment) bool {
	c.Lock()
	defer c.Unlock()

	// we need to notify cri-resmgr if the deleted adjustment used to apply to this node
	if _, ok := c.inscope[o.Name]; ok {
		delete(c.inscope, o.Name)
		return true
	}

	delete(c.ignored, o.Name)
	return false
}

// getAdjustmentNames returns the names of in scope and ignored adjustments.
func (c *cachedConfig) getAdjustmentNames() ([]string, []string) {
	c.RLock()
	defer c.RUnlock()

	inscope := make([]string, 0, len(c.inscope))
	ignored := make([]string, 0, len(c.ignored))
	for name := range c.inscope {
		inscope = append(inscope, name)
	}
	for name := range c.ignored {
		ignored = append(ignored, name)
	}
	return inscope, ignored
}

// cache the status of the last adjustment update
func (c *cachedConfig) setStatus(status *resmgrStatus) {
	c.Lock()
	defer c.Unlock()
	c.status = status
}

// get the last cached adjustment update status
func (c *cachedConfig) getStatus() *resmgrStatus {
	c.RLock()
	defer c.RUnlock()
	return c.status
}
