// Copyright 2022 Intel Corporation. All Rights Reserved.
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

package dyp

import (
	"fmt"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	cpucontrol "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control/cpu"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/introspect"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	policyapi "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/utils"
	idset "github.com/intel/goresctrl/pkg/utils"
)

const (
	// PolicyName is the name used to activate this policy.
	PolicyName = "dynamic-pools"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "The cpuset of the dynamic pools can be dynamically changed based on workload."
	// PolicyPath is the path of this policy in the configuration hierarchy.
	PolicyPath = "policy." + PolicyName
	// dynamicPoolKey is a pod annotation key, the value is a pod dynamicPool name.
	dynamicPoolKey = "dynamic-pool." + PolicyName + "." + kubernetes.ResmgrKeyNamespace
	// reservedDynamicPoolDefName is the name in the reserved dynamicPool definition.
	reservedDynamicPoolDefName = "reserved"
	// defaultDynamicPoolDefName is the name in the default dynamicPool definition.
	defaultDynamicPoolDefName = "shared"
)

// dynamicPools contains configuration and runtime attributes of the dynamic-pools policy
type dynamicPools struct {
	options   *policyapi.BackendOptions // configuration common to all policies
	dpoptions DynamicPoolsOptions       // dynamicPool-specific configuration
	cch       cache.Cache               // cri-resmgr cache
	allowed   cpuset.CPUSet             // bounding set of CPUs we're allowed to use
	reserved  cpuset.CPUSet             // system-/kube-reserved CPUs
	freeCpus  cpuset.CPUSet             // CPUs to be included in growing dynamicPools

	reservedDynamicPoolDef *DynamicPoolDef // built-in definition of the reserved dynamicPool
	defaultDynamicPoolDef  *DynamicPoolDef // built-in definition of the default dynamicPool
	dynamicPools           []*DynamicPool  // dynamicPool instances: reserved, default and user-defined

	cpuAllocator cpuallocator.CPUAllocator // CPU allocator used by the policy
}

// DynamicPool contains attributes of a dynamicPool
type DynamicPool struct {
	// Def is the definition from which this dynamicPool is created.
	Def *DynamicPoolDef
	// Cpus is the set of CPUs exclusive to this dynamicPool only.
	Cpus cpuset.CPUSet
	// Mems is the set of memory nodes with minimal access delay from CPUs.
	Mems idset.IDSet
	// PodIDs maps pod ID to list of container IDs.
	// - len(PodIDs) is the number of pods in the dynamicPool.
	// - len(PodIDs[podID]) is the number of containers of podID currently assigned to the dynamicPool.
	PodIDs map[string][]string
}

var log logger.Logger = logger.NewLogger("policy")

// String is a stringer for a dynamicPool.
func (dp DynamicPool) String() string {
	return fmt.Sprintf("%s{Cpus:%s, Mems:%s}", dp.PrettyName(), dp.Cpus, dp.Mems)
}

// PrettyName returns a unique name for a dynamicPool.
func (dp DynamicPool) PrettyName() string {
	return dp.Def.Name
}

// ContainerIDs returns IDs of containers assigned in a dynamicPool.
// (Using cache.Container.GetCacheID()'s)
func (dp DynamicPool) ContainerIDs() []string {
	cIDs := []string{}
	for _, ctrIDs := range dp.PodIDs {
		cIDs = append(cIDs, ctrIDs...)
	}
	return cIDs
}

// ContainerCount returns the number of containers in a dynamicPool.
func (dp DynamicPool) ContainerCount() int {
	count := 0
	for _, ctrIDs := range dp.PodIDs {
		count += len(ctrIDs)
	}
	return count
}

// AvailMilliCpus returns the number of CPUs in a dynamicPool.
func (dp DynamicPool) AvailMilliCpus() int {
	return dp.Cpus.Size() * 1000
}

// updateRealCpuUsed returns cpu utilization of a dynamicPool.
func (p *dynamicPools) updateRealCpuUsed(dp *DynamicPool) (int, error) {
	if dp.Cpus.Size() == 0 {
		log.Info("dynamic pool %s cpuset is 0", dp.Def.Name)
		return 0, nil
	}
	cpuInfo, _ := getCpuUtilization(time.Duration(time.Second))
	cpus := dp.Cpus.ToSlice()
	var sum float64
	for i := 0; i < len(cpus); i++ {
		sum += cpuInfo[cpus[i]]
	}
	res := int(sum / float64(100))

	log.Info("dynamic pool %s cpuset: %s,  cpu utilization: %d", dp.Def.Name, dp.Cpus, res)
	return res, nil
}

