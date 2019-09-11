// Copyright 2019 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cache

import (
	"encoding/json"
	"strconv"

	"k8s.io/api/core/v1"

	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
)

const (
	// KeyResourceAnnotation is the annotation key our webhook uses.
	KeyResourceAnnotation = "intel.com/resources"
)

// Create a pod from a run request.
func (p *pod) fromRunRequest(req *cri.RunPodSandboxRequest) error {
	cfg := req.Config
	if cfg == nil {
		return cacheError("pod %s has no config", p.Id)
	}
	meta := cfg.Metadata
	if meta == nil {
		return cacheError("pod %s has no request metadata", p.Id)
	}

	p.Name = meta.Name
	p.Namespace = meta.Namespace
	p.State = PodState(int32(PodStateReady))
	p.Labels = cfg.Labels
	p.Annotations = cfg.Annotations
	p.CgroupParent = cfg.GetLinux().GetCgroupParent()

	p.parseResourceAnnotations()
	p.extractLabels()

	return nil
}

// Create a pod from a list response.
func (p *pod) fromListResponse(pod *cri.PodSandbox) error {
	meta := pod.Metadata
	if meta == nil {
		return cacheError("pod %s has no reply metadata", p.Id)
	}

	p.Name = meta.Name
	p.Namespace = meta.Namespace
	p.State = PodState(int32(pod.State))
	p.Labels = pod.Labels
	p.Annotations = pod.Annotations

	p.parseResourceAnnotations()
	p.extractLabels()

	return nil
}

// Get the init containers of a pod.
func (p *pod) GetInitContainers() []Container {
	if p.Resources == nil {
		return nil
	}

	containers := []Container{}

	for _, c := range p.cache.Containers {
		if _, ok := p.Resources.InitContainers[c.Id]; ok {
			containers = append(containers, c)
		}
	}

	return containers
}

// Get the normal containers of a pod.
func (p *pod) GetContainers() []Container {
	containers := []Container{}

	for _, c := range p.cache.Containers {
		if p.Resources != nil {
			if _, ok := p.Resources.Containers[c.Id]; !ok {
				continue
			}
		}

		containers = append(containers, c)
	}

	return containers
}

// Get the id of a pod.
func (p *pod) GetId() string {
	return p.Id
}

// Get the (k8s) unique id of a pod.
func (p *pod) GetUid() string {
	return p.Uid
}

// Get the name of a pod.
func (p *pod) GetName() string {
	return p.Name
}

// Get the namespace of a pod.
func (p *pod) GetNamespace() string {
	return p.Namespace
}

// Get the PodState of a pod.
func (p *pod) GetState() PodState {
	return p.State
}

// Get the keys of all labels of a pod.
func (p *pod) GetLabelKeys() []string {
	keys := make([]string, len(p.Labels))

	idx := 0
	for key := range p.Labels {
		keys[idx] = key
		idx++
	}

	return keys
}

// Get the label for a key of a pod.
func (p *pod) GetLabel(key string) (string, bool) {
	value, ok := p.Labels[key]
	return value, ok
}

// Get all label keys in the cri-resource-manager namespace.
func (p *pod) GetResmgrLabelKeys() []string {
	return keysInNamespace(&p.Labels, kubernetes.ResmgrKeyNamespace)
}

// Get the label for the given key in the cri-resource-manager namespace.
func (p *pod) GetResmgrLabel(key string) (string, bool) {
	value, ok := p.Labels[kubernetes.ResmgrKey(key)]
	return value, ok
}

// Get the keys of all annotations of a pod.
func (p *pod) GetAnnotationKeys() []string {
	keys := make([]string, len(p.Annotations))

	idx := 0
	for key := range p.Annotations {
		keys[idx] = key
		idx++
	}

	return keys
}

// Get pod annotation for the given key.
func (p *pod) GetAnnotation(key string) (string, bool) {
	value, ok := p.Annotations[key]
	return value, ok
}

// Get and decode/unmarshal pod annotation for the given key.
func (p *pod) GetAnnotationObject(key string, objPtr interface{},
	decode func([]byte, interface{}) error) (bool, error) {
	var err error

	value, ok := p.GetAnnotation(key)
	if !ok {
		return false, nil
	}

	// decode with decoder function, if given
	if decode != nil {
		err = decode([]byte(value), objPtr)
		return true, err
	}

	// decode with type-specific default decoder
	switch objPtr.(type) {
	case *string:
		*objPtr.(*string) = value
	case *bool:
		*objPtr.(*bool), err = strconv.ParseBool(value)
	case *int:
		var i int64
		i, err = strconv.ParseInt(value, 0, 0)
		*objPtr.(*int) = int(i)
	case *uint:
		var i uint64
		i, err = strconv.ParseUint(value, 0, 0)
		*objPtr.(*uint) = uint(i)
	case *int64:
		*objPtr.(*int64), err = strconv.ParseInt(value, 0, 64)
	case *uint64:
		*objPtr.(*uint64), err = strconv.ParseUint(value, 0, 64)
	default:
		err = json.Unmarshal([]byte(value), objPtr)
	}

	if err != nil {
		p.cache.Error("failed to decode annotation %s (%s): %v", key, value, err)
	}

	return true, err
}

// Get the keys of all annotation in the cri-resource-manager namespace.
func (p *pod) GetResmgrAnnotationKeys() []string {
	return keysInNamespace(&p.Annotations, kubernetes.ResmgrKeyNamespace)
}

// Get the value of the given annotation in the cri-resource-manager namespace.
func (p *pod) GetResmgrAnnotation(key string) (string, bool) {
	return p.GetAnnotation(kubernetes.ResmgrKey(key))
}

// Get and decode the pod annotation for the key in the cri-resource-manager namespace..
func (p *pod) GetResmgrAnnotationObject(key string, objPtr interface{},
	decode func([]byte, interface{}) error) (bool, error) {
	return p.GetAnnotationObject(kubernetes.ResmgrKey(key), objPtr, decode)
}

// Get the cgroup parent directory of a pod, if known.
func (p *pod) GetCgroupParentDir() string {
	return p.CgroupParent
}

// Get the resource requirements of a pod.
func (p *pod) GetPodResourceRequirements() PodResourceRequirements {
	if p.Resources == nil {
		return PodResourceRequirements{}
	}

	return *p.Resources
}

// Extract oft-used data (currently only k8s uid) from pod labels.
func (p *pod) extractLabels() {
	uid, ok := p.GetLabel(kubetypes.KubernetesPodUIDLabel)
	if !ok {
		p.cache.Warn("can't find (k8s) uid label for pod %s", p.Id)
	}
	p.Uid = uid
}

// Parse per container resource requirements from webhook annotations.
func (p *pod) parseResourceAnnotations() {
	p.Resources = &PodResourceRequirements{}
	p.GetAnnotationObject(KeyResourceAnnotation, p.Resources, nil)
}

// Determine the QoS class of the pod.
func (p *pod) GetQOSClass() v1.PodQOSClass {
	if p.QOSClass == "" {
		p.QOSClass = cgroupParentToQOS(p.CgroupParent)
	}

	if p.QOSClass == "" {
		p.QOSClass = resourcesToQOS(p.Resources)
	}

	return p.QOSClass
}
