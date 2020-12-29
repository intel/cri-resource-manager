// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package podpools

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/introspect"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	policyapi "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

const (
	// PolicyName is the name used to activate this policy.
	PolicyName = "podpools"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "Pod-granularity workload placement"
	// PolicyPath is the path of this policy in the configuration hierarchy.
	PolicyPath = "policy." + PolicyName
	// poolTypeKey is a pod annotation key, the value is a pool type.
	poolTypeKey = "pooltype." + PolicyName + "." + kubernetes.ResmgrKeyNamespace
	// reservedPoolTypeName is the name of the pool type of reserved CPUs.
	reservedPoolTypeName = "reserved"
	// sharedPoolTypeName is the name of the pool type of shared CPUs.
	sharedPoolTypeName = "shared"
)

type podpools struct {
	options          *policyapi.BackendOptions
	ppoptions        PodpoolsOptions
	cch              cache.Cache
	allowed          cpuset.CPUSet             // bounding set of CPUs we're allowed to use
	reserved         cpuset.CPUSet             // system-/kube-reserved CPUs
	reservedPoolType *PoolType                 // built-in type of pool of reserved CPUs
	sharedPoolType   *PoolType                 // built-in type of pool of shared CPUs
	pools            []*Pool                   // pools for pods, reserved, shared and user-defined
	cpuAllocator     cpuallocator.CPUAllocator // CPU allocator used by the policy
}

type Pool struct {
	Type            *PoolType
	Instance        int
	CPUs            cpuset.CPUSet
	Mems            sysfs.IDSet
	FullPodCapacity int
	// PodIDs maps pod ID to list of container IDs
	// len(PodIDs) is the number of pods in the pool.
	// len(PodIDs[podID]) is the number of containers of podID
	// currently assigned to the pool.
	// FullPodCapacity - len(PodIDs) is free pod capacity.
	PodIDs map[string][]string
}

var log logger.Logger = logger.NewLogger("policy")
var _ policy.Backend = &podpools{}

func (pool Pool) String() string {
	podCount := len(pool.PodIDs)
	contCount := 0
	for _, contIDs := range pool.PodIDs {
		contCount += len(contIDs)
	}
	s := fmt.Sprintf("%s{capacity.pod: %d, pods:%d, containers:%d}",
		pool.PrettyName(), pool.FullPodCapacity, podCount, contCount)
	return s
}

func (pool Pool) PrettyName() string {
	return fmt.Sprintf("%s[%d]", pool.Type.Name, pool.Instance)
}