// calculateAllPoolWeights returns weights of all dynamicPools and the sum of weights.
// Use dynamicPool's cpu utilization as its weight.
func (p *dynamicPools) calculateAllPoolWeights() (map[*DynamicPool]int, int, error) {
	weight := make(map[*DynamicPool]int)
	sumWeight := 0
	for _, dp := range p.dynamicPools {
		if dp.Def.Name == "reserved" {
			continue
		}
		realCpuUsed, err := p.updateRealCpuUsed(dp)
		if err != nil {
			return weight, sumWeight, dynamicPoolsError("The actual cpu usage of the dynamic pool %s cannot be obtained: %w",
				dp.PrettyName(), err)
		}
		weight[dp] = realCpuUsed
		sumWeight += weight[dp]
		log.Info("dyanmic pool: %s, realCpuUsed: %d, weight: %d", dp, realCpuUsed, weight[dp])
	}
	log.Info("sum weight: %d", sumWeight)
	return weight, sumWeight, nil
}

// calculateAllPoolRequests returns the sum of the requests of containers in each dynamicPool and remaining free cpu.
// remainFree = allowed cpu - reserved cpu - sum(requests of containers in each dynamicPool)
func (p *dynamicPools) calculateAllPoolRequests() (map[*DynamicPool]int, int) {
	requestCpu := make(map[*DynamicPool]int)
	remainFree := p.allowed.Difference(p.reserved).Size()
	for _, dp := range p.dynamicPools {
		if dp.Def.Name == "reserved" {
			continue
		}
		requestCpu[dp] = (p.requestedMinMilliCpus(dp) + 999) / 1000
		remainFree -= requestCpu[dp]
		log.Info("dyanmic pool %s request cpu %d", dp, requestCpu[dp])
	}
	log.Info("sum remain free cpu %d", remainFree)
	return requestCpu, remainFree
}

func (p *dynamicPools) containerPinPool(dp *DynamicPool) {
	dp.Mems = p.closestMems(dp.Cpus)
	for _, cID := range dp.ContainerIDs() {
		if c, ok := p.cch.LookupContainer(cID); ok {
			p.pinCpuMem(c, dp.Cpus, dp.Mems)
		}
	}
}

// updatePoolCpuset updates the cpuset of the dynamicPools.
func (p *dynamicPools) updatePoolCpuset() error {
	requestCpu, remainFree := p.calculateAllPoolRequests()
	weight, sumWeight, err := p.calculateAllPoolWeights()
	if err != nil {
		return err
	}

	if remainFree >= 1 {
		usedCpu := 0
		// Ensure that there is at least one cpu in the shared dynamicPool.
		for _, dp := range p.dynamicPools {
			if dp.Def.Name == "shared" && sumWeight != 0 {
				addCpu := remainFree * weight[dp] / sumWeight
				if requestCpu[dp]+addCpu < 1 {
					requestCpu[dp] = 1
					remainFree -= 1
				}
			}
		}
		for _, dp := range p.dynamicPools {
			if dp.Def.Name == "reserved" {
				continue
			}
			if sumWeight != 0 {
				addCpu := remainFree * weight[dp] / sumWeight
				requestCpu[dp] += addCpu
				usedCpu += addCpu
			}
			log.Info("dyanmic pool %s new request cpu %d, remain free cpu %d", dp, requestCpu[dp], remainFree-usedCpu)
		}
		if usedCpu < remainFree {
			// If there is still cpus, give the dynamicPool with the highest cpu utilization.
			tmp := p.dynamicPools[1]
			for _, dp := range p.dynamicPools {
				if dp.Def.Name == "reserved" {
					continue
				}
				if weight[dp] > weight[tmp] {
					tmp = dp
				}
			}
			requestCpu[tmp] += (remainFree - usedCpu)
			log.Info("dyanmic pool %s new request cpu %d, remain free cpu %d", tmp, requestCpu[tmp], 0)
		}
	}

	// If the number of newly allocated CPUs is the same as the number of existing CPUs in the pool,
	// it means that there is no need to re-allocate
	isNeedReallocate := false
	for _, dp := range p.dynamicPools {
		if dp.Def.Name == "reserved" {
			continue
		}
		if dp.Cpus.Size() != requestCpu[dp] {
			isNeedReallocate = true
			break
		}
	}
	if !isNeedReallocate {
		log.Info("The number of CPUs required by the pools is the same as the number of CPUs already in the pools, so there is no need to reallocate.")
		for _, dp := range p.dynamicPools {
			p.containerPinPool(dp)
		}
		return nil
	}

	for _, dp := range p.dynamicPools {
		if dp.Def.Name == "reserved" {
			continue
		}
		if dp.Cpus.Size() == 0 {
			continue
		}
		p.forgetCpuClass(dp)
		oldCpus := dp.Cpus.Clone()
		keptCpus, err := p.cpuAllocator.ReleaseCpus(&oldCpus, dp.Cpus.Size(), dp.Def.AllocatorPriority)
		if err != nil || keptCpus.Size() != 0 {
			return dynamicPoolsError("releasing %d CPUs from %s failed: %w (kept: %s)", dp.Cpus.Size(), dp, err, keptCpus)
		}
		p.freeCpus = p.freeCpus.Union(dp.Cpus)
	}
	for _, dp := range p.dynamicPools {
		if dp.Def.Name == "reserved" {
			continue
		}
		newCpus, err := p.cpuAllocator.AllocateCpus(&p.freeCpus, requestCpu[dp], dp.Def.AllocatorPriority)
		if err != nil {
			return dynamicPoolsError("allocating %d CPUs for %s failed: %w", requestCpu[dp], dp, err)
		}
		dp.Cpus = newCpus
		log.Debugf("resize successful for container: %s, new Cpus: %#s", dp.PrettyName(), dp.Cpus)
		p.containerPinPool(dp)
		p.useCpuClass(dp)
	}
	return nil
}

