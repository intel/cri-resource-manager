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

package kubernetes

import (
	core "k8s.io/kubernetes/pkg/apis/core"
	kubelet "k8s.io/kubernetes/pkg/kubelet/types"
)

const (
	// ResmgrKeyNamespace is a CRI Resource Manager namespace
	ResmgrKeyNamespace = "cri-resource-manager.intel.com"

	// NamespaceSystem is the kubernetes system namespace.
	NamespaceSystem = core.NamespaceSystem
	// PodNameLabel is the label key for the kubernetes pod name.
	PodNameLabel = kubelet.KubernetesPodNameLabel
	// ContainerNameLabel is the label key for the kubernetes container name.
	ContainerNameLabel = kubelet.KubernetesContainerNameLabel
)

// ResmgrKey returns a full namespaced name of a resource manager specific key
func ResmgrKey(name string) string {
	return ResmgrKeyNamespace + "/" + name
}
