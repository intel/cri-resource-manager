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
	"strconv"
	"strings"

	"github.com/intel/cri-resource-manager/pkg/apis/resmgr"
	"github.com/intel/cri-resource-manager/pkg/cgroups"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	"github.com/intel/cri-resource-manager/pkg/topology"

	v1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	extapi "github.com/intel/cri-resource-manager/pkg/apis/resmgr/v1alpha1"
)

// Create a container for a create request.
func (c *container) fromCreateRequest(req *cri.CreateContainerRequest) error {
	c.PodID = req.PodSandboxId

	pod, ok := c.cache.Pods[c.PodID]
	if !ok {
		return cacheError("can't find cached pod %s for container to create", c.PodID)
	}

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

	genHints := true
	if hintSetting, ok := c.GetEffectiveAnnotation(TopologyHintsKey); ok {
		preference, err := strconv.ParseBool(hintSetting)
		if err != nil {
			c.cache.Error("invalid annotation %q=%q: %v", TopologyHintsKey, hintSetting, err)
		} else {
			genHints = preference
		}
	}
	c.cache.Info("automatic topology hint generation %s for %q",
		map[bool]string{false: "disabled", true: "enabled"}[genHints], c.PrettyName())

	c.Mounts = make(map[string]*Mount)
	for _, m := range cfg.Mounts {
		c.Mounts[m.ContainerPath] = &Mount{
			Container:   m.ContainerPath,
			Host:        m.HostPath,
			Readonly:    m.Readonly,
			Relabel:     m.SelinuxRelabel,
			Propagation: MountType(m.Propagation),
		}

		if genHints {
			if hints := getTopologyHints(m.HostPath, m.ContainerPath, m.Readonly); len(hints) > 0 {
				c.TopologyHints = topology.MergeTopologyHints(c.TopologyHints, hints)
			}
		}
	}

	c.Devices = make(map[string]*Device)
	for _, d := range cfg.Devices {
		c.Devices[d.ContainerPath] = &Device{
			Container:   d.ContainerPath,
			Host:        d.HostPath,
			Permissions: d.Permissions,
		}
		if genHints {
			if hints := getTopologyHints(d.HostPath, d.ContainerPath, strings.IndexAny(d.Permissions, "wm") == -1); len(hints) > 0 {
				c.TopologyHints = topology.MergeTopologyHints(c.TopologyHints, hints)
			}
		}
	}

	c.Tags = make(map[string]string)

	c.LinuxReq = cfg.GetLinux().GetResources()

	if pod.Resources != nil {
		if r, ok := pod.Resources.InitContainers[c.Name]; ok {
			c.Resources = r
		} else if r, ok := pod.Resources.Containers[c.Name]; ok {
			c.Resources = r
		}
	}

	if len(c.Resources.Requests) == 0 && len(c.Resources.Limits) == 0 {
		c.Resources = estimateComputeResources(c.LinuxReq, pod.CgroupParent)
	}

	c.TopologyHints = topology.MergeTopologyHints(c.TopologyHints, getKubeletHint(c.GetCpusetCpus(), c.GetCpusetMems()))

	if err := c.setDefaults(); err != nil {
		return err
	}

	return nil
}

// Create container from a container list response.
func (c *container) fromListResponse(lrc *cri.Container) error {
	c.PodID = lrc.PodSandboxId

	pod, ok := c.cache.Pods[c.PodID]
	if !ok {
		return cacheError("can't find cached pod %s for listed container", c.PodID)
	}

	meta := lrc.Metadata
	if meta == nil {
		return cacheError("listed container of pod %s has no metadata", c.PodID)
	}

	c.ID = lrc.Id
	c.Name = meta.Name
	c.Namespace = pod.Namespace
	c.State = ContainerState(int32(lrc.State))
	c.Image = lrc.GetImage().GetImage()
	c.Labels = lrc.Labels
	c.Annotations = lrc.Annotations
	c.Tags = make(map[string]string)

	if pod.Resources != nil {
		if r, ok := pod.Resources.InitContainers[c.Name]; ok {
			c.Resources = r
		} else if r, ok := pod.Resources.Containers[c.Name]; ok {
			c.Resources = r
		}
	}

	if len(c.Resources.Requests) == 0 && len(c.Resources.Limits) == 0 {
		c.Resources = estimateComputeResources(c.LinuxReq, pod.CgroupParent)
	}

	if err := c.setDefaults(); err != nil {
		return err
	}

	return nil
}