// CreateDynamicPoolsPolicy creates a new policy instance.
func CreateDynamicPoolsPolicy(policyOptions *policy.BackendOptions) policy.Backend {
	p := &dynamicPools{
		options:      policyOptions,
		cch:          policyOptions.Cache,
		cpuAllocator: cpuallocator.NewCPUAllocator(policyOptions.System),
	}
	log.Info("creating %s policy...", PolicyName)
	// Handle common policy options: AvailableResources and ReservedResources.
	// p.allowed: CPUs available for the policy
	if allowed, ok := policyOptions.Available[policyapi.DomainCPU]; ok {
		p.allowed = allowed.(cpuset.CPUSet)
	} else {
		// Available CPUs not specified, default to all on-line CPUs.
		p.allowed = policyOptions.System.CPUSet().Difference(policyOptions.System.Offlined())
	}
	// p.reserved: CPUs reserved for kube-system pods, subset of p.allowed.
	p.reserved = cpuset.NewCPUSet()
	if reserved, ok := p.options.Reserved[policyapi.DomainCPU]; ok {
		switch v := reserved.(type) {
		case cpuset.CPUSet:
			p.reserved = p.allowed.Intersection(v)
		case resapi.Quantity:
			reserveCnt := (int(v.MilliValue()) + 999) / 1000
			cpus, err := p.cpuAllocator.AllocateCpus(&p.allowed, reserveCnt, cpuallocator.PriorityNone)
			if err != nil {
				log.Fatal("failed to allocate reserved CPUs: %s", err)
			}
			p.reserved = cpus
			p.allowed = p.allowed.Union(cpus)
		}
	}
	if p.reserved.IsEmpty() {
		log.Fatal("%s cannot run without reserved CPUs that are also AvailableResources", PolicyName)
	}
	// Handle policy-specific options
	log.Debug("creating %s configuration", PolicyName)
	if err := p.setConfig(dynamicPoolsOptions); err != nil {
		log.Fatal("failed to create %s policy: %v", PolicyName, err)
	}
	pkgcfg.GetModule(PolicyPath).AddNotify(p.configNotify)

	return p
}

// Name returns the name of this policy.
func (p *dynamicPools) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (p *dynamicPools) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (p *dynamicPools) Start(add []cache.Container, del []cache.Container) error {
	log.Info("%s policy started", PolicyName)
	return p.Sync(p.cch.GetContainers(), nil)
}

// Sync synchronizes the active policy state.
func (p *dynamicPools) Sync(add []cache.Container, del []cache.Container) error {
	log.Debug("synchronizing state...")
	for _, c := range del {
		p.ReleaseResources(c)
	}
	for _, c := range add {
		p.AllocateResources(c)
	}
	return nil
}

// AllocateResources is a resource allocation request for this policy.
func (p *dynamicPools) AllocateResources(c cache.Container) error {
	log.Debug("allocating resources for container %s...", c.PrettyName())
	dp, err := p.allocateDynamicPool(c)
	if err != nil {
		return dynamicPoolsError("dynamicPool allocation for container %s failed: %w", c.PrettyName(), err)
	}
	if dp == nil {
		return dynamicPoolsError("no suitable dynamicPools found for container %s", c.PrettyName())
	}
	log.Info("assigning container %s to dynamicPool %s", c.PrettyName(), dp)
	podID := c.GetPodID()
	dp.PodIDs[podID] = append(dp.PodIDs[podID], c.GetCacheID())
	if dp.Cpus.Equals(p.reserved) {
		p.assignContainer(c, dp)
		log.Debugf("if dynamic pool is reserved, do not updatePoolCpuset.")
	} else {
		p.updatePoolCpuset()
	}
	if log.DebugEnabled() {
		log.Debug(p.dumpDynamicPool(dp))
	}
	return nil
}

