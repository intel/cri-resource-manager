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
	"regexp"
	"sort"
	"strings"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	"github.com/intel/cri-resource-manager/pkg/sysfs"

	v1 "k8s.io/api/core/v1"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Create a container for a create request.
func (c *container) fromCreateRequest(req *cri.CreateContainerRequest) error {
	c.PodID = req.PodSandboxId

	cfg := req.Config
	if cfg == nil {
		return cacheError("container of pod %s has no config", c.PodID)
	}
	meta := cfg.Metadata
	if meta == nil {
		return cacheError("container of pod %s has no request metadata", c.PodID)
	}
	podCfg := req.SandboxConfig
	if podCfg == nil {
		return cacheError("container of pod %s has no request pod config data", c.PodID)
	}
	podMeta := podCfg.Metadata
	if podMeta == nil {
		return cacheError("container of pod %s has no request pod metadata", c.PodID)
	}

	c.Name = meta.Name
	c.Namespace = podMeta.Namespace
	c.State = ContainerStateCreating
	c.Image = cfg.GetImage().GetImage()
	c.Command = cfg.Command
	c.Args = cfg.Args
	c.Labels = cfg.Labels
	c.Annotations = cfg.Annotations

	c.Env = make(map[string]string)
	for _, kv := range cfg.Envs {
		c.Env[kv.Key] = kv.Value
	}

	c.Mounts = make(map[string]*Mount)
	for _, m := range cfg.Mounts {
		c.Mounts[m.ContainerPath] = &Mount{
			Container:   m.ContainerPath,
			Host:        m.HostPath,
			Readonly:    m.Readonly,
			Relabel:     m.SelinuxRelabel,
			Propagation: MountType(m.Propagation),
		}
		if hints := getTopologyHints(m.HostPath, m.ContainerPath, m.Readonly); len(hints) > 0 {
			c.TopologyHints = sysfs.MergeTopologyHints(c.TopologyHints, hints)
		}
	}

	c.Devices = make(map[string]*Device)
	for _, d := range cfg.Devices {
		c.Devices[d.ContainerPath] = &Device{
			Container:   d.ContainerPath,
			Host:        d.HostPath,
			Permissions: d.Permissions,
		}
		if hints := getTopologyHints(d.HostPath, d.ContainerPath, strings.IndexAny(d.Permissions, "wm") == -1); len(hints) > 0 {
			c.TopologyHints = sysfs.MergeTopologyHints(c.TopologyHints, hints)
		}
	}

	// if we get more than one hint, check that there are no duplicates
	// if len(c.TopologyHints) > 1 {
	// 	c.TopologyHints = sysfs.DeDuplicateTopologyHints(c.TopologyHints)
	// }

	c.LinuxReq = cfg.GetLinux().GetResources()

	if p, _ := c.cache.Pods[c.PodID]; p != nil && p.Resources != nil {
		if r, ok := p.Resources.InitContainers[c.Name]; ok {
			c.Resources = r
		} else if r, ok := p.Resources.Containers[c.Name]; ok {
			c.Resources = r
		}
	}

	if len(c.Resources.Requests) == 0 && len(c.Resources.Limits) == 0 {
		c.Resources = estimateComputeResources(c.LinuxReq)
	}

	c.TopologyHints = sysfs.MergeTopologyHints(c.TopologyHints, getKubeletHint(c.GetCpusetCpus(), c.GetCpusetMems()))

	return nil
}

// Create container from a container list response.
func (c *container) fromListResponse(lrc *cri.Container) error {
	c.PodID = lrc.PodSandboxId

	p, _ := c.cache.Pods[c.PodID]
	if p == nil {
		return cacheError("can't find cached pod %s for listed container", c.PodID)
	}

	meta := lrc.Metadata
	if meta == nil {
		return cacheError("listed container of pod %s has no metadata", c.PodID)
	}

	c.ID = lrc.Id
	c.Name = meta.Name
	c.Namespace = p.Namespace
	c.State = ContainerState(int32(lrc.State))
	c.Image = lrc.GetImage().GetImage()
	c.Labels = lrc.Labels
	c.Annotations = lrc.Annotations

	if p.Resources != nil {
		if r, ok := p.Resources.InitContainers[c.Name]; ok {
			c.Resources = r
		} else if r, ok := p.Resources.Containers[c.Name]; ok {
			c.Resources = r
		}
	}

	return nil
}

