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

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	v1 "k8s.io/api/core/v1"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type mockSystem struct {
	isolatedCPU int
}

func (fake *mockSystem) Node(system.ID) *system.Node {
	return &system.Node{}
}
func (fake *mockSystem) Package(system.ID) *system.Package {
	return &system.Package{}
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
func (fake *mockSystem) SocketCount() int {
	return 2
}
func (fake *mockSystem) NUMANodeCount() int {
	return 2
}
func (fake *mockSystem) PackageIDs() []system.ID {
	return []system.ID{0, 1}
}
func (fake *mockSystem) NodeIDs() []system.ID {
	return []system.ID{0, 1}
}

type mockContainer struct {
	name                                  string
	namespace                             string
	returnValueForGetResourceRequirements v1.ResourceRequirements
	returnValueForGetCacheID              string
}

func (m *mockContainer) PrettyName() string {
	return m.name
}
func (m *mockContainer) GetPod() (cache.Pod, bool) {
	return &mockPod{}, false
}
func (m *mockContainer) GetID() string {
	panic("unimplemented")
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
	panic("unimplemented")
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
func (m *mockContainer) GetTopologyHints() sysfs.TopologyHints {
	return sysfs.TopologyHints{}
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
	panic("unimplemented")
}
func (m *mockContainer) GetOomScoreAdj() int64 {
	panic("unimplemented")
}
func (m *mockContainer) GetCpusetCpus() string {
	panic("unimplemented")
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
	panic("unimplemented")
}
func (m *mockContainer) UpdateCriCreateRequest(*cri.CreateContainerRequest) error {
	panic("unimplemented")
}
func (m *mockContainer) CriUpdateRequest() (*cri.UpdateContainerResourcesRequest, error) {
	panic("unimplemented")
}
func (m *mockContainer) GetAffinity() []*cache.Affinity {
	return nil
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

type mockPod struct {
	name                               string
	returnValueFotGetQOSClass          v1.PodQOSClass
	returnValue1FotGetResmgrAnnotation string
	returnValue2FotGetResmgrAnnotation bool
}

func (m *mockPod) GetInitContainers() []cache.Container {
	panic("unimplemented")
}
func (m *mockPod) GetContainers() []cache.Container {
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
	return m.returnValue1FotGetResmgrAnnotation, m.returnValue2FotGetResmgrAnnotation
}
func (m *mockPod) GetResmgrAnnotationObject(string, interface{}, func([]byte, interface{}) error) (bool, error) {
	panic("unimplemented")
}
func (m *mockPod) GetCgroupParentDir() string {
	panic("unimplemented")
}
func (m *mockPod) GetPodResourceRequirements() cache.PodResourceRequirements {
	panic("unimplemented")
}
func (m *mockPod) GetContainerAffinity(string) []*cache.Affinity {
	panic("unimplemented")
}
func (m *mockPod) ScopeExpression() *cache.Expression {
	panic("unimplemented")
}

type mockCache struct {
	returnValueForGetPolicyEntry   bool
	returnValue1ForLookupContainer cache.Container
	returnValue2ForLookupContainer bool
}

func (m *mockCache) InsertPod(string, interface{}) cache.Pod {
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
func (m *mockCache) StartTransaction() error {
	panic("unimplemented")
}
func (m *mockCache) CommitTransaction() []cache.Container {
	panic("unimplemented")
}
func (m *mockCache) QueryTransaction() []cache.Container {
	panic("unimplemented")
}
func (m *mockCache) AbortTransaction() {
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
func (m *mockCache) FilterScope(*cache.Expression) []cache.Container {
	panic("unimplemented")
}
func (m *mockCache) EvaluateAffinity(*cache.Affinity) map[string]int32 {
	return map[string]int32{
		"fake key": 1,
	}
}
func (m *mockCache) GetActivePolicy() string {
	panic("unimplemented")
}
func (m *mockCache) SetActivePolicy(string) error {
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
func (m *mockCache) Save() error {
	panic("unimplemented")
}
func (m *mockCache) Refresh(interface{}) ([]cache.Pod, []cache.Pod, []cache.Container, []cache.Container) {
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