// ReleaseResources is a resource release request for this policy.
func (p *dynamicPools) ReleaseResources(c cache.Container) error {
	log.Debug("releasing container %s...", c.PrettyName())
	dp := p.dynamicPoolByContainer(c)
	if dp == nil {
		log.Debug("ReleaseResources: dynamicPool-less container %s, nothing to release", c.PrettyName())
		return nil
	}
	p.dismissContainer(c, dp)
	if dp.Cpus.Equals(p.reserved) {
		log.Debugf("if dynamic pool is reserved, do not updatePoolCpuset.")
	} else {
		p.updatePoolCpuset()
	}
	if log.DebugEnabled() {
		log.Debug(p.dumpDynamicPool(dp))
	}
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (p *dynamicPools) UpdateResources(c cache.Container) error {
	log.Debug("(not) updating container %s...", c.PrettyName())
	return nil
}

// Rebalance tries to find an optimal allocation of resources for the current containers.
func (p *dynamicPools) Rebalance() (bool, error) {
	log.Debug("rebalancing containers...")
	err := p.updatePoolCpuset()
	return true, err
}

// HandleEvent handles policy-specific events.
func (p *dynamicPools) HandleEvent(*events.Policy) (bool, error) {
	log.Debug("(not) handling event...")
	return false, nil
}

// ExportResourceData provides resource data to export for the container.
func (p *dynamicPools) ExportResourceData(c cache.Container) map[string]string {
	return nil
}

// Introspect provides data for external introspection.
func (p *dynamicPools) Introspect(*introspect.State) {
	return
}

// dynamicPoolByContainer returns a dynamicPool that contains a container.
func (p *dynamicPools) dynamicPoolByContainer(c cache.Container) *DynamicPool {
	podID := c.GetPodID()
	cID := c.GetCacheID()
	for _, dp := range p.dynamicPools {
		for _, ctrID := range dp.PodIDs[podID] {
			if ctrID == cID {
				return dp
			}
		}
	}
	return nil
}

// dynamicPoolsByDef returns a dynamicPool instantiated from a dynamicPool definition.
func (p *dynamicPools) dynamicPoolByDef(dpDef *DynamicPoolDef) *DynamicPool {
	for _, dp := range p.dynamicPools {
		if dp.Def == dpDef {
			return dp
		}
	}
	return nil
}

// dynamicPoolDefByName returns a dynamicPool definition with a name.
func (p *dynamicPools) dynamicPoolDefByName(defName string) *DynamicPoolDef {
	if defName == "reserved" {
		return p.reservedDynamicPoolDef
	}
	if defName == "default" {
		return p.defaultDynamicPoolDef
	}
	for _, dpDef := range p.dpoptions.DynamicPoolDefs {
		if dpDef.Name == defName {
			return dpDef
		}
	}
	return nil
}

// chooseDynamicPoolDef returns the dynamicPoolDef selected by the container
func (p *dynamicPools) chooseDynamicPoolDef(c cache.Container) (*DynamicPoolDef, error) {
	var dpDef *DynamicPoolDef
	// If the requests and limits of container are 0, they are assigned to the default dynamicPool.
	if !namespaceMatches(c.GetNamespace(), append(p.dpoptions.ReservedPoolNamespaces, metav1.NamespaceSystem)) &&
		p.containerRequestedMilliCpus(c.GetCacheID()) == 0 && p.containerLimitedMilliCpus(c.GetCacheID()) == 0 {
		return p.defaultDynamicPoolDef, nil
	}

	// DynamicPoolDef is defined by annotation?
	if dpDefName, ok := c.GetEffectiveAnnotation(dynamicPoolKey); ok {
		dpDef = p.dynamicPoolDefByName(dpDefName)
		if dpDef == nil {
			return nil, dynamicPoolsError("no dynamicPool for annotation %q", dpDefName)
		}
		return dpDef, nil
	}

	// DynamicPoolDef is defined by a special namespace (kube-system +
	// ReservedPoolNamespaces)?
	if namespaceMatches(c.GetNamespace(), append(p.dpoptions.ReservedPoolNamespaces, metav1.NamespaceSystem)) {
		return p.dynamicPools[0].Def, nil
	}

	// DynamicPoolDef is defined by the namespace?
	for _, dpDef := range append([]*DynamicPoolDef{p.reservedDynamicPoolDef, p.defaultDynamicPoolDef},
		p.dpoptions.DynamicPoolDefs...) {
		if namespaceMatches(c.GetNamespace(), dpDef.Namespaces) {
			return dpDef, nil
		}
	}
	// Fallback to the default dynamicPool.
	return p.defaultDynamicPoolDef, nil
}

func (p *dynamicPools) containerRequestedMilliCpus(contID string) int {
	cont, ok := p.cch.LookupContainer(contID)
	if !ok {
		return 0
	}
	reqCpu, ok := cont.GetResourceRequirements().Requests[corev1.ResourceCPU]
	if !ok {
		return 0
	}
	return int(reqCpu.MilliValue())
}

func (p *dynamicPools) containerLimitedMilliCpus(contID string) int {
	cont, ok := p.cch.LookupContainer(contID)
	if !ok {
		return 0
	}
	limitCpu, ok := cont.GetResourceRequirements().Limits[corev1.ResourceCPU]
	if !ok {
		return 0
	}
	return int(limitCpu.MilliValue())
}

// requestedMaxMilliCpus sums up and returns CPU limits of all
// containers assigned to a dynamicPool.
func (p *dynamicPools) requestedMaxMilliCpus(dp *DynamicPool) int {
	cpuRequested := 0
	for _, cID := range dp.ContainerIDs() {
		cpuRequested += p.containerLimitedMilliCpus(cID)
	}
	return cpuRequested
}

// requestedMinMilliCpus sums up and returns CPU requests of all
// containers assigned to a dynamicPool.
func (p *dynamicPools) requestedMinMilliCpus(dp *DynamicPool) int {
	cpuRequested := 0
	for _, cID := range dp.ContainerIDs() {
		cpuRequested += p.containerRequestedMilliCpus(cID)
	}
	return cpuRequested
}

// resetCpuClass resets CPU configurations globally. All dynamicPools can
// be ignored, their CPU configurations will be applied later.
func (p *dynamicPools) resetCpuClass() error {
	// Usual inputs:
	// - p.allowed (cpuset.CPUset): all CPUs available for this
	//   policy.
	// - p.IdleCpuClass (string): CPU class for allowed CPUs.
	//
	// Other inputs, if needed:
	// - p.reserved (cpuset.CPUset): CPUs of ReservedResources
	//   (typically for kube-system containers).
	//
	// Note: p.useCpuClass(dynamicPool) will be called before assigning
	// containers on the dynamicPool, including the reserved dynamicPool.
	cpucontrol.Assign(p.cch, p.dpoptions.IdleCpuClass, p.allowed.ToSliceNoSort()...)
	log.Debugf("resetCpuClass available: %s; reserved: %s", p.allowed, p.reserved)
	return nil
}

// useCpuClass configures CPUs of a dynamicPool.
func (p *dynamicPools) useCpuClass(dp *DynamicPool) error {
	// Usual inputs:
	// - CPUs that cpuallocator has reserved for this dynamicPool:
	//   dp.Cpus (cpuset.CPUSet).
	// - User-defined CPU configuration for CPUs of dynamicPool of this type:
	//   dp.Def.CpuClass (string).
	// - Current configuration(?): feel free to add data
	//   structure for this. For instance policy-global p.cpuConfs,
	//   or dynamicPool-local dp.cpuConfs.
	//
	// Other input examples, if needed:
	// - Requested CPU resources by all containers in the dynamicPool:
	//   p.requestedMilliCpus(dp).
	// - Free CPU resources in the dynamicPool: p.freeMilliCpus(dp).
	// - Number of assigned containers: dp.ContainerCount().
	// - Container details: access p.cch with dp.ContainerIDs().
	// - User-defined CPU AllocatorPriority: dp.Def.AllocatorPriority.
	// - All existing dynamicPool instances: p.dynamicPools.
	// - CPU configurations by user: dp.Def.CpuClass (for dp in p.dynamicPools)
	cpucontrol.Assign(p.cch, dp.Def.CpuClass, dp.Cpus.ToSliceNoSort()...)
	log.Debugf("useCpuClass Cpus: %s; CpuClass: %s", dp.Cpus, dp.Def.CpuClass)
	return nil
}

// forgetCpuClass is called when CPUs of a dynamicPool are released from duty.
func (p *dynamicPools) forgetCpuClass(dp *DynamicPool) {
	// Use p.IdleCpuClass for dp.Cpus.
	// Usual inputs: see useCpuClass
	// cpucontrol.Assign(p.cch, p.dpoptions.IdleCpuClass, dp.Cpus.ToSliceNoSort()...)
	// Release CPUs to dafault dynamicPool.
	cpucontrol.Assign(p.cch, p.dpoptions.IdleCpuClass, dp.Cpus.ToSliceNoSort()...)
	log.Debugf("forgetCpuClass Cpus: %s; CpuClass: %s", dp.Cpus, dp.Def.CpuClass)
}

func (p *dynamicPools) newDynamicPool(dpDef *DynamicPoolDef, confCpus bool) (*DynamicPool, error) {
	var cpus cpuset.CPUSet
	var err error
	if dpDef == p.reservedDynamicPoolDef {
		cpus = p.reserved
	} else {
		cpus, err = p.cpuAllocator.AllocateCpus(&p.freeCpus, 0, dpDef.AllocatorPriority)

		if err != nil {
			return nil, dynamicPoolsError("could not allocate Cpus for dynamicPool %s: %w", dpDef.Name, err)
		}
	}
	dp := &DynamicPool{
		Def:    dpDef,
		PodIDs: make(map[string][]string),
		Cpus:   cpus,
		Mems:   p.closestMems(cpus),
	}
	if confCpus {
		if err = p.useCpuClass(dp); err != nil {
			log.Errorf("failed to apply CPU configuration to new dynamicPool %s (cpus: %s): %w", dpDef.Name, cpus, err)
			return nil, err
		}
	}
	return dp, nil
}

func namespaceMatches(namespace string, patterns []string) bool {
	for _, pattern := range patterns {
		ret, err := filepath.Match(pattern, namespace)
		if err == nil && ret {
			return true
		}
	}
	return false
}

// allocateDynamicPool returns a dynamicPool allocated for a container.
func (p *dynamicPools) allocateDynamicPool(c cache.Container) (*DynamicPool, error) {
	dpDef, err := p.chooseDynamicPoolDef(c)
	if err != nil {
		return nil, err
	}
	if dpDef == nil {
		return nil, dynamicPoolsError("no applicable dynamicPool type found")
	}
	dynamicPool := p.dynamicPoolByDef(dpDef)
	if dynamicPool == nil {
		return nil, dynamicPoolsError("no suitable dynamicPool instance available")
	}
	return dynamicPool, err
}

// dumpDynamicPool dumps dynamicPool contents in detail.
func (p *dynamicPools) dumpDynamicPool(dp *DynamicPool) string {
	conts := []string{}
	pods := []string{}
	for podID, contIDs := range dp.PodIDs {
		podName := podID
		if pod, ok := p.cch.LookupPod(podID); ok {
			podName = pod.GetName()
		}
		pods = append(pods, podName)
		for _, contID := range contIDs {
			if cont, ok := p.cch.LookupContainer(contID); ok {
				conts = append(conts, cont.PrettyName())
			} else {
				conts = append(conts, podName+"."+contID)
			}
		}
	}
	s := fmt.Sprintf("DynamicPool %s{Cpus: %s; Mems: %s; mCPU requests: %d; mCPU limits: %d; capacity: %d; pods: %s; conts: %s}",
		dp.PrettyName(),
		dp.Cpus,
		dp.Mems,
		p.requestedMinMilliCpus(dp),
		p.requestedMaxMilliCpus(dp),
		dp.AvailMilliCpus(),
		pods,
		conts)
	return s
}

// changesDynamicPools returns true if two dynamicPools policy configurations
// may lead into different dynamicPools or workload assignment.
func changesDynamicPools(opts0, opts1 *DynamicPoolsOptions) bool {
	if opts0 == nil && opts1 == nil {
		return false
	}
	if opts0 == nil || opts1 == nil {
		return true
	}
	if len(opts0.DynamicPoolDefs) != len(opts1.DynamicPoolDefs) {
		return true
	}
	o0 := opts0.DeepCopy()
	o1 := opts1.DeepCopy()
	// Ignore differences in CPU class names. Every other change
	// potentially changes dynamicPools or workloads.
	o0.IdleCpuClass = ""
	o1.IdleCpuClass = ""
	for i := range o0.DynamicPoolDefs {
		o0.DynamicPoolDefs[i].CpuClass = ""
		o1.DynamicPoolDefs[i].CpuClass = ""
	}
	return utils.DumpJSON(o0) != utils.DumpJSON(o1)
}

// changesCpuClasses returns true if two dynamicPools policy
// configurations can lead to using different CPU classes on
// corresponding dynamicPool instances. Calling changesCpuClasses(o0, o1)
// makes sense only if changesDynamicPools(o0, o1) has returned false.
func changesCpuClasses(opts0, opts1 *DynamicPoolsOptions) bool {
	if opts0 == nil && opts1 == nil {
		return false
	}
	if opts0 == nil || opts1 == nil {
		return true
	}
	if opts0.IdleCpuClass != opts1.IdleCpuClass {
		return true
	}
	if len(opts0.DynamicPoolDefs) != len(opts1.DynamicPoolDefs) {
		return true
	}
	for i := range opts0.DynamicPoolDefs {
		if opts0.DynamicPoolDefs[i].CpuClass != opts1.DynamicPoolDefs[i].CpuClass {
			return true
		}
	}
	return false
}

// configNotify applies new configuration.
func (p *dynamicPools) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	log.Info("configuration %s", event)
	defer log.Debug("effective configuration:\n%s\n", utils.DumpJSON(p.dpoptions))
	newDynamicPoolsOptions := dynamicPoolsOptions.DeepCopy()
	if !changesDynamicPools(&p.dpoptions, newDynamicPoolsOptions) {
		if !changesCpuClasses(&p.dpoptions, newDynamicPoolsOptions) {
			log.Info("no configuration changes")
		} else {
			log.Info("configuration changes only on CPU classes")
			// Update new CPU classes to existing DynamicPool
			// definitions. The same DynamicPoolDef instances
			// must be kept in use, because each dynamicPool
			// instance holds a direct reference to its
			// DynamicPoolDef.
			for i := range p.dpoptions.DynamicPoolDefs {
				p.dpoptions.DynamicPoolDefs[i].CpuClass = newDynamicPoolsOptions.DynamicPoolDefs[i].CpuClass
			}
			// (Re)configures all CPUs in DynamicPools.
			p.resetCpuClass()
			for _, dp := range p.dynamicPools {
				p.useCpuClass(dp)
			}
		}
		return nil
	}
	if err := p.setConfig(newDynamicPoolsOptions); err != nil {
		log.Error("config update failed: %v", err)
		return err
	}
	log.Info("config updated successfully")
	p.Sync(p.cch.GetContainers(), p.cch.GetContainers())
	return nil
}