// UpdateCriCreateRequest updates a CRI ContainerCreateRequest for the container.
func (c *container) UpdateCriCreateRequest(req *cri.CreateContainerRequest) error {
	if c.State != ContainerStateCreating || c.ID != "" {
		c.cache.Warn("hmm... cache thinks container (%v/%v) being created exists",
			c.CacheID, c.ID)
	}

	req.Config.Command = c.Command
	req.Config.Args = c.Args
	req.Config.Labels = c.Labels
	req.Config.Annotations = c.Annotations

	req.Config.Envs = make([]*cri.KeyValue, len(c.Env))
	idx := 0
	for k, v := range c.Env {
		req.Config.Envs[idx] = &cri.KeyValue{
			Key:   k,
			Value: v,
		}
		idx++
	}

	req.Config.Mounts = make([]*cri.Mount, len(c.Mounts))
	idx = 0
	for _, m := range c.Mounts {
		req.Config.Mounts[idx] = &cri.Mount{
			ContainerPath:  m.Container,
			HostPath:       m.Host,
			Readonly:       m.Readonly,
			SelinuxRelabel: m.Relabel,
			Propagation:    cri.MountPropagation(m.Propagation),
		}
		idx++
	}

	req.Config.Devices = make([]*cri.Device, len(c.Devices))
	idx = 0
	for _, d := range c.Devices {
		req.Config.Devices[idx] = &cri.Device{
			ContainerPath: d.Container,
			HostPath:      d.Host,
			Permissions:   d.Permissions,
		}
		idx++
	}

	req.Config.Linux.Resources = c.LinuxReq

	return nil
}

// CriUpdateRequest creates a CRI UpdateContainerResourcesRequest for the container.
func (c *container) CriUpdateRequest() (*cri.UpdateContainerResourcesRequest, error) {
	if c.ID == "" {
		return nil, cacheError("can't udpate container %s, not created yet", c.CacheID)
	}

	if c.LinuxReq == nil {
		return nil, nil
	}

	return &cri.UpdateContainerResourcesRequest{
		ContainerId: c.ID,
		Linux:       &(*c.LinuxReq),
	}, nil
}

func (c *container) PrettyName() string {
	pod, ok := c.GetPod()
	if !ok {
		return c.PodID + ":" + c.Name
	}
	return pod.GetName() + ":" + c.Name
}

func (c *container) GetPod() (Pod, bool) {
	pod, found := c.cache.Pods[c.PodID]
	return pod, found
}

func (c *container) GetID() string {
	return c.ID
}

func (c *container) GetPodID() string {
	return c.PodID
}

func (c *container) GetCacheID() string {
	return c.CacheID
}

func (c *container) GetName() string {
	return c.Name
}

func (c *container) GetNamespace() string {
	return c.Namespace
}

func (c *container) UpdateState(state ContainerState) {
	c.State = state
}

func (c *container) GetState() ContainerState {
	return c.State
}

func (c *container) GetQOSClass() v1.PodQOSClass {
	var qos v1.PodQOSClass

	if pod, found := c.GetPod(); found {
		qos = pod.GetQOSClass()
	}

	return qos
}

func (c *container) GetImage() string {
	return c.Image
}

func (c *container) GetCommand() []string {
	command := make([]string, len(c.Command))
	copy(command, c.Command)
	return command
}

func (c *container) GetArgs() []string {
	args := make([]string, len(c.Args))
	copy(args, c.Args)
	return args
}

func keysInNamespace(m *map[string]string, namespace string) []string {
	keys := make([]string, 0, len(*m))

	for key := range *m {
		split := strings.SplitN(key, "/", 2)
		if len(split) == 2 && split[0] == namespace {
			keys = append(keys, split[1])
		} else if len(split) == 1 && len(namespace) == 0 {
			keys = append(keys, split[0])
		}
	}

	return keys
}

func (c *container) GetLabelKeys() []string {
	keys := make([]string, len(c.Labels))

	idx := 0
	for key := range c.Labels {
		keys[idx] = key
		idx++
	}

	return keys
}

func (c *container) GetLabel(key string) (string, bool) {
	value, ok := c.Labels[key]
	return value, ok
}

func (c *container) GetResmgrLabelKeys() []string {
	return keysInNamespace(&c.Labels, kubernetes.ResmgrKeyNamespace)
}

func (c *container) GetResmgrLabel(key string) (string, bool) {
	value, ok := c.Labels[kubernetes.ResmgrKey(key)]
	return value, ok
}

func (c *container) GetLabels() map[string]string {
	if c.Labels == nil {
		return nil
	}
	labels := make(map[string]string)
	for key, value := range c.Labels {
		labels[key] = value
	}
	return labels
}

