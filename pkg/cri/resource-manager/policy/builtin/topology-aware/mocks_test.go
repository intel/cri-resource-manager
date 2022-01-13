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

package topologyaware

import (
	"os"
	"time"

	"github.com/intel/cri-resource-manager/pkg/apis/resmgr"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/intel/cri-resource-manager/pkg/topology"
	"github.com/intel/goresctrl/pkg/sst"
	idset "github.com/intel/goresctrl/pkg/utils"
	v1 "k8s.io/api/core/v1"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type mockSystemNode struct {
	id       idset.ID // node id
	memFree  uint64
	memTotal uint64
	memType  system.MemoryType
	distance []int
}

func (fake *mockSystemNode) MemoryInfo() (*system.MemInfo, error) {
	return &system.MemInfo{MemFree: fake.memFree, MemTotal: fake.memTotal}, nil
}

func (fake *mockSystemNode) PackageID() idset.ID {
	return 0
}

func (fake *mockSystemNode) DieID() idset.ID {
	return 0
}

func (fake *mockSystemNode) ID() idset.ID {
	return fake.id
}

func (fake *mockSystemNode) GetMemoryType() system.MemoryType {
	return fake.memType
}

func (fake *mockSystemNode) HasNormalMemory() bool {
	return true
}

func (fake *mockSystemNode) CPUSet() cpuset.CPUSet {
	return cpuset.NewCPUSet()
}

func (fake *mockSystemNode) Distance() []int {
	if len(fake.distance) == 0 {
		return []int{0}
	}
	return fake.distance
}

func (fake *mockSystemNode) DistanceFrom(id idset.ID) int {
	return 0
}

type mockCPUPackage struct {
}

func (p *mockCPUPackage) ID() idset.ID {
	return idset.ID(0)
}

func (p *mockCPUPackage) CPUSet() cpuset.CPUSet {
	return cpuset.NewCPUSet()
}

func (p *mockCPUPackage) NodeIDs() []idset.ID {
	return []idset.ID{}
}

func (p *mockCPUPackage) DieIDs() []idset.ID {
	return []idset.ID{0}
}

func (p *mockCPUPackage) DieCPUSet(idset.ID) cpuset.CPUSet {
	return cpuset.NewCPUSet()
}

func (p *mockCPUPackage) DieNodeIDs(idset.ID) []idset.ID {
	return []idset.ID{}
}

func (p *mockCPUPackage) SstInfo() *sst.SstPackageInfo {
	return &sst.SstPackageInfo{}
}

type mockCPU struct {
	isolated cpuset.CPUSet
	online   cpuset.CPUSet
	id       idset.ID
	node     mockSystemNode
	pkg      mockCPUPackage
}

func (c *mockCPU) BaseFrequency() uint64 {
	return 0
}
func (c *mockCPU) EPP() system.EPP {
	return system.EPPUnknown
}
func (c *mockCPU) ID() idset.ID {
	return idset.ID(0)
}
func (c *mockCPU) PackageID() idset.ID {
	return c.pkg.ID()
}
func (c *mockCPU) DieID() idset.ID {
	return idset.ID(0)
}
func (c *mockCPU) NodeID() idset.ID {
	return c.node.ID()
}
func (c *mockCPU) CoreID() idset.ID {
	return c.id
}
func (c *mockCPU) ThreadCPUSet() cpuset.CPUSet {
	return cpuset.NewCPUSet()
}
func (c *mockCPU) FrequencyRange() system.CPUFreq {
	return system.CPUFreq{}
}
func (c *mockCPU) Online() bool {
	return true
}
func (c *mockCPU) Isolated() bool {
	return false
}
func (c *mockCPU) SetFrequencyLimits(min, max uint64) error {
	return nil
}

func (c *mockCPU) SstClos() int {
	return -1
}

type mockSystem struct {
	isolatedCPU  int
	nodes        []system.Node
	cpuCount     int
	packageCount int
	socketCount  int
}

func (fake *mockSystem) Node(id idset.ID) system.Node {
	for _, node := range fake.nodes {
		if node.ID() == id {
			return node
		}
	}
	return &mockSystemNode{}
}

func (fake *mockSystem) CPU(idset.ID) system.CPU {
	return &mockCPU{}
}
func (fake *mockSystem) CPUCount() int {
	if fake.cpuCount == 0 {
		return 1
	}
	return fake.cpuCount
}
func (fake *mockSystem) Discover(flags system.DiscoveryFlag) error {
	return nil
}
func (fake *mockSystem) Package(idset.ID) system.CPUPackage {
	return &mockCPUPackage{}
}
func (fake *mockSystem) Offlined() cpuset.CPUSet {
	return cpuset.NewCPUSet()
}
func (fake *mockSystem) Isolated() cpuset.CPUSet {
	if fake.isolatedCPU > 0 {
		return cpuset.NewCPUSet(fake.isolatedCPU)
	}

	return cpuset.NewCPUSet()
}
func (fake *mockSystem) CPUSet() cpuset.CPUSet {
	return cpuset.NewCPUSet()
}
func (fake *mockSystem) CPUIDs() []idset.ID {
	return []idset.ID{}
}
func (fake *mockSystem) PackageCount() int {
	if fake.packageCount == 0 {
		return 1
	}
	return fake.packageCount
}
func (fake *mockSystem) SocketCount() int {
	if fake.socketCount == 0 {
		return 1
	}
	return fake.socketCount
}
func (fake *mockSystem) NUMANodeCount() int {
	return len(fake.nodes)
}
func (fake *mockSystem) ThreadCount() int {
	if fake.cpuCount == 0 {
		return 1
	}
	return fake.cpuCount
}
func (fake *mockSystem) PackageIDs() []idset.ID {
	ids := make([]idset.ID, len(fake.nodes))
	for i, node := range fake.nodes {
		ids[i] = node.PackageID()
	}
	return ids
}
func (fake *mockSystem) NodeIDs() []idset.ID {
	ids := make([]idset.ID, len(fake.nodes))
	for i, node := range fake.nodes {
		ids[i] = node.ID()
	}
	return ids
}
func (fake *mockSystem) SetCPUFrequencyLimits(min, max uint64, cpus idset.IDSet) error {
	return nil
}
func (fake *mockSystem) SetCpusOnline(online bool, cpus idset.IDSet) (idset.IDSet, error) {
	return idset.NewIDSet(), nil
}
func (fake *mockSystem) NodeDistance(idset.ID, idset.ID) int {
	return 10
}

type mockContainer struct {
	name                                  string
	namespace                             string
	returnValueForGetResourceRequirements v1.ResourceRequirements
	returnValueForGetCacheID              string
	returnValueForGetID                   string
	memoryLimit                           int64
	cpuset                                cpuset.CPUSet
	returnValueForQOSClass                v1.PodQOSClass
	pod                                   cache.Pod
}

func (m *mockContainer) PrettyName() string {
	return m.name
}
func (m *mockContainer) GetPod() (cache.Pod, bool) {
	if m.pod == nil {
		return &mockPod{}, false
	}
	return m.pod, true
}
func (m *mockContainer) GetID() string {
	return m.returnValueForGetID
}
func (m *mockContainer) GetPodID() string {
	panic("unimplemented")
}
func (m *mockContainer) GetCacheID() string {
	if len(m.returnValueForGetCacheID) == 0 {
		return "0"
	}

	return m.returnValueForGetCacheID
}
func (m *mockContainer) GetName() string {
	return m.name
}
func (m *mockContainer) GetNamespace() string {
	return m.namespace
}
func (m *mockContainer) UpdateState(cache.ContainerState) {
	panic("unimplemented")
}
func (m *mockContainer) GetState() cache.ContainerState {
	panic("unimplemented")
}
func (m *mockContainer) GetQOSClass() v1.PodQOSClass {
	if len(m.returnValueForQOSClass) == 0 {
		return v1.PodQOSGuaranteed
	}

	return m.returnValueForQOSClass
}
func (m *mockContainer) GetImage() string {
	panic("unimplemented")
}
func (m *mockContainer) GetCommand() []string {
	panic("unimplemented")
}
func (m *mockContainer) GetArgs() []string {
	panic("unimplemented")
}
func (m *mockContainer) GetLabelKeys() []string {
	panic("unimplemented")
}
func (m *mockContainer) GetLabel(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) GetLabels() map[string]string {
	panic("unimplemented")
}
func (m *mockContainer) GetResmgrLabelKeys() []string {
	panic("unimplemented")
}
func (m *mockContainer) GetResmgrLabel(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) GetAnnotationKeys() []string {
	panic("unimplemented")
}
func (m *mockContainer) GetAnnotation(string, interface{}) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) GetResmgrAnnotationKeys() []string {
	panic("unimplemented")
}
func (m *mockContainer) GetResmgrAnnotation(string, interface{}) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) GetEffectiveAnnotation(key string) (string, bool) {
	pod, ok := m.GetPod()
	if !ok {
		return "", false
	}
	return pod.GetEffectiveAnnotation(key, m.name)
}
func (m *mockContainer) GetAnnotations() map[string]string {
	panic("unimplemented")
}
func (m *mockContainer) GetEnvKeys() []string {
	panic("unimplemented")
}
func (m *mockContainer) GetEnv(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) GetMounts() []cache.Mount {
	panic("unimplemented")
}
func (m *mockContainer) GetMountByHost(string) *cache.Mount {
	panic("unimplemented")
}
func (m *mockContainer) GetMountByContainer(string) *cache.Mount {
	panic("unimplemented")
}
func (m *mockContainer) GetDevices() []cache.Device {
	panic("unimplemented")
}
func (m *mockContainer) GetDeviceByHost(string) *cache.Device {
	panic("unimplemented")
}
func (m *mockContainer) GetDeviceByContainer(string) *cache.Device {
	panic("unimplemented")
}
func (m *mockContainer) GetResourceRequirements() v1.ResourceRequirements {
	return m.returnValueForGetResourceRequirements
}
func (m *mockContainer) GetLinuxResources() *cri.LinuxContainerResources {
	panic("unimplemented")
}
func (m *mockContainer) SetCommand([]string) {
	panic("unimplemented")
}
func (m *mockContainer) SetArgs([]string) {
	panic("unimplemented")
}
func (m *mockContainer) SetLabel(string, string) {
	panic("unimplemented")
}
func (m *mockContainer) DeleteLabel(string) {
	panic("unimplemented")
}
func (m *mockContainer) SetAnnotation(string, string) {
	panic("unimplemented")
}
func (m *mockContainer) DeleteAnnotation(string) {
	panic("unimplemented")
}
func (m *mockContainer) SetEnv(string, string) {
	panic("unimplemented")
}
func (m *mockContainer) UnsetEnv(string) {
	panic("unimplemented")
}
func (m *mockContainer) InsertMount(*cache.Mount) {
	panic("unimplemented")
}
func (m *mockContainer) DeleteMount(string) {
	panic("unimplemented")
}
func (m *mockContainer) InsertDevice(*cache.Device) {
	panic("unimplemented")
}
func (m *mockContainer) DeleteDevice(string) {
	panic("unimplemented")
}
func (m *mockContainer) GetTopologyHints() topology.Hints {
	return topology.Hints{}
}
func (m *mockContainer) GetCPUPeriod() int64 {
	panic("unimplemented")
}
func (m *mockContainer) GetCPUQuota() int64 {
	panic("unimplemented")
}
func (m *mockContainer) GetCPUShares() int64 {
	panic("unimplemented")
}
func (m *mockContainer) GetMemoryLimit() int64 {
	return m.memoryLimit
}
func (m *mockContainer) GetOomScoreAdj() int64 {
	panic("unimplemented")
}
func (m *mockContainer) GetCpusetCpus() string {
	return m.cpuset.String()
}
func (m *mockContainer) GetCpusetMems() string {
	panic("unimplemented")
}
func (m *mockContainer) SetLinuxResources(*cri.LinuxContainerResources) {
	panic("unimplemented")
}
func (m *mockContainer) SetCPUPeriod(int64) {
	panic("unimplemented")
}
func (m *mockContainer) SetCPUQuota(int64) {
	panic("unimplemented")
}
func (m *mockContainer) SetCPUShares(int64) {
}
func (m *mockContainer) SetMemoryLimit(int64) {
	panic("unimplemented")
}
func (m *mockContainer) SetOomScoreAdj(int64) {
	panic("unimplemented")
}
func (m *mockContainer) SetCpusetCpus(string) {
}
func (m *mockContainer) SetCpusetMems(string) {
}
func (m *mockContainer) UpdateCriCreateRequest(*cri.CreateContainerRequest) error {
	panic("unimplemented")
}
func (m *mockContainer) CriUpdateRequest() (*cri.UpdateContainerResourcesRequest, error) {
	panic("unimplemented")
}
func (m *mockContainer) GetAffinity() ([]*cache.Affinity, error) {
	return nil, nil
}
func (m *mockContainer) SetRDTClass(string) {
	panic("unimplemented")
}
func (m *mockContainer) GetRDTClass() string {
	panic("unimplemented")
}
func (m *mockContainer) SetBlockIOClass(string) {
	panic("unimplemented")
}
func (m *mockContainer) GetBlockIOClass() string {
	panic("unimplemented")
}
func (m *mockContainer) SetToptierLimit(int64) {
	panic("unimplemented")
}
func (m *mockContainer) GetToptierLimit() int64 {
	panic("unimplemented")
}
func (m *mockContainer) SetPageMigration(*cache.PageMigrate) {
	return
}
func (m *mockContainer) GetPageMigration() *cache.PageMigrate {
	return nil
}
func (m *mockContainer) SetCRIRequest(req interface{}) error {
	panic("unimplemented")
}
func (m *mockContainer) GetCRIRequest() (interface{}, bool) {
	panic("unimplemented")
}
func (m *mockContainer) ClearCRIRequest() (interface{}, bool) {
	panic("unimplemented")
}
func (m *mockContainer) GetCRIEnvs() []*cri.KeyValue {
	panic("unimplemented")
}
func (m *mockContainer) GetCRIMounts() []*cri.Mount {
	panic("unimplemented")
}
func (m *mockContainer) GetCRIDevices() []*cri.Device {
	panic("unimplemented")
}
func (m *mockContainer) GetPending() []string {
	panic("unimplemented")
}
func (m *mockContainer) HasPending(string) bool {
	panic("unimplemented")
}
func (m *mockContainer) ClearPending(string) {
	panic("unimplemented")
}
func (m *mockContainer) GetTag(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) SetTag(string, string) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) DeleteTag(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockContainer) String() string {
	return "mockContainer"
}
func (m *mockContainer) Eval(string) interface{} {
	panic("unimplemented")
}
func (m *mockContainer) GetProcesses() ([]string, error) {
	panic("unimplemented")
}
func (m *mockContainer) GetTasks() ([]string, error) {
	panic("unimplemented")
}
func (m *mockContainer) GetCgroupDir() string {
	panic("unimplemented")
}

