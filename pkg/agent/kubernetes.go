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

	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	agent_v1 "github.com/intel/cri-resource-manager/pkg/agent/api/v1"
)

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

	//a.Debug("patching status of node with '%s'", patch)
	_, err := cli.CoreV1().Nodes().PatchStatus(nodeName, []byte(patch))

	return err
}

// watchConfigMap watches changes in a ConfigMap object
func watchConfigMap(cli *k8sclient.Clientset, ns string, name string) (watch.Interface, error) {
	listOpts := meta_v1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	}
	watch, err := cli.CoreV1().ConfigMaps(ns).Watch(listOpts)
	if err != nil {
		return nil, agentError("failed to create a watch for configmaps in ns %q: %v", ns, err)
	}

	return watch, nil
}

func init() {
	// Node name is expected to be set in an environment variable
	nodeName = os.Getenv("NODE_NAME")
}
