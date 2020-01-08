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
	"encoding/json"
	"fmt"
	"os"
	"time"

	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	agent_v1 "github.com/intel/cri-resource-manager/pkg/agent/api/v1"
)

type namespace string

// nodeName contains the name of the k8s we're running on
var nodeName string

// getK8sClient initializes a new Kubernetes client
func (a *agent) getK8sClient(kubeconfig string) (*k8sclient.Clientset, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		a.Info("using in-cluster kubeconfig")
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		return nil, err
	}

	return k8sclient.NewForConfig(config)
}

// getNodeObject gets a k8s Node object
func getNodeObject(cli *k8sclient.Clientset) (*core_v1.Node, error) {
	node, err := cli.CoreV1().Nodes().Get(nodeName, meta_v1.GetOptions{})
	if err != nil {
		return nil, agentError("failed to get node object for node %q: %v", nodeName, err)
	}
	return node, nil
}

// patchNodeObject is a helper for patching a k8s Node object
func patchNode(cli *k8sclient.Clientset, patchList []*agent_v1.JsonPatch) error {
	// Convert patch list into bytes
	data, err := json.Marshal(patchList)
	if err != nil {
		return agentError("failed to marshal Node patches: %v", err)
	}

	// Patch our node
	pt := types.JSONPatchType
	_, err = cli.CoreV1().Nodes().Patch(nodeName, pt, data)
	if err != nil {
		return err
	}
	return nil
}

// patchNodeStatus is a helper for patching the status of a k8s Node object
func patchNodeStatus(cli *k8sclient.Clientset, fields map[string]string) error {
	patch, sep := fmt.Sprintf(`{"status": {`), ""
	for f, v := range fields {
		patch += sep + fmt.Sprintf(`"%s": %s`, f, v)
		sep = ","
	}
	patch += "}}"

	_, err := cli.CoreV1().Nodes().PatchStatus(nodeName, []byte(patch))

	return err
}

// watch is a wrapper around the k8s watch.Interface
type watch struct {
	parent  *watcher
	kind    string
	ns      namespace
	name    string
	openfn  func(namespace, string) (k8swatch.Interface, error)
	queryfn func(namespace, string) (interface{}, error)
	stop    chan struct{}
	events  chan k8swatch.Event
}

// openFn is the type for functions creating k8s watcher of a particular kind.
type openFn func(ns namespace, name string) (k8swatch.Interface, error)

// queryFn is the type for functions querying k8s objects being watched.
type queryFn func(ns namespace, name string) (interface{}, error)

const (
	// SyntheticMissing is a synthetic initial event for currently non-existent object.
	SyntheticMissing = k8swatch.EventType("SyntheticMissing")
)

func newWatch(parent *watcher, kind string, ns namespace, open openFn, query queryFn) *watch {
	return &watch{
		parent:  parent,
		kind:    kind,
		ns:      ns,
		stop:    make(chan struct{}),
		events:  make(chan k8swatch.Event),
		openfn:  open,
		queryfn: query,
	}
}

// newNodeWatch creates a watch for k8s Node
func newNodeWatch(parent *watcher) *watch {
	w := newWatch(parent, "Node", namespace(""),
		func(ns namespace, name string) (k8swatch.Interface, error) {
			selector := meta_v1.ListOptions{FieldSelector: "metadata.name=" + name}
			k8w, err := parent.k8sCli.CoreV1().Nodes().Watch(selector)
			if err != nil {
				return nil, err
			}
			return k8w, nil
		},
		func(ns namespace, name string) (interface{}, error) {
			noopts := meta_v1.GetOptions{}
			node, err := parent.k8sCli.CoreV1().Nodes().Get(name, noopts)
			if err != nil {
				return nil, err
			}
			return node, nil
		})
	w.Start(nodeName)
	return w
}

// newConfigMapWatch creates a watch for k8s ConfigMap
func newConfigMapWatch(parent *watcher, name string, ns namespace) *watch {
	w := newWatch(parent, "ConfigMap", ns,
		func(ns namespace, name string) (k8swatch.Interface, error) {
			selector := meta_v1.ListOptions{FieldSelector: "metadata.name=" + name}
			k8w, err := parent.k8sCli.CoreV1().ConfigMaps(string(ns)).Watch(selector)
			if err != nil {
				return nil, err
			}
			return k8w, nil
		},
		func(ns namespace, name string) (interface{}, error) {
			noopts := meta_v1.GetOptions{}
			cm, err := parent.k8sCli.CoreV1().ConfigMaps(string(ns)).Get(name, noopts)
			if err != nil {
				return nil, err
			}
			return cm, nil
		})
	w.Start(name)
	return w
}

func (w *watch) Name() string {
	ns, name := w.ns, w.name
	if ns != "" {
		ns += "/"
	}
	if name == "" {
		name = "<none>"
	}
	return w.kind + ":" + string(ns) + name
}

// Query queries the object being watched.
func (w *watch) Query() (interface{}, error) {
	if w.name == "" {
		return nil, nil
	}
	return w.queryfn(w.ns, w.name)
}

// Start watching an object.
func (w *watch) Start(name string) {
	w.Stop()
	w.name = name

	if w.name == "" {
		return
	}

	// proxy events from a go-routing until we're told to stop.
	go func() {
		var k8w k8swatch.Interface
		var events <-chan k8swatch.Event
		var ratelimit <-chan time.Time
		var err error

		// let the watcher know not to expect initial event
		if _, err = w.queryfn(w.ns, w.name); err != nil {
			w.events <- k8swatch.Event{Type: SyntheticMissing}
		}

		for {
			if events == nil {
				w.parent.Info("creating %s watch", w.Name())
				if k8w, err = w.openfn(w.ns, w.name); err != nil {
					w.parent.Warn("failed to create %s watch: %v", w.Name(), err)
					ratelimit = time.After(1 * time.Second)
				} else {
					events = k8w.ResultChan()
					ratelimit = nil
				}
			}

			select {
			case _ = <-w.stop:
				if events != nil {
					k8w.Stop()
				}
				return
			case e, ok := <-events:
				if ok {
					w.events <- e
				} else {
					k8w.Stop()
					events = nil
				}
			case _ = <-ratelimit:
			}
		}
	}()
}

// Close closes a watch.
func (w *watch) Stop() {
	select {
	case w.stop <- struct{}{}:
	default:
	}
}

// ResultChan returns the event channel of the watch.
func (w *watch) ResultChan() <-chan k8swatch.Event {
	return w.events
}

func init() {
	// Node name is expected to be set in an environment variable
	nodeName = os.Getenv("NODE_NAME")
}