// applyDynamicPoolDef creates user-defined dynamicPools or reconfigures built-in
// dynamicPools according to the dpDef. Does not initialize dynamicPool CPUs.
func (p *dynamicPools) applyDynamicPoolDef(dynamicPools *[]*DynamicPool, dpDef *DynamicPoolDef) error {
	if len(*dynamicPools) < 2 {
		return dynamicPoolsError("internal error: reserved and default dynamicPools missing, cannot apply dynamicPool definitions")
	}
	reservedDynamicPool := (*dynamicPools)[0]
	defaultDynamicPool := (*dynamicPools)[1]
	// Every dynamicPoolDef does one of the following:
	// 1. reconfigures the "reserved" dynamicPool (most restricted)
	// 2. reconfigures the "default" dynamicPool (somewhat restricted)
	// 3. defines new user-defined dynamicPool.
	switch dpDef.Name {
	case "":
		// Case 0: bad name
		return dynamicPoolsError("undefined or empty dynamicPool name")
	case reservedDynamicPool.Def.Name:
		// Case 1: reconfigure the "reserved" dynamicPool.
		p.reservedDynamicPoolDef.AllocatorPriority = dpDef.AllocatorPriority
		p.reservedDynamicPoolDef.CpuClass = dpDef.CpuClass
		p.reservedDynamicPoolDef.Namespaces = dpDef.Namespaces
	case defaultDynamicPool.Def.Name:
		// Case 2: reconfigure the "default" dynamicPool.
		p.defaultDynamicPoolDef.AllocatorPriority = dpDef.AllocatorPriority
		p.defaultDynamicPoolDef.CpuClass = dpDef.CpuClass
		p.defaultDynamicPoolDef.Namespaces = dpDef.Namespaces
	default:
		// Case 3: create each user-defined dynamicPool without CPU.
		newdp, err := p.newDynamicPool(dpDef, false)
		if err != nil {
			return err
		}
		*dynamicPools = append(*dynamicPools, newdp)
	}
	return nil
}