func (c *container) GetAnnotationKeys() []string {
	keys := make([]string, len(c.Annotations))

	idx := 0
	for key := range c.Annotations {
		keys[idx] = key
		idx++
	}

	return keys
}

func (c *container) GetAnnotation(key string, objPtr interface{}) (string, bool) {
	jsonStr, ok := c.Annotations[key]
	if !ok {
		return "", false
	}

	if objPtr != nil {
		if err := json.Unmarshal([]byte(jsonStr), objPtr); err != nil {
			c.cache.Error("failed to unmarshal annotation %s (%s) of pod %s into %T",
				key, jsonStr, c.ID, objPtr)
			return "", false
		}
	}

	return jsonStr, true
}

func (c *container) GetResmgrAnnotationKeys() []string {
	return keysInNamespace(&c.Annotations, kubernetes.ResmgrKeyNamespace)
}

func (c *container) GetResmgrAnnotation(key string, objPtr interface{}) (string, bool) {
	return c.GetAnnotation(kubernetes.ResmgrKey(key), objPtr)
}

func (c *container) GetAnnotations() map[string]string {
	if c.Annotations == nil {
		return nil
	}
	annotations := make(map[string]string)
	for key, value := range c.Annotations {
		annotations[key] = value
	}
	return annotations
}

func (c *container) GetEnvKeys() []string {
	keys := make([]string, len(c.Env))

	idx := 0
	for key := range c.Env {
		keys[idx] = key
		idx++
	}

	return keys
}

func (c *container) GetEnv(key string) (string, bool) {
	value, ok := c.Env[key]
	return value, ok
}

func (c *container) GetMounts() []Mount {
	mounts := make([]Mount, len(c.Mounts))

	idx := 0
	for _, m := range c.Mounts {
		mounts[idx] = *m
		idx++
	}

	return mounts
}

func (c *container) GetMountByHost(path string) *Mount {
	for _, m := range c.Mounts {
		if m.Host == path {
			return &(*m)
		}
	}

	return nil
}

func (c *container) GetMountByContainer(path string) *Mount {
	m, ok := c.Mounts[path]
	if !ok {
		return nil
	}

	return &(*m)
}

func (c *container) GetDevices() []Device {
	devices := make([]Device, len(c.Devices))

	idx := 0
	for _, d := range c.Devices {
		devices[idx] = *d
		idx++
	}

	return devices
}

func (c *container) GetDeviceByHost(path string) *Device {
	for _, d := range c.Devices {
		if d.Host == path {
			return &(*d)
		}
	}

	return nil
}

func (c *container) GetDeviceByContainer(path string) *Device {
	d, ok := c.Devices[path]
	if !ok {
		return nil
	}

	return &(*d)
}

func (c *container) GetResourceRequirements() v1.ResourceRequirements {
	return c.Resources
}

func (c *container) GetLinuxResources() *cri.LinuxContainerResources {
	if c.LinuxReq == nil {
		return nil
	}

	return &(*c.LinuxReq)
}

func (c *container) SetCommand(value []string) {
	c.Command = value
	c.markPending(CRI)
}

func (c *container) SetArgs(value []string) {
	c.Args = value
	c.markPending(CRI)
}

func (c *container) SetLabel(key, value string) {
	if c.Labels == nil {
		c.Labels = make(map[string]string)
	}
	c.Labels[key] = value
	c.markPending(CRI)
}

func (c *container) DeleteLabel(key string) {
	if _, ok := c.Labels[key]; ok {
		delete(c.Labels, key)
		c.markPending(CRI)
	}
}

func (c *container) SetAnnotation(key, value string) {
	if c.Annotations == nil {
		c.Annotations = make(map[string]string)
	}
	c.Annotations[key] = value
	c.markPending(CRI)
}

func (c *container) DeleteAnnotation(key string) {
	if _, ok := c.Annotations[key]; ok {
		delete(c.Annotations, key)
		c.markPending(CRI)
	}
}

func (c *container) SetEnv(key, value string) {
	if c.Env == nil {
		c.Env = make(map[string]string)
	}
	c.Env[key] = value
	c.markPending(CRI)
}

func (c *container) UnsetEnv(key string) {
	if _, ok := c.Env[key]; ok {
		delete(c.Env, key)
		c.markPending(CRI)
	}
}

func (c *container) InsertMount(m *Mount) {
	if c.Mounts == nil {
		c.Mounts = make(map[string]*Mount)
	}
	c.Mounts[m.Container] = m
	c.markPending(CRI)
}

