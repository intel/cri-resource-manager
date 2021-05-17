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
	"strings"

	nri "github.com/containerd/nri/v2alpha1/pkg/api"
	v1 "k8s.io/api/core/v1"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/intel/cri-resource-manager/pkg/apis/resmgr"
	"github.com/intel/cri-resource-manager/pkg/cgroups"
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
		return cacheError("pod %s has no config", p.ID)
	}
	meta := cfg.Metadata
	if meta == nil {
		return cacheError("pod %s has no request metadata", p.ID)
	}

	p.containers = make(map[string]string)
	p.UID = meta.Uid
	p.Name = meta.Name
	p.Namespace = meta.Namespace
	p.State = PodState(int32(PodStateReady))
	p.Labels = cfg.Labels
	p.Annotations = cfg.Annotations
	p.CgroupParent = cfg.GetLinux().GetCgroupParent()

	if err := p.discoverQOSClass(); err != nil {
		p.cache.Error("%v", err)
	}

	p.parseResourceAnnotations()

	return nil
}

// Create a pod from a list response.
func (p *pod) fromListResponse(pod *cri.PodSandbox, status *PodStatus) error {
	meta := pod.Metadata
	if meta == nil {
		return cacheError("pod %s has no reply metadata", p.ID)
	}

	p.containers = make(map[string]string)
	p.UID = meta.Uid
	p.Name = meta.Name
	p.Namespace = meta.Namespace
	p.State = PodState(int32(pod.State))
	p.Labels = pod.Labels
	p.Annotations = pod.Annotations
	p.CgroupParent = status.CgroupParent

	if err := p.discoverQOSClass(); err != nil {
		p.cache.Error("%v", err)
	}

	p.parseResourceAnnotations()

	return nil
}

// Create a pod from an NRI request.
func (p *pod) fromNRI(pod *nri.PodSandbox) error {
	p.containers = make(map[string]string)
	p.UID = pod.Uid
	p.Name = pod.Name
	p.Namespace = pod.Namespace
	p.State = PodState(int32(cri.PodSandboxState_SANDBOX_READY))
	p.Labels = pod.Labels
	p.Annotations = pod.Annotations
	p.CgroupParent = pod.CgroupParent

	if err := p.discoverQOSClass(); err != nil {
		p.cache.Error("%v", err)
	}

	p.parseResourceAnnotations()

	return nil
}

// Get the init containers of a pod.
func (p *pod) GetInitContainers() []Container {
	if p.Resources == nil {
		return nil
	}

	containers := []Container{}

	for id, c := range p.cache.Containers {
		if id != c.CacheID {
			continue
		}
		if _, ok := p.Resources.InitContainers[c.ID]; ok {
			containers = append(containers, c)
		}
	}

	return containers
}

// Get the normal containers of a pod.
func (p *pod) GetContainers() []Container {
	containers := []Container{}

	for id, c := range p.cache.Containers {
		if c.PodID != p.ID || id != c.CacheID {
			continue
		}
		if p.Resources != nil {
			if _, ok := p.Resources.InitContainers[c.ID]; ok {
				continue
			}
		}
		containers = append(containers, c)
	}

	return containers
}

// Get container pointer by its name.
func (p *pod) getContainer(name string) *container {
	var found *container

	if id, ok := p.containers[name]; ok {
		return p.cache.Containers[id]
	}

	for _, c := range p.GetContainers() {
		cptr := c.(*container)
		p.containers[cptr.Name] = cptr.ID
		if cptr.Name == name {
			found = cptr
		}
	}

	return found
}

// Get container by its name.
func (p *pod) GetContainer(name string) (Container, bool) {
	c := p.getContainer(name)

	return c, c != nil
}

// Get the id of a pod.
func (p *pod) GetID() string {
	return p.ID
}