// setConfig takes new dynamicPool configuration into use.
func (p *dynamicPools) setConfig(dpoptions *DynamicPoolsOptions) error {
	// Create the default reserved and default dynamicPool
	// definitions. Some properties of these definitions may be
	// altered by user configuration.
	p.reservedDynamicPoolDef = &DynamicPoolDef{
		Name:              reservedDynamicPoolDefName,
		AllocatorPriority: 3,
	}
	p.defaultDynamicPoolDef = &DynamicPoolDef{
		Name:              defaultDynamicPoolDefName,
		AllocatorPriority: 3,
	}
	p.dynamicPools = []*DynamicPool{}
	p.freeCpus = p.allowed.Clone()
	p.freeCpus = p.freeCpus.Difference(p.reserved)
	// Instantiate built-in reserved and default dynamicPool.
	reservedDynamicPool, err := p.newDynamicPool(p.reservedDynamicPoolDef, false)
	if err != nil {
		return err
	}
	p.dynamicPools = append(p.dynamicPools, reservedDynamicPool)
	defaultDynamicPool, err := p.newDynamicPool(p.defaultDynamicPoolDef, false)
	if err != nil {
		return err
	}
	p.dynamicPools = append(p.dynamicPools, defaultDynamicPool)
	// First apply customizations to built-in dynamicPools: "reserved"
	// and "default".
	for _, dpDef := range dpoptions.DynamicPoolDefs {
		if dpDef.Name != reservedDynamicPoolDefName && dpDef.Name != defaultDynamicPoolDefName {
			continue
		}
		if err := p.applyDynamicPoolDef(&p.dynamicPools, dpDef); err != nil {
			return err
		}
	}
	// Apply all user dynamicPool definitions, skip already customized
	// "reserved" and "default" dynamicPools.
	for _, dpDef := range dpoptions.DynamicPoolDefs {
		if dpDef.Name == reservedDynamicPoolDefName || dpDef.Name == defaultDynamicPoolDefName {
			continue
		}
		if err := p.applyDynamicPoolDef(&p.dynamicPools, dpDef); err != nil {
			return err
		}
	}
	// Finish dynamicPool initialization.
	log.Info("%s policy dynamicPools:", PolicyName)
	for dpIdx, dp := range p.dynamicPools {
		log.Info("- dynamicPool %d: %s", dpIdx, dp)
	}
	// No errors in dynamicPool creation, take new configuration into use.
	p.dpoptions = *dpoptions
	// (Re)configures all CPUs in dynamicPools.
	p.resetCpuClass()
	for _, dp := range p.dynamicPools {
		p.useCpuClass(dp)
	}
	return nil
}