func (c *container) DeleteMount(path string) {
	if _, ok := c.Mounts[path]; ok {
		delete(c.Mounts, path)
		c.markPending(CRI)
	}
}

func (c *container) InsertDevice(d *Device) {
	if c.Devices == nil {
		c.Devices = make(map[string]*Device)
	}
	c.Devices[d.Container] = d
	c.markPending(CRI)
}

func (c *container) DeleteDevice(path string) {
	if _, ok := c.Devices[path]; ok {
		delete(c.Devices, path)
		c.markPending(CRI)
	}
}

func (c *container) GetTopologyHints() sysfs.TopologyHints {
	return c.TopologyHints
}

func (c *container) GetCPUPeriod() int64 {
	if c.LinuxReq == nil {
		return 0
	}
	return c.LinuxReq.CpuPeriod
}

func (c *container) GetCPUQuota() int64 {
	if c.LinuxReq == nil {
		return 0
	}
	return c.LinuxReq.CpuQuota
}

func (c *container) GetCPUShares() int64 {
	if c.LinuxReq == nil {
		return 0
	}
	return c.LinuxReq.CpuShares
}

func (c *container) GetMemoryLimit() int64 {
	if c.LinuxReq == nil {
		return 0
	}
	return c.LinuxReq.MemoryLimitInBytes
}

func (c *container) GetOomScoreAdj() int64 {
	if c.LinuxReq == nil {
		return 0
	}
	return c.LinuxReq.OomScoreAdj
}

func (c *container) GetCpusetCpus() string {
	if c.LinuxReq == nil {
		return ""
	}
	return c.LinuxReq.CpusetCpus
}

func (c *container) GetCpusetMems() string {
	if c.LinuxReq == nil {
		return ""
	}
	return c.LinuxReq.CpusetMems
}

func (c *container) SetLinuxResources(req *cri.LinuxContainerResources) {
	c.LinuxReq = req
	c.markPending(CRI)
}

func (c *container) SetCPUPeriod(value int64) {
	if c.LinuxReq == nil {
		c.LinuxReq = &cri.LinuxContainerResources{}
	}
	c.LinuxReq.CpuPeriod = value
	c.markPending(CRI)
}

func (c *container) SetCPUQuota(value int64) {
	if c.LinuxReq == nil {
		c.LinuxReq = &cri.LinuxContainerResources{}
	}
	c.LinuxReq.CpuQuota = value
	c.markPending(CRI)
}

func (c *container) SetCPUShares(value int64) {
	if c.LinuxReq == nil {
		c.LinuxReq = &cri.LinuxContainerResources{}
	}
	c.LinuxReq.CpuShares = value
	c.markPending(CRI)
}

func (c *container) SetMemoryLimit(value int64) {
	if c.LinuxReq == nil {
		c.LinuxReq = &cri.LinuxContainerResources{}
	}
	c.LinuxReq.MemoryLimitInBytes = value
	c.markPending(CRI)
}

func (c *container) SetOomScoreAdj(value int64) {
	if c.LinuxReq == nil {
		c.LinuxReq = &cri.LinuxContainerResources{}
	}
	c.LinuxReq.OomScoreAdj = value
	c.markPending(CRI)
}

func (c *container) SetCpusetCpus(value string) {
	if c.LinuxReq == nil {
		c.LinuxReq = &cri.LinuxContainerResources{}
	}
	c.LinuxReq.CpusetCpus = value
	c.markPending(CRI)
}

func (c *container) SetCpusetMems(value string) {
	if c.LinuxReq == nil {
		c.LinuxReq = &cri.LinuxContainerResources{}
	}
	c.LinuxReq.CpusetMems = value
	c.markPending(CRI)
}

func getTopologyHints(hostPath, containerPath string, readOnly bool) (ret sysfs.TopologyHints) {
	if readOnly {
		// if device or path is read-only, assume it as non-important for now
		// TODO: determine topology hint, but use it with low priority
		return
	}

	// ignore topology information for small files in /etc, service files in /var/lib/kubelet and host libraries mounts
	ignoredTopologyPaths := []string{"/.cri-resmgr", "/etc/", "/dev/termination-log", "/lib/", "/lib64/", "/usr/lib/", "/usr/lib32/", "/usr/lib64/"}

	for _, path := range ignoredTopologyPaths {
		if strings.HasPrefix(hostPath, path) || strings.HasPrefix(containerPath, path) {
			return
		}
	}

	// More complext rules, for Kubelet secrets and config maps
	ignoredTopologyPathRegexps := []*regexp.Regexp{
		// Kubelet directory can be different, but we can detect it by structure inside of it.
		// For now, we can safely ignore exposed config maps and secrets for topology hints.
		regexp.MustCompile(`(kubelet)?/pods/[[:xdigit:]-]+/volumes/kubernetes.io~(configmap|secret)/`),
	}
	for _, re := range ignoredTopologyPathRegexps {
		if re.MatchString(hostPath) || re.MatchString(containerPath) {
			return
		}
	}

	if devPath, err := sysfs.FindSysFsDevice(hostPath); err == nil {
		if hints, err := sysfs.NewTopologyHints(devPath); err == nil && len(hints) > 0 {
			ret = sysfs.MergeTopologyHints(ret, hints)
		}
	}
	return
}