// CreatePodpoolsPolicy creates a new policy instance.
func CreatePodpoolsPolicy(policyOptions *policy.BackendOptions) policy.Backend {
	p := &podpools{
		options: policyOptions,
		cch:     policyOptions.Cache,
		reservedPoolType: &PoolType{
			Name:      reservedPoolTypeName,
			FillOrder: FillBalanced,
		},
		sharedPoolType: &PoolType{
			Name:      sharedPoolTypeName,
			FillOrder: FillBalanced,
		},
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
			cpus, err := p.cpuAllocator.AllocateCpus(&p.allowed, reserveCnt, false)
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
	if err := p.setConfig(podpoolsOptions); err != nil {
		log.Fatal("failed to create %s policy: %v", PolicyName, err)
	}

	pkgcfg.GetModule(PolicyPath).AddNotify(p.configNotify)

	return p
}

// Name returns the name of this policy.
func (p *podpools) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (p *podpools) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (p *podpools) Start(add []cache.Container, del []cache.Container) error {
	log.Info("%s policy started", PolicyName)
	return nil
}

// Sync synchronizes the active policy state.
func (p *podpools) Sync(add []cache.Container, del []cache.Container) error {
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
func (p *podpools) AllocateResources(c cache.Container) error {
	log.Debug("allocating container %s...", c.PrettyName())
	// Assign container to correct pool.
	if pool := p.allocatePool(c); pool != nil {
		p.assignContainer(c, pool)
	} else {
		// Cannot assign container to any of the pooled CPUs.
		return podpoolsError("cannot find CPUs to run container %s - no shared or reserved CPUs available")
	}
	return nil
}

// ReleaseResources is a resource release request for this policy.
func (p *podpools) ReleaseResources(c cache.Container) error {
	pool := p.allocatedPool(c)
	if pool == nil {
		log.Debug("ReleaseResources: pool-less container %s, nothing to release", c.PrettyName())
		return nil
	}
	log.Debug("releasing container %s from pool %s", c, pool)
	podID := c.GetPodID()
	pool.PodIDs[podID] = removeString(pool.PodIDs[podID], c.GetCacheID())
	if len(pool.PodIDs[podID]) == 0 {
		log.Debug("all containers removed, free pool allocation %s", pool.PrettyName())
		// All containers of the pod removed from the pool.
		// Free the pool for other pods.
		delete(pool.PodIDs, podID)
	}
	log.Debug("pool after release: %s", pool)
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (p *podpools) UpdateResources(c cache.Container) error {
	log.Debug("(not) updating container %s...", c.PrettyName())
	return nil
}

// Rebalance tries to find an optimal allocation of resources for the current containers.
func (p *podpools) Rebalance() (bool, error) {
	log.Debug("(not) rebalancing containers...")
	return false, nil
}

// HandleEvent handles policy-specific events.
func (p *podpools) HandleEvent(*events.Policy) (bool, error) {
	log.Debug("(not) handling event...")
	return false, nil
}

// ExportResourceData provides resource data to export for the container.
func (p *podpools) ExportResourceData(c cache.Container) map[string]string {
	return nil
}

// Introspect provides data for external introspection.
func (p *podpools) Introspect(*introspect.State) {
	return
}

// allocatedPool returns a pool already allocated for the pod of a container.
func (p *podpools) allocatedPool(c cache.Container) *Pool {
	podID := c.GetPodID()
	pools := filterPools(p.pools,
		func(pl *Pool) bool { _, ok := pl.PodIDs[podID]; return ok })
	if len(pools) == 0 {
		return nil
	}
	return pools[0]
}

// allocatePool returns a pool allocated for the pod of a container.
func (p *podpools) allocatePool(c cache.Container) *Pool {
	if pool := p.allocatedPool(c); pool != nil {
		return pool
	}
	poolType := p.getPoolType(c)
	if poolType == nil {
		return nil
	}
	// Try to find a suitable pool and allocate it for the pod of
	// the container.
	podID := c.GetPodID()
	// A pool must be of correct type and have capacity left for at least one pod.
	pools := filterPools(p.pools,
		func(pl *Pool) bool {
			return poolType.Name == pl.Type.Name && (pl.FullPodCapacity > len(pl.PodIDs) || pl.FullPodCapacity == -1)
		})
	// Sort pools according to pool type fill order so that the
	// first pool in the list is the preferred one.
	switch poolType.FillOrder {
	case FillBalanced:
		sort.Slice(pools, func(i, j int) bool {
			return len(pools[i].PodIDs) < len(pools[j].PodIDs)
		})
	case FillPacked:
		sort.Slice(pools, func(i, j int) bool {
			return len(pools[i].PodIDs) > len(pools[j].PodIDs)
		})
	case FillFirstFree:
		// FirstFree is already the first of the pools list.
	}
	if len(pools) == 0 {
		log.Warn("cannot find free pool of type %q needed by container %s", poolType.Name, c.PrettyName())
		return nil
	}
	// Found a suitable pool. Allocate it for the pod.
	pool := pools[0]
	pool.PodIDs[podID] = []string{}
	log.Debug("allocated pool %s[%d] for container %s", pool.Type.Name, pool.Instance, c.PrettyName())
	return pool
}

func (p *podpools) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	log.Info("configuration %s", event)
	if err := p.setConfig(podpoolsOptions); err != nil {
		log.Error("config update failed: %v", err)
		return err
	}
	log.Info("config updated successfully")
	return nil
}

// getPoolTypeName returns the name of the pool type for a container.
func (p *podpools) getPoolTypeName(c cache.Container) string {
	if poolTypeName, ok := c.GetEffectiveAnnotation(poolTypeKey); ok {
		return poolTypeName
	}
	if c.GetNamespace() == "kube-system" {
		return reservedPoolTypeName
	}
	return sharedPoolTypeName
}

// getPoolType returns the type of the pool for a container.
func (p *podpools) getPoolType(c cache.Container) *PoolType {
	poolTypeName := p.getPoolTypeName(c)
	if poolTypeName == reservedPoolTypeName {
		return p.reservedPoolType
	}
	if poolTypeName == sharedPoolTypeName {
		return p.sharedPoolType
	}
	for _, poolType := range p.ppoptions.PoolTypes {
		if poolType.Name == poolTypeName {
			return poolType
		}
	}
	return nil
}

// setConfig takes new pool configuration into use.
func (p *podpools) setConfig(ppoptions *PodpoolsOptions) error {
	// Instantiate pools for pods
	p.pools = []*Pool{}
	reservedPool := Pool{
		Type:            p.reservedPoolType,
		Instance:        0,
		CPUs:            p.reserved,
		Mems:            p.closestMems(p.reserved),
		FullPodCapacity: -1,
		PodIDs:          make(map[string][]string),
	}
	p.pools = append(p.pools, &reservedPool)
	// The shared pool used reserved CPUs by default. If CPUs are
	// left over after constructing user-defined pools, those will
	// be used as shared instead.
	sharedPool := Pool{
		Type:            p.sharedPoolType,
		Instance:        0,
		CPUs:            reservedPool.CPUs,
		Mems:            reservedPool.Mems,
		FullPodCapacity: -1,
		PodIDs:          make(map[string][]string),
	}
	p.pools = append(p.pools, &sharedPool)
	// Create user-defined pools
	freeCpus := p.allowed.Clone()
	freeCpus = freeCpus.Difference(p.reserved)
	nonReservedCpuCount := freeCpus.Size()
	poolCpus := cpuset.NewCPUSet()
	for _, poolType := range ppoptions.PoolTypes {
		cpusPerPoolType, err := parseCountOrPercentage(poolType.TypeResources.CPU, nonReservedCpuCount)
		if err != nil {
			return podpoolsError("invalid typeResources.cpu %q in pool %q: %w",
				poolType.TypeResources.CPU, poolType.Name, err)
		}
		cpusPerPool, err := parseCountOrPercentage(poolType.Resources.CPU, cpusPerPoolType)
		if err != nil {
			return podpoolsError("invalid resources.cpu %q in pool %q: %w",
				poolType.Resources.CPU, poolType.Name, err)
		}
		poolCount := cpusPerPoolType / cpusPerPool
		log.Debug("allocating at most %d out of %d non-reserved CPUs for %d pools of type %q",
			cpusPerPoolType, freeCpus.Size(), poolCount, poolType.Name)
		for poolIndex := 0; poolIndex < poolCount; poolIndex++ {
			if cpusPerPool > freeCpus.Size() {
				return podpoolsError("ran out of CPUs when trying to allocate %d CPUs for pool %s[%d]",
					cpusPerPool, poolType.Name, poolIndex)
			}
			cpus, err := p.cpuAllocator.AllocateCpus(&freeCpus, cpusPerPool, false)
			if err != nil {
				return podpoolsError("could not allocate %d CPUs for pool %d of poolType %q: %w",
					cpusPerPool, poolIndex, poolType.Name, err)
			}
			mems := p.closestMems(cpus)
			pool := Pool{
				Type:            poolType,
				Instance:        poolIndex,
				CPUs:            cpus,
				Mems:            mems,
				FullPodCapacity: poolType.Capacity.Pod,
				PodIDs:          make(map[string][]string),
			}
			p.pools = append(p.pools, &pool)
			poolCpus = poolCpus.Union(cpus)
		}
	}
	if freeCpus.Size() > 0 {
		log.Debug("%d unused CPUs are used as the shared pool.", freeCpus.Size())
		p.pools[1].CPUs = freeCpus
		p.pools[1].Mems = p.closestMems(freeCpus)
	} else {
		log.Debug("All non-reserved CPUs allocated to user-defined pools. Shared and reserved pools run on the same CPUs.")
	}
	log.Debug("%s policy pools:", PolicyName)
	for index, pool := range p.pools {
		log.Info("- pool %d: %s[%d] cpuset: %s, mem nodes: %s, pod capacity: %d",
			index, pool.Type.Name, pool.Instance, pool.CPUs, pool.Mems, pool.FullPodCapacity)
	}
	log.Debug("new %s configuration:\n%s", PolicyName, utils.DumpJSON(ppoptions))
	p.ppoptions = *ppoptions
	return nil
}

// closestMems returns memory node IDs good for pinning containers
// that run on given CPUs
func (p *podpools) closestMems(cpus cpuset.CPUSet) sysfs.IDSet {
	mems := sysfs.NewIDSet()
	sys := p.options.System
	for _, nodeID := range sys.NodeIDs() {
		if !cpus.Intersection(sys.Node(nodeID).CPUSet()).IsEmpty() {
			mems.Add(nodeID)
		}
	}
	return mems
}

// filterPools returns pools for which the test function returns true
func filterPools(pools []*Pool, test func(*Pool) bool) (ret []*Pool) {
	for _, pool := range pools {
		if test(pool) {
			ret = append(ret, pool)
		}
	}
	return
}

// parseCountOrPercentage parses number or percentage from string
func parseCountOrPercentage(s string, maxCount int) (int, error) {
	var n int
	if strings.HasSuffix(s, "%") {
		ps := strings.TrimSpace(strings.TrimSuffix(s, "%"))
		n64, err := strconv.ParseInt(ps, 0, 32)
		if err != nil {
			return 0, err
		}
		n = int(n64) * maxCount / 100
	} else {
		n64, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return 0, err
		}
		n = int(n64)
	}
	if n > maxCount {
		return 0, podpoolsError("value (%d) exceeds maximum (%d)", n, maxCount)
	}
	return n, nil
}

func (p *podpools) assignContainer(c cache.Container, pool *Pool) {
	log.Info("assigning container %s to pool %s[%d]", c.PrettyName(), pool.Type.Name, pool.Instance)
	podID := c.GetPodID()
	pool.PodIDs[podID] = append(pool.PodIDs[podID], c.GetCacheID())
	p.pinCpuMem(c, pool.CPUs, pool.Mems)

	// Check CPU booking in this pool. Print warning on overbooking.
	cpuAvail := pool.CPUs.Size() * 1000
	cpuRequested := 0
	containers := 0
	for _, cids := range pool.PodIDs {
		for _, cid := range cids {
			for _, container := range p.cch.GetContainers() {
				if container.GetCacheID() == cid {
					if reqCpu, ok := container.GetResourceRequirements().Requests[corev1.ResourceCPU]; ok {
						cpuRequested += int(reqCpu.MilliValue())
						containers += 1
					}
				}
			}
		}
	}
	if cpuRequested > cpuAvail {
		log.Warn("overbooked cpuset:%s: %dm CPUs requested by %d containers", pool.CPUs, cpuRequested, containers)
	}
}

// pinCpuMem pins container to CPUs and memory nodes if flagged
func (p *podpools) pinCpuMem(c cache.Container, cpus cpuset.CPUSet, mems sysfs.IDSet) {
	if p.ppoptions.PinCPU {
		log.Debug("  - pinning to cpuset: %s", cpus)
		c.SetCpusetCpus(cpus.String())
		if reqCpu, ok := c.GetResourceRequirements().Requests[corev1.ResourceCPU]; ok {
			mCpu := int(reqCpu.MilliValue())
			c.SetCPUShares(int64(cache.MilliCPUToShares(mCpu)))
		}
	}
	if p.ppoptions.PinMemory {
		log.Debug("  - pinning to memory %s", mems)
		c.SetCpusetMems(mems.String())
	}
}

func podpoolsError(format string, args ...interface{}) error {
	return fmt.Errorf(PolicyName+": "+format, args...)
}

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
	policy.Register(PolicyName, PolicyDescription, CreatePodpoolsPolicy)
}