// closestMems returns memory node IDs good for pinning containers
// that run on given CPUs.
func (p *dynamicPools) closestMems(cpus cpuset.CPUSet) idset.IDSet {
	mems := idset.NewIDSet()
	sys := p.options.System
	for _, nodeID := range sys.NodeIDs() {
		if !cpus.Intersection(sys.Node(nodeID).CPUSet()).IsEmpty() {
			mems.Add(nodeID)
		}
	}
	return mems
}

// assignContainer adds a container to a dynamicPool.
func (p *dynamicPools) assignContainer(c cache.Container, dp *DynamicPool) {
	log.Info("assigning container %s to dynamicPool %s", c.PrettyName(), dp)
	podID := c.GetPodID()
	dp.PodIDs[podID] = append(dp.PodIDs[podID], c.GetCacheID())
	p.pinCpuMem(c, dp.Cpus, dp.Mems)
}

// dismissContainer removes a container from a dynamicPool.
func (p *dynamicPools) dismissContainer(c cache.Container, dp *DynamicPool) {
	podID := c.GetPodID()
	dp.PodIDs[podID] = removeString(dp.PodIDs[podID], c.GetCacheID())
	if len(dp.PodIDs[podID]) == 0 {
		delete(dp.PodIDs, podID)
	}
}