type mockPod struct {
	name                               string
	returnValueFotGetQOSClass          v1.PodQOSClass
	returnValue1FotGetResmgrAnnotation string
	returnValue2FotGetResmgrAnnotation bool
	coldStartTimeout                   time.Duration
	coldStartContainerName             string
	annotations                        map[string]string
}

func (m *mockPod) GetInitContainers() []cache.Container {
	panic("unimplemented")
}
func (m *mockPod) GetContainers() []cache.Container {
	panic("unimplemented")
}
func (m *mockPod) GetContainer(string) (cache.Container, bool) {
	panic("unimplemented")
}
func (m *mockPod) GetID() string {
	panic("unimplemented")
}
func (m *mockPod) GetUID() string {
	panic("unimplemented")
}
func (m *mockPod) GetName() string {
	return m.name
}
func (m *mockPod) GetNamespace() string {
	panic("unimplemented")
}
func (m *mockPod) GetState() cache.PodState {
	panic("unimplemented")
}
func (m *mockPod) GetQOSClass() v1.PodQOSClass {
	return m.returnValueFotGetQOSClass
}
func (m *mockPod) GetLabelKeys() []string {
	panic("unimplemented")
}
func (m *mockPod) GetLabel(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockPod) GetResmgrLabelKeys() []string {
	panic("unimplemented")
}
func (m *mockPod) GetResmgrLabel(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockPod) GetAnnotationKeys() []string {
	panic("unimplemented")
}
func (m *mockPod) GetAnnotation(string) (string, bool) {
	panic("unimplemented")
}
func (m *mockPod) GetAnnotationObject(string, interface{}, func([]byte, interface{}) error) (bool, error) {
	panic("unimplemented")
}
func (m *mockPod) GetResmgrAnnotationKeys() []string {
	panic("unimplemented")
}
func (m *mockPod) GetResmgrAnnotation(key string) (string, bool) {
	if key == keyColdStartPreference && len(m.coldStartContainerName) > 0 {
		return m.coldStartContainerName + ": { duration: " + m.coldStartTimeout.String() + " }", true
	}
	return m.returnValue1FotGetResmgrAnnotation, m.returnValue2FotGetResmgrAnnotation
}
func (m *mockPod) GetResmgrAnnotationObject(string, interface{}, func([]byte, interface{}) error) (bool, error) {
	panic("unimplemented")
}
func (m *mockPod) GetEffectiveAnnotation(key, container string) (string, bool) {
	if v, ok := m.annotations[key+"/container."+container]; ok {
		return v, true
	}
	if v, ok := m.annotations[key+"/pod"]; ok {
		return v, true
	}
	v, ok := m.annotations[key]
	return v, ok
}
func (m *mockPod) GetCgroupParentDir() string {
	panic("unimplemented")
}
func (m *mockPod) GetPodResourceRequirements() cache.PodResourceRequirements {
	panic("unimplemented")
}
func (m *mockPod) GetContainerAffinity(string) ([]*cache.Affinity, error) {
	panic("unimplemented")
}
func (m *mockPod) ScopeExpression() *resmgr.Expression {
	panic("unimplemented")
}
func (m *mockPod) String() string {
	return "mockPod"
}
func (m *mockPod) Eval(string) interface{} {
	panic("unimplemented")
}
func (m *mockPod) GetProcesses(bool) ([]string, error) {
	panic("unimplemented")
}
func (m *mockPod) GetTasks(bool) ([]string, error) {
	panic("unimplemented")
}