func (c *container) setDefaults() error {
	class, ok := c.GetEffectiveAnnotation(RDTClassKey)
	if !ok {
		class = RDTClassPodQoS
	}
	c.SetRDTClass(class)

	class, ok = c.GetEffectiveAnnotation(BlockIOClassKey)
	if !ok {
		class = string(c.GetQOSClass())
	}
	c.SetBlockIOClass(class)

	limit, ok := c.GetEffectiveAnnotation(ToptierLimitKey)
	if !ok {
		c.ToptierLimit = ToptierLimitUnset
	} else {
		qty, err := resapi.ParseQuantity(limit)
		if err != nil {
			return cacheError("%q: failed to parse top tier limit annotation %q (%q): %v",
				c.PrettyName(), ToptierLimitKey, limit, err)
		}
		c.SetToptierLimit(qty.Value())
	}

	return nil
}

func (c *container) PrettyName() string {
	if c.prettyName != "" {
		return c.prettyName
	}
	if pod, ok := c.GetPod(); !ok {
		c.prettyName = c.PodID + ":" + c.Name
	} else {
		c.prettyName = pod.GetName() + ":" + c.Name
	}
	return c.prettyName
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

func keysInNamespace(m map[string]string, namespace string) []string {
	keys := make([]string, 0, len(m))

	for key := range m {
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
	return keysInNamespace(c.Labels, kubernetes.ResmgrKeyNamespace)
}

func (c *container) GetResmgrLabel(key string) (string, bool) {
	value, ok := c.Labels[kubernetes.ResmgrKey(key)]
	return value, ok
}

func (c *container) GetLabels() map[string]string {
	if c.Labels == nil {
		return nil
	}
	labels := make(map[string]string, len(c.Labels))
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
	return keysInNamespace(c.Annotations, kubernetes.ResmgrKeyNamespace)
}

func (c *container) GetResmgrAnnotation(key string, objPtr interface{}) (string, bool) {
	return c.GetAnnotation(kubernetes.ResmgrKey(key), objPtr)
}

func (c *container) GetEffectiveAnnotation(key string) (string, bool) {
	pod, ok := c.GetPod()
	if !ok {
		return "", false
	}
	return pod.GetEffectiveAnnotation(key, c.Name)
}

func (c *container) GetAnnotations() map[string]string {
	if c.Annotations == nil {
		return nil
	}
	annotations := make(map[string]string, len(c.Annotations))
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
	if adjust, _ := c.getEffectiveAdjustment(); adjust != nil {
		if resources, ok := adjust.GetResourceRequirements(); ok {
			return resources
		}
	}
	return c.Resources
}

func (c *container) GetLinuxResources() *cri.LinuxContainerResources {
	if c.LinuxReq == nil {
		return nil
	}

	return &(*c.LinuxReq)
}

func (c *container) setEffectiveAdjustment(name string) string {
	previous := c.Adjustment
	c.Adjustment = name
	return previous
}

func (c *container) getEffectiveAdjustment() (*extapi.AdjustmentSpec, string) {
	if c.Adjustment == "" {
		return nil, ""
	}
	if c.cache.External != nil {
		return c.cache.External.Adjustments[c.Adjustment], c.Adjustment
	}
	return nil, c.Adjustment
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

func (c *container) GetTopologyHints() topology.Hints {
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

func getTopologyHints(hostPath, containerPath string, readOnly bool) topology.Hints {

	if readOnly {
		// if device or path is read-only, assume it as non-important for now
		// TODO: determine topology hint, but use it with low priority
		return topology.Hints{}
	}

	// ignore topology information for small files in /etc, service files in /var/lib/kubelet and host libraries mounts
	ignoredTopologyPaths := []string{"/.cri-resmgr", "/etc/", "/dev/termination-log", "/lib/", "/lib64/", "/usr/lib/", "/usr/lib32/", "/usr/lib64/"}

	for _, path := range ignoredTopologyPaths {
		if strings.HasPrefix(hostPath, path) || strings.HasPrefix(containerPath, path) {
			return topology.Hints{}
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
			return topology.Hints{}
		}
	}

	if devPath, err := topology.FindSysFsDevice(hostPath); err == nil {
		// errors are ignored
		if hints, err := topology.NewTopologyHints(devPath); err == nil && len(hints) > 0 {
			return hints
		}
	}

	return topology.Hints{}
}

func getKubeletHint(cpus, mems string) (ret topology.Hints) {
	if cpus != "" || mems != "" {
		ret = topology.Hints{
			topology.ProviderKubelet: topology.Hint{
				Provider: topology.ProviderKubelet,
				CPUs:     cpus,
				NUMAs:    mems}}
	}
	return
}

func (c *container) GetAffinity() ([]*Affinity, error) {
	pod, ok := c.GetPod()
	if !ok {
		c.cache.Error("internal error: can't find Pod for container %s", c.PrettyName())
	}
	affinity, err := pod.GetContainerAffinity(c.GetName())
	if err != nil {
		return nil, err
	}
	affinity = append(affinity, c.implicitAffinities()...)
	c.cache.Debug("affinity for container %s:", c.PrettyName())
	for _, a := range affinity {
		c.cache.Debug("  - %s", a.String())
	}

	return affinity, nil
}

func (c *container) GetCgroupDir() string {
	if c.CgroupDir != "" {
		return c.CgroupDir
	}
	if pod, ok := c.GetPod(); ok {
		parent, podID := pod.GetCgroupParentDir(), pod.GetID()
		ID := c.GetID()
		c.CgroupDir = findContainerDir(parent, podID, ID)
	}
	return c.CgroupDir
}

func (c *container) SetRDTClass(class string) {
	c.RDTClass = class
	c.markPending(RDT)
}

func (c *container) GetRDTClass() string {
	if adjust, _ := c.getEffectiveAdjustment(); adjust != nil {
		if class, ok := adjust.GetRDTClass(); ok {
			return class
		}
	}
	return c.RDTClass
}

func (c *container) SetBlockIOClass(class string) {
	c.BlockIOClass = class
	c.markPending(BlockIO)
}

func (c *container) GetBlockIOClass() string {
	if adjust, _ := c.getEffectiveAdjustment(); adjust != nil {
		if class, ok := adjust.GetBlockIOClass(); ok {
			return class
		}
	}
	return c.BlockIOClass
}

func (c *container) SetToptierLimit(limit int64) {
	c.ToptierLimit = limit
	c.markPending(Memory)
}

func (c *container) GetToptierLimit() int64 {
	if adjust, _ := c.getEffectiveAdjustment(); adjust != nil {
		if adjust.ToptierLimit != nil {
			return adjust.ToptierLimit.Value()
		}
	}
	return c.ToptierLimit
}

func (c *container) SetPageMigration(p *PageMigrate) {
	c.PageMigrate = p
	c.markPending(PageMigration)
}

func (c *container) GetPageMigration() *PageMigrate {
	return c.PageMigrate
}

func (c *container) GetProcesses() ([]string, error) {
	dir := c.GetCgroupDir()
	if dir == "" {
		return nil, cacheError("%s: unknown cgroup directory", c.PrettyName())
	}
	return cgroups.Cpu.Group(dir).GetProcesses()
}

func (c *container) GetTasks() ([]string, error) {
	dir := c.GetCgroupDir()
	if dir == "" {
		return nil, cacheError("%s: unknown cgroup directory", c.PrettyName())
	}
	return cgroups.Cpu.Group(dir).GetTasks()
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

func (c *container) markPending(controllers ...string) {
	if c.pending == nil {
		c.pending = make(map[string]struct{})
	}
	for _, ctrl := range controllers {
		c.pending[ctrl] = struct{}{}
		c.cache.markPending(c)
	}
}

func (c *container) ClearPending(controller string) {
	delete(c.pending, controller)
	if len(c.pending) == 0 {
		c.cache.clearPending(c)
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

func (c *container) GetTag(key string) (string, bool) {
	value, ok := c.Tags[key]
	return value, ok
}

func (c *container) SetTag(key string, value string) (string, bool) {
	prev, ok := c.Tags[key]
	c.Tags[key] = value
	return prev, ok
}

func (c *container) DeleteTag(key string) (string, bool) {
	value, ok := c.Tags[key]
	delete(c.Tags, key)
	return value, ok
}

func (c *container) implicitAffinities() []*Affinity {
	implicit := []*Affinity{}
	for name, ia := range c.cache.implicit {
		if ia.Eligible == nil || ia.Eligible(c) {
			c.cache.Debug("adding implicit affinity %s (%s)", name, ia.Affinity.String())
			implicit = append(implicit, ia.Affinity)
		}
	}
	return implicit
}

func (c *container) String() string {
	return c.PrettyName()
}

func (c *container) Eval(key string) interface{} {
	switch key {
	case resmgr.KeyPod:
		pod, ok := c.GetPod()
		if !ok {
			return cacheError("%s: failed to find pod %s", c.PrettyName(), c.PodID)
		}
		return pod
	case resmgr.KeyName:
		return c.Name
	case resmgr.KeyNamespace:
		return c.Namespace
	case resmgr.KeyQOSClass:
		return c.GetQOSClass()
	case resmgr.KeyLabels:
		return c.Labels
	case resmgr.KeyTags:
		return c.Tags
	case resmgr.KeyID:
		return c.ID
	default:
		return cacheError("%s: Container cannot evaluate of %q", c.PrettyName(), key)
	}
}

// CompareContainersFn compares two containers by some arbitrary property.
// It returns a negative integer, 0, or a positive integer, depending on
// whether the first container is considered smaller, equal, or larger than
// the second.
type CompareContainersFn func(Container, Container) int

// SortContainers sorts a slice of containers using the given comparison functions.
// If the containers are otherwise equal they are sorted by pod and container name.
// If the comparison functions are omitted, containers are compared by QoS class,
// memory and cpuset size.
func SortContainers(containers []Container, compareFns ...CompareContainersFn) {
	if len(compareFns) == 0 {
		compareFns = CompareByQOSMemoryCPU
	}
	sort.Slice(containers, func(i, j int) bool {
		ci, cj := containers[i], containers[j]
		for _, cmpFn := range compareFns {
			switch diff := cmpFn(ci, cj); {
			case diff < 0:
				return true
			case diff > 0:
				return false
			}
		}
		// If two containers are otherwise equal they are sorted by pod and container name.
		if pi, ok := ci.GetPod(); ok {
			if pj, ok := cj.GetPod(); ok {
				ni, nj := pi.GetName(), pj.GetName()
				if ni != nj {
					return ni < nj
				}
			}
		}
		return ci.GetName() < cj.GetName()
	})
}

// CompareByQOSMemoryCPU is a slice for comparing container by QOS, memory, and CPU.
var CompareByQOSMemoryCPU = []CompareContainersFn{CompareQOS, CompareMemory, CompareCPU}

// CompareQOS compares containers by QOS class.
func CompareQOS(ci, cj Container) int {
	qosi, qosj := ci.GetQOSClass(), cj.GetQOSClass()
	switch {
	case qosi == v1.PodQOSGuaranteed && qosj != v1.PodQOSGuaranteed:
		return -1
	case qosj == v1.PodQOSGuaranteed && qosi != v1.PodQOSGuaranteed:
		return +1
	case qosi == v1.PodQOSBurstable && qosj == v1.PodQOSBestEffort:
		return -1
	case qosj == v1.PodQOSBurstable && qosi == v1.PodQOSBestEffort:
		return +1
	}
	return 0
}

// CompareMemory compares containers by memory requests and limits.
func CompareMemory(ci, cj Container) int {
	var reqi, limi, reqj, limj int64

	resi := ci.GetResourceRequirements()
	if qty, ok := resi.Requests[v1.ResourceMemory]; ok {
		reqi = qty.Value()
	}
	if qty, ok := resi.Limits[v1.ResourceMemory]; ok {
		limi = qty.Value()
	}
	resj := cj.GetResourceRequirements()
	if qty, ok := resj.Requests[v1.ResourceMemory]; ok {
		reqj = qty.Value()
	}
	if qty, ok := resj.Limits[v1.ResourceMemory]; ok {
		limj = qty.Value()
	}

	switch diff := reqj - reqi; {
	case diff < 0:
		return -1
	case diff > 0:
		return +1
	}
	switch diff := limj - limi; {
	case diff < 0:
		return -1
	case diff > 0:
		return +1
	}
	return 0
}

// CompareCPU compares containers by CPU requests and limits.
func CompareCPU(ci, cj Container) int {
	var reqi, limi, reqj, limj int64

	resi := ci.GetResourceRequirements()
	if qty, ok := resi.Requests[v1.ResourceCPU]; ok {
		reqi = qty.MilliValue()
	}
	if qty, ok := resi.Limits[v1.ResourceCPU]; ok {
		limi = qty.MilliValue()
	}
	resj := cj.GetResourceRequirements()
	if qty, ok := resj.Requests[v1.ResourceCPU]; ok {
		reqj = qty.MilliValue()
	}
	if qty, ok := resj.Limits[v1.ResourceCPU]; ok {
		limj = qty.MilliValue()
	}

	switch diff := reqj - reqi; {
	case diff < 0:
		return -1
	case diff > 0:
		return +1
	}
	switch diff := limj - limi; {
	case diff < 0:
		return -1
	case diff > 0:
		return +1
	}
	return 0
}