func getKubeletHint(cpus, mems string) (ret sysfs.TopologyHints) {
	if cpus != "" || mems != "" {
		ret = sysfs.TopologyHints{
			sysfs.ProviderKubelet: sysfs.TopologyHint{
				Provider: sysfs.ProviderKubelet,
				CPUs:     cpus,
				NUMAs:    mems}}
	}
	return
}

func (c *container) GetAffinity() []*Affinity {
	pod, ok := c.GetPod()
	if !ok {
		c.cache.Error("internal error: can't find Pod for container %s", c.PrettyName())
	}

	affinity := pod.GetContainerAffinity(c.GetName())
	c.cache.Debug("affinity for container %s:", c.PrettyName())
	for _, a := range affinity {
		c.cache.Debug("  - %s", a.String())
	}

	return affinity
}

func (c *container) SetRDTClass(class string) {
	c.RDTClass = class
	c.markPending(RDT)
}

func (c *container) GetRDTClass() string {
	return c.RDTClass
}

func (c *container) SetBlockIOClass(class string) {
	c.BlockIOClass = class
	c.markPending(BlockIO)
}

func (c *container) GetBlockIOClass() string {
	return c.BlockIOClass
}

func (c *container) SetCRIRequest(req interface{}) error {
	if c.req != nil {
		return cacheError("can't set pending container request: another pending")
	}
	c.req = &req
	return nil
}

func (c *container) GetCRIRequest() (interface{}, bool) {
	if c.req == nil {
		return nil, false
	}

	return *c.req, true
}

func (c *container) ClearCRIRequest() (interface{}, bool) {
	req, ok := c.GetCRIRequest()
	c.req = nil
	return req, ok
}

func (c *container) GetCRIEnvs() []*cri.KeyValue {
	envs := make([]*cri.KeyValue, len(c.Env), len(c.Env))
	idx := 0
	for k, v := range c.Env {
		envs[idx] = &cri.KeyValue{
			Key:   k,
			Value: v,
		}
		idx++
	}
	return envs
}

func (c *container) GetCRIMounts() []*cri.Mount {
	if c.Mounts == nil {
		return nil
	}
	mounts := make([]*cri.Mount, len(c.Mounts), len(c.Mounts))
	idx := 0
	for _, m := range c.Mounts {
		mounts[idx] = &cri.Mount{
			ContainerPath:  m.Container,
			HostPath:       m.Host,
			Readonly:       m.Readonly,
			SelinuxRelabel: m.Relabel,
			Propagation:    cri.MountPropagation(m.Propagation),
		}
		idx++
	}
	return mounts
}

func (c *container) GetCRIDevices() []*cri.Device {
	if c.Devices == nil {
		return nil
	}
	devices := make([]*cri.Device, len(c.Devices), len(c.Devices))
	idx := 0
	for _, d := range c.Devices {
		devices[idx] = &cri.Device{
			ContainerPath: d.Container,
			HostPath:      d.Host,
			Permissions:   d.Permissions,
		}
		idx++
	}
	return devices
}

func (c *container) markPending(controller string) {
	if c.pending == nil {
		c.pending = make(map[string]struct{})
	}
	c.pending[controller] = struct{}{}
	c.cache.markChanged(c)
}

func (c *container) ClearPending(controller string) {
	delete(c.pending, controller)
	if len(c.pending) == 0 {
		c.cache.clearChanged(c)
	}
}

func (c *container) GetPending() []string {
	if c.pending == nil {
		return nil
	}
	pending := make([]string, 0, len(c.pending))
	for controller := range c.pending {
		pending = append(pending, controller)
	}
	sort.Strings(pending)
	return pending
}

func (c *container) HasPending(controller string) bool {
	if c.pending == nil {
		return false
	}
	_, pending := c.pending[controller]
	return pending
}