type mockCache struct {
	returnValueForGetPolicyEntry   bool
	returnValue1ForLookupContainer cache.Container
	returnValue2ForLookupContainer bool
}

func (m *mockCache) InsertPod(string, interface{}, *cache.PodStatus) (cache.Pod, error) {
	panic("unimplemented")
}
func (m *mockCache) DeletePod(string) cache.Pod {
	panic("unimplemented")
}
func (m *mockCache) LookupPod(string) (cache.Pod, bool) {
	panic("unimplemented")
}
func (m *mockCache) InsertContainer(interface{}) (cache.Container, error) {
	panic("unimplemented")
}
func (m *mockCache) UpdateContainerID(string, interface{}) (cache.Container, error) {
	panic("unimplemented")
}
func (m *mockCache) DeleteContainer(string) cache.Container {
	panic("unimplemented")
}
func (m *mockCache) LookupContainer(string) (cache.Container, bool) {
	return m.returnValue1ForLookupContainer, m.returnValue2ForLookupContainer
}
func (m *mockCache) LookupContainerByCgroup(path string) (cache.Container, bool) {
	panic("unimplemented")
}
func (m *mockCache) GetPendingContainers() []cache.Container {
	panic("unimplemented")
}
func (m *mockCache) GetPods() []cache.Pod {
	panic("unimplemented")
}
func (m *mockCache) GetContainers() []cache.Container {
	panic("unimplemented")
}
func (m *mockCache) GetContainerCacheIds() []string {
	panic("unimplemented")
}
func (m *mockCache) GetContainerIds() []string {
	panic("unimplemented")
}
func (m *mockCache) FilterScope(*resmgr.Expression) []cache.Container {
	panic("unimplemented")
}
func (m *mockCache) EvaluateAffinity(*cache.Affinity) map[string]int32 {
	return map[string]int32{
		"fake key": 1,
	}
}
func (m *mockCache) AddImplicitAffinities(map[string]cache.ImplicitAffinity) error {
	return nil
}
func (m *mockCache) GetActivePolicy() string {
	panic("unimplemented")
}
func (m *mockCache) SetActivePolicy(string) error {
	panic("unimplemented")
}
func (m *mockCache) ResetActivePolicy() error {
	panic("unimplemented")
}
func (m *mockCache) SetPolicyEntry(string, interface{}) {
}
func (m *mockCache) GetPolicyEntry(string, interface{}) bool {
	return m.returnValueForGetPolicyEntry
}
func (m *mockCache) SetConfig(*config.RawConfig) error {
	panic("unimplemented")
}
func (m *mockCache) GetConfig() *config.RawConfig {
	panic("unimplemented")
}
func (m *mockCache) ResetConfig() error {
	panic("unimplemented")
}
func (m *mockCache) SetAdjustment(*config.Adjustment) (bool, map[string]error) {
	panic("unimplemented")
}
func (m *mockCache) Save() error {
	return nil
}
func (m *mockCache) RefreshPods(*cri.ListPodSandboxResponse, map[string]*cache.PodStatus) ([]cache.Pod, []cache.Pod, []cache.Container) {
	panic("unimplemented")
}
func (m *mockCache) RefreshContainers(*cri.ListContainersResponse) ([]cache.Container, []cache.Container) {
	panic("unimplemented")
}
func (m *mockCache) ContainerDirectory(string) string {
	panic("unimplemented")
}
func (m *mockCache) OpenFile(string, string, os.FileMode) (*os.File, error) {
	panic("unimplemented")
}
func (m *mockCache) WriteFile(string, string, os.FileMode, []byte) error {
	panic("unimplemented")
}