// pinCpuMem pins container to CPUs and memory nodes if flagged.
func (p *dynamicPools) pinCpuMem(c cache.Container, cpus cpuset.CPUSet, mems idset.IDSet) {
	if p.dpoptions.PinCPU == nil || *p.dpoptions.PinCPU {
		log.Debug("  - pinning %s to cpuset: %s", c.PrettyName(), cpus)
		c.SetCpusetCpus(cpus.String())
		if reqCpu, ok := c.GetResourceRequirements().Requests[corev1.ResourceCPU]; ok {
			mCpu := int(reqCpu.MilliValue())
			c.SetCPUShares(int64(cache.MilliCPUToShares(mCpu)))
		}
	}
	if p.dpoptions.PinMemory == nil || *p.dpoptions.PinMemory {
		log.Debug("  - pinning %s to memory %s", c.PrettyName(), mems)
		c.SetCpusetMems(mems.String())
	}
}

// dynamicPoolsError formats an error from this policy.
func dynamicPoolsError(format string, args ...interface{}) error {
	return fmt.Errorf(PolicyName+": "+format, args...)
}

// removeString returns the first occurrence of a string from string slice.
func removeString(strings []string, element string) []string {
	for index, s := range strings {
		if s == element {
			strings[index] = strings[len(strings)-1]
			return strings[:len(strings)-1]
		}
	}
	return strings
}

// Register us as a policy implementation.
func init() {
	policy.Register(PolicyName, PolicyDescription, CreateDynamicPoolsPolicy)
}