// Get the (k8s) unique id of a pod.
func (p *pod) GetUID() string {
	return p.UID
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
	return keysInNamespace(p.Labels, kubernetes.ResmgrKeyNamespace)
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
	return keysInNamespace(p.Annotations, kubernetes.ResmgrKeyNamespace)
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

// Get the effective annotation for the container.
func (p *pod) GetEffectiveAnnotation(key, container string) (string, bool) {
	if v, ok := p.Annotations[key+"/container."+container]; ok {
		return v, true
	}
	if v, ok := p.Annotations[key+"/pod"]; ok {
		return v, true
	}
	v, ok := p.Annotations[key]
	return v, ok
}

// Get the cgroup parent directory of a pod, if known.
func (p *pod) GetCgroupParentDir() string {
	return p.CgroupParent
}

// discover a pod's QoS class by parsing the cgroup parent directory.
func (p *pod) discoverQOSClass() error {
	if p.CgroupParent == "" {
		return cacheError("%s: unknown cgroup parent ", p.ID)
	}

	dirs := strings.Split(p.CgroupParent[1:], "/")
	if len(dirs) < 1 {
		return cacheError("%s: failed to parse %q for QoS class",
			p.ID, p.CgroupParent)

	}

	// consume any potential --cgroup-root passed to kubelet
	if dirs[0] != "kubepods.slice" && dirs[0] != "kubepods" {
		dirs = dirs[1:]
	}
	if len(dirs) < 1 {
		return cacheError("%s: failed to parse %q for QoS class",
			p.ID, p.CgroupParent)
	}

	// consume potential kubepods[.slice]
	if dirs[0] == "kubepods.slice" || dirs[0] == "kubepods" {
		dirs = dirs[1:]
	}
	if len(dirs) < 1 {
		return cacheError("%s: failed to parse %q for QoS class",
			p.ID, p.CgroupParent)
	}

	// check for besteffort, burstable, or lack thereof indicating guaranteed
	switch dir := dirs[0]; {
	case dir == "kubepods-besteffort.slice" || dir == "besteffort":
		p.QOSClass = v1.PodQOSBestEffort
		return nil
	case dir == "kubepods-burstable.slice" || dir == "burstable":
		p.QOSClass = v1.PodQOSBurstable
		return nil
	case strings.HasPrefix(dir, "kubepods-pod") || strings.HasPrefix(dir, "pod"):
		p.QOSClass = v1.PodQOSGuaranteed
		return nil
	}

	return cacheError("%s: failed to parse %q for QoS class",
		p.ID, p.CgroupParent)
}

// Get the resource requirements of a pod.
func (p *pod) GetPodResourceRequirements() PodResourceRequirements {
	if p.Resources == nil {
		return PodResourceRequirements{}
	}

	return *p.Resources
}

// Parse per container resource requirements from webhook annotations.
func (p *pod) parseResourceAnnotations() {
	p.Resources = &PodResourceRequirements{}
	p.GetAnnotationObject(KeyResourceAnnotation, p.Resources, nil)
}

// Determine the QoS class of the pod.
func (p *pod) GetQOSClass() v1.PodQOSClass {
	return p.QOSClass
}

// GetContainerAffinity returns the annotated affinity for the named container.
func (p *pod) GetContainerAffinity(name string) ([]*Affinity, error) {
	if p.Affinity != nil {
		return (*p.Affinity)[name], nil
	}

	affinity := &podContainerAffinity{}

	value, ok := p.GetResmgrAnnotation(keyAffinity)
	if ok {
		weight := DefaultWeight
		if !affinity.parseSimple(p, value, weight) {
			if err := affinity.parseFull(p, value, weight); err != nil {
				p.cache.Error("%v", err)
				return nil, err
			}
		}
	}
	value, ok = p.GetResmgrAnnotation(keyAntiAffinity)
	if ok {
		weight := -DefaultWeight
		if !affinity.parseSimple(p, value, weight) {
			if err := affinity.parseFull(p, value, weight); err != nil {
				p.cache.Error("%v", err)
				return nil, err
			}
		}
	}

	if p.cache.DebugEnabled() {
		p.cache.Debug("Pod container affinity for %s:", p.GetName())
		for id, ca := range *affinity {
			p.cache.Debug("  - container %s:", id)
			for _, a := range ca {
				p.cache.Debug("    * %s", a.String())
			}
		}
	}

	p.Affinity = affinity

	return (*p.Affinity)[name], nil
}

// ScopeExpression returns an affinity expression for defining this pod as the scope.
func (p *pod) ScopeExpression() *resmgr.Expression {
	return &resmgr.Expression{
		//      Domain: LabelsDomain,
		Key:    kubernetes.PodNameLabel,
		Op:     resmgr.Equals,
		Values: []string{p.GetName()},
	}
}

// String returns a string representation of pod.
func (p *pod) String() string {
	return p.Name
}

// Eval returns the value of a key for expression evaluation.
func (p *pod) Eval(key string) interface{} {
	switch key {
	case resmgr.KeyName:
		return p.Name
	case resmgr.KeyNamespace:
		return p.Namespace
	case resmgr.KeyQOSClass:
		return p.GetQOSClass()
	case resmgr.KeyLabels:
		return p.Labels
	case resmgr.KeyID:
		return p.ID
	case resmgr.KeyUID:
		return p.UID
	default:
		return cacheError("Pod cannot evaluate of %q", key)
	}
}

// GetProcesses returns the pids of processes in a pod.
func (p *pod) GetProcesses(recursive bool) ([]string, error) {
	return p.getTasks(recursive, true)
}

// GetTasks returns the pids of threads in a pod.
func (p *pod) GetTasks(recursive bool) ([]string, error) {
	return p.getTasks(recursive, false)
}

// getTasks returns the pids of processes or threads in a pod.
func (p *pod) getTasks(recursive, processes bool) ([]string, error) {
	var pids, childPids []string
	var err error

	dir := p.GetCgroupParentDir()
	if dir == "" {
		return nil, cacheError("%s: unknown cgroup parent directory", p.Name)
	}

	if processes {
		pids, err = cgroups.Cpu.Group(dir).GetProcesses()
	} else {
		pids, err = cgroups.Cpu.Group(dir).GetTasks()
	}
	if err != nil {
		return nil, cacheError("%s: failed to read pids: %v", p.Name, err)
	}

	if !recursive {
		return pids, nil
	}

	for _, c := range append(p.GetInitContainers(), p.GetContainers()...) {
		if c.GetState() == ContainerStateRunning {
			if processes {
				childPids, err = c.GetProcesses()
			} else {
				childPids, err = c.GetTasks()
			}
			if err == nil {
				pids = append(pids, childPids...)
				continue
			}

			p.cache.Error("%s: failed to read pids of %s: %v", p.Name,
				c.PrettyName(), err)
		}
	}

	return pids, nil
}

// ParsePodStatus parses a PodSandboxStatusResponse into a PodStatus.
func ParsePodStatus(response *cri.PodSandboxStatusResponse) (*PodStatus, error) {
	var name string

	type infoRuntimeSpec struct {
		Annotations map[string]string `json:"annotations"`
	}
	type infoConfig struct {
		Linux *struct {
			CgroupParent string `json:"cgroup_parent"`
		} `json:"linux"`
	}
	type statusInfo struct {
		RuntimeSpec *infoRuntimeSpec `json:"runtimeSpec"`
		Config      *infoConfig      `json:"config"`
	}

	if response.Status.Metadata != nil {
		name = response.Status.Metadata.Name
	} else {
		name = response.Status.Id
	}

	blob, ok := response.Info["info"]
	if !ok {
		return nil, cacheError("%s: missing info in pod status response", name)
	}
	info := statusInfo{}
	if err := json.Unmarshal([]byte(blob), &info); err != nil {
		return nil, cacheError("%s: failed to extract pod status info: %v",
			name, err)
	}

	ps := &PodStatus{}

	if info.Config != nil { // containerd
		// CgroupParent: Info["config"]["linux"]["cgroup_parent"]
		ps.CgroupParent = info.Config.Linux.CgroupParent
	} else if info.RuntimeSpec != nil { // cri-o
		// CgroupParent: Info["info"]["runtimeSpec"]["annotations"][crioCgroupParent]
		const (
			crioCgroupParent = "io.kubernetes.cri-o.CgroupParent"
		)

		ps.CgroupParent = info.RuntimeSpec.Annotations[crioCgroupParent]
	}

	if ps.CgroupParent == "" {
		return nil, cacheError("%s: failed to extract cgroup parent from pod status",
			name)
	}

	return ps, nil
}
