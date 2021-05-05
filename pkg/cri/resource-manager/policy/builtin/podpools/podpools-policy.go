// Copyright 2020-2021 Intel Corporation. All Rights Reserved.
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
	// podpoolKey is a pod annotation key, the value is a pod pool name.
	podpoolKey = "pool." + PolicyName + "." + kubernetes.ResmgrKeyNamespace
	// reservedPoolDefName is the name in the reserved pool definition.
	reservedPoolDefName = "reserved"
	// defaultPoolDefName is the name in the default pool definition.
	defaultPoolDefName = "default"
	// podMilliCPUErrorMargin is the maximum error in requested vs
	// allocated mCPUs per pod. For instance, 10 mCPU error margin
	// allows error of magnitude of +-0.5 mCPU/container up to 20
	// containers/pod.
	podMilliCPUErrorMargin = int64(10)
)

// podpools contains configuration and runtime attributes of the podpools policy
type podpools struct {
	options         *policyapi.BackendOptions // configuration common to all policies
	ppoptions       PodpoolsOptions           // podpools-specific configuration
	cch             cache.Cache               // cri-resmgr cache
	allowed         cpuset.CPUSet             // bounding set of CPUs we're allowed to use
	reserved        cpuset.CPUSet             // system-/kube-reserved CPUs
	reservedPoolDef *PoolDef                  // built-in definition of the reserved pool
	defaultPoolDef  *PoolDef                  // built-in definition of the default pool
	pools           []*Pool                   // pools for pods: reserved, default and user-defined
	podMaxMilliCPU  map[string]int64          // maximum total MilliCPUs requested by containers of pods in pools
	cpuAllocator    cpuallocator.CPUAllocator // CPU allocator used by the policy
}

// Pool contains attributes of a pool instance
type Pool struct {
	// Def is the definition from which this pool instance is created.
	Def *PoolDef
	// Instance is the index of this pool instance, starting from
	// zero for every pool definition.
	Instance int
	// CPUs is the set of CPUs exclusive to this pool instance only.
	CPUs cpuset.CPUSet
	// Mems is the set of memory nodes with minimal access delay
	// from CPUs.
	Mems sysfs.IDSet
	// PodIDs maps pod ID to list of container IDs.
	// - len(PodIDs) is the number of pods in the pool.
	// - len(PodIDs[podID]) is the number of containers of podID
	//   currently assigned to the pool.
	// - Def.MaxPods - len(PodIDs) is free pod capacity.
	PodIDs map[string][]string
}

var log logger.Logger = logger.NewLogger("policy")

// String is a stringer for a pool.
func (pool Pool) String() string {
	podCount := len(pool.PodIDs)
	contCount := 0
	for _, contIDs := range pool.PodIDs {
		contCount += len(contIDs)
	}
	s := fmt.Sprintf("%s{cpus:%s, mems:%s, pods:%d/%d, containers:%d}",
		pool.PrettyName(), pool.CPUs, pool.Mems, podCount, pool.Def.MaxPods, contCount)
	return s
}

// PrettyName returns unique name for a pool.
func (pool Pool) PrettyName() string {
	return fmt.Sprintf("%s[%d]", pool.Def.Name, pool.Instance)
}

// CreatePodpoolsPolicy creates a new policy instance.
func CreatePodpoolsPolicy(policyOptions *policy.BackendOptions) policy.Backend {
	p := &podpools{
		options: policyOptions,
		cch:     policyOptions.Cache,
		reservedPoolDef: &PoolDef{
			Name:    reservedPoolDefName,
			MaxPods: 0,
		},
		defaultPoolDef: &PoolDef{
			Name:    defaultPoolDefName,
			MaxPods: 0,
		},
		podMaxMilliCPU: make(map[string]int64),
		cpuAllocator:   cpuallocator.NewCPUAllocator(policyOptions.System),
	}
	log.Infof("creating %s policy...", PolicyName)
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
				log.Fatalf("failed to allocate reserved CPUs: %s", err)
			}
			p.reserved = cpus
			p.allowed = p.allowed.Union(cpus)
		}
	}
	if p.reserved.IsEmpty() {
		log.Fatalf("%s cannot run without reserved CPUs that are also AvailableResources", PolicyName)
	}
	// Handle policy-specific options
	log.Debugf("creating %s configuration", PolicyName)
	if err := p.setConfig(podpoolsOptions); err != nil {
		log.Fatalf("failed to create %s policy: %v", PolicyName, err)
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
	log.Infof("%s policy started", PolicyName)
	return p.Sync(add, del)
}

// Sync synchronizes the active policy state.
func (p *podpools) Sync(add []cache.Container, del []cache.Container) error {
	log.Debugf("synchronizing state...")
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
	log.Debugf("allocating container %s...", c.PrettyName())
	// Assign container to correct pool.
	pod, ok := c.GetPod()
	if !ok {
		return podpoolsError("cannot find pod of container %s from the cache", c.PrettyName())
	}
	if pool := p.allocatePool(pod); pool != nil {
		p.assignContainer(c, pool)
		p.trackPodCPU(pod, pool)
		if log.DebugEnabled() {
			log.Debugf(p.dumpPool(pool))
		}
	} else {
		// Cannot assign container to any of the pooled CPUs.
		return podpoolsError("cannot find CPUs to run container %s - no default or reserved CPUs available", c.PrettyName())
	}
	return nil
}

// ReleaseResources is a resource release request for this policy.
func (p *podpools) ReleaseResources(c cache.Container) error {
	log.Debugf("releasing container %s...", c.PrettyName())
	pod, ok := c.GetPod()
	if !ok {
		return podpoolsError("cannot find pod of container %s from the cache", c.PrettyName())
	}
	if pool := p.allocatedPool(pod); pool != nil {
		p.dismissContainer(c, pool)
		if log.DebugEnabled() {
			log.Debugf(p.dumpPool(pool))
		}
		if p.containersInPool(pod, pool) == 0 {
			log.Debugf("all containers removed, free pool allocation %s for pod %q", pool.PrettyName(), pod.GetName())
			p.validatePodCPU(pod, pool)
			p.freePool(pod, pool)
		}
	} else {
		log.Debugf("ReleaseResources: pool-less container %s, nothing to release", c.PrettyName())
	}
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (p *podpools) UpdateResources(c cache.Container) error {
	log.Debugf("(not) updating container %s...", c.PrettyName())
	return nil
}

// Rebalance tries to find an optimal allocation of resources for the current containers.
func (p *podpools) Rebalance() (bool, error) {
	log.Debugf("(not) rebalancing containers...")
	return false, nil
}

// HandleEvent handles policy-specific events.
func (p *podpools) HandleEvent(*events.Policy) (bool, error) {
	log.Debugf("(not) handling event...")
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

// allocatedPool returns a pool already allocated for a pod.
func (p *podpools) allocatedPool(pod cache.Pod) *Pool {
	podID := pod.GetID()
	pools := filterPools(p.pools,
		func(pl *Pool) bool { _, ok := pl.PodIDs[podID]; return ok })
	if len(pools) == 0 {
		return nil
	}
	return pools[0]
}

// allocatePool returns a pool allocated for a pod.
func (p *podpools) allocatePool(pod cache.Pod) *Pool {
	if pool := p.allocatedPool(pod); pool != nil {
		return pool
	}
	poolDef := p.getPoolDef(pod)
	if poolDef == nil {
		return nil
	}
	// Try to find a suitable pool and allocate it for the pod.
	pools := filterPools(p.pools,
		func(pl *Pool) bool {
			return poolDef.Name == pl.Def.Name && (pl.Def.MaxPods > len(pl.PodIDs) || pl.Def.MaxPods == 0)
		})
	// Sort pools according to pool type fill order so that the
	// first pool in the list is the preferred one.
	switch poolDef.FillOrder {
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
		log.Errorf("cannot find free %q pool for pod %q, falling back to %q", poolDef.Name, pod.GetName(), defaultPoolDefName)
		pools = []*Pool{p.pools[1]}
	}
	// Found a suitable pool. Allocate it for the pod.
	podID := pod.GetID()
	pool := pools[0]
	pool.PodIDs[podID] = []string{}
	log.Debugf("allocated pool %s[%d] for pod %q", pool.Def.Name, pool.Instance, pod.GetName())
	return pool
}

// containersInPool returns the number of containers of a pod in a pool.
func (p *podpools) containersInPool(pod cache.Pod, pool *Pool) int {
	if cnts, ok := pool.PodIDs[pod.GetID()]; ok {
		return len(cnts)
	}
	return 0
}

// dumpPool dumps pool contents in detail.
func (p *podpools) dumpPool(pool *Pool) string {
	conts := []string{}
	pods := []string{}
	for podID, contIDs := range pool.PodIDs {
		podName := podID
		if pod, ok := p.cch.LookupPod(podID); ok {
			podName = pod.GetName()
		}
		pods = append(pods, fmt.Sprintf("%s (mCPU: %d, max=%d)", podName, p.getPodMilliCPU(podID), p.podMaxMilliCPU[podID]))
		for _, contID := range contIDs {
			if cont, ok := p.cch.LookupContainer(contID); ok {
				conts = append(conts, cont.PrettyName())
			} else {
				conts = append(conts, podName+":"+contID)
			}
		}
	}
	s := fmt.Sprintf("Pool{Def.Name: %q, Instance: %d, CPUs: %s, Mems: %s, Def.MaxPods: %d, pods: %v, containers:%v}",
		pool.Def.Name, pool.Instance, pool.CPUs, pool.Mems, pool.Def.MaxPods, pods, conts)
	return s
}

// freePool removes an empty pod from a pool
func (p *podpools) freePool(pod cache.Pod, pool *Pool) {
	podID := pod.GetID()
	delete(pool.PodIDs, podID)
	delete(p.podMaxMilliCPU, podID)
}

// trackPodCPU keeps track on pod's CPU requests.
func (p *podpools) trackPodCPU(pod cache.Pod, pool *Pool) {
	// As we do not have direct information on total CPU resources
	// requested by a pod, we gather the information indirectly by
	// tracking the sum of requested CPUs of its running
	// containers. This enables reacting to misalignment between
	// CPU resources per pod in a pool and CPU resource requests
	// visible to the kube-scheduler.
	podID := pod.GetID()
	current := p.getPodMilliCPU(podID)
	if max, ok := p.podMaxMilliCPU[podID]; ok {
		if max < current {
			p.podMaxMilliCPU[podID] = current
		}
	} else {
		p.podMaxMilliCPU[podID] = current
	}
	// Check overbooking
	if cpuAvail := p.availableMilliCPUs(pool); cpuAvail < 0 {
		log.Errorf("overbooked pool %q, cpuset:%s: %dm / %dm CPUs used, %d mCPU available", pool.PrettyName(), pool.CPUs, pool.CPUs.Size()*1000-int(cpuAvail), pool.CPUs.Size()*1000, cpuAvail)
	}
}

// validatePodCPU compares max CPU requests against pool CPU capacity per pod.
func (p *podpools) validatePodCPU(pod cache.Pod, pool *Pool) {
	// Log pod configuration error if a pool has fixed amount of
	// CPUs per pod but the pod failed to request the correct
	// amount.
	podID := pod.GetID()
	if podmCPU, ok := p.podMaxMilliCPU[podID]; ok {
		if pool.Def.MaxPods > 0 {
			poolmCPUperPod := int64(pool.CPUs.Size() * 1000 / pool.Def.MaxPods)
			mCPUerr := podmCPU - poolmCPUperPod
			// Allow rounding errors (up and down) when
			// comparing the sum of containers' CPU usages
			// against milli-CPUs allocated per pod in its
			// pool.
			if mCPUerr < -podMilliCPUErrorMargin || mCPUerr > podMilliCPUErrorMargin {
				podName := ""
				if pod, ok := p.cch.LookupPod(podID); ok {
					podName = pod.GetName()
				}
				log.Errorf("bad CPU requests: pod %q requested %d mCPUs, but in pool %q pods must request %d mCPUs.", podName, podmCPU, pool.Def.Name, poolmCPUperPod)
			}
		}
	}
}

// getPodMilliCPU returns mCPUs requested by podID.
func (p *podpools) getPodMilliCPU(podID string) int64 {
	cpuRequested := int64(0)
	for _, c := range p.cch.GetContainers() {
		if c.GetPodID() == podID {
			if reqCpu, ok := c.GetResourceRequirements().Requests[corev1.ResourceCPU]; ok {
				cpuRequested += reqCpu.MilliValue()
			}
		}
	}
	return cpuRequested
}

// configNotify applies new configuration.
func (p *podpools) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	log.Infof("configuration %s", event)
	if err := p.setConfig(podpoolsOptions); err != nil {
		log.Errorf("config update failed: %v", err)
		return err
	}
	log.Infof("config updated successfully")
	p.Sync(p.cch.GetContainers(), nil)
	return nil
}

// getPoolDefName returns the name of the pool definition of a pod.
func (p *podpools) getPoolDefName(pod cache.Pod) string {
	if poolDefName, ok := pod.GetEffectiveAnnotation(podpoolKey, ""); ok {
		return poolDefName
	}
	if pod.GetNamespace() == "kube-system" {
		return reservedPoolDefName
	}
	return defaultPoolDefName
}

// getPoolDef returns the pool definition of a pod.
func (p *podpools) getPoolDef(pod cache.Pod) *PoolDef {
	poolDefName := p.getPoolDefName(pod)
	if poolDefName == reservedPoolDefName {
		return p.reservedPoolDef
	}
	if poolDefName == defaultPoolDefName {
		return p.defaultPoolDef
	}
	for _, poolDef := range p.ppoptions.PoolDefs {
		if poolDef.Name == poolDefName {
			return poolDef
		}
	}
	log.Errorf("pod %q pool %q does not match any pool definition, falling back to %q", pod.GetName(), poolDefName, p.defaultPoolDef.Name)
	return p.defaultPoolDef
}

// applyPoolDef creates user-defined pools or reconfigures built-in
// pools according to the poolDef.
func (p *podpools) applyPoolDef(pools *[]*Pool, poolDef *PoolDef, freeCpus *cpuset.CPUSet, nonReservedCpuCount int) error {
	if len(*pools) < 2 {
		return podpoolsError("internal error: reserved and default pools missing, cannot apply pool definitions")
	}
	reservedPool := (*pools)[0]
	defaultPool := (*pools)[1]
	// Every PoolDef does one of the following:
	// 1. reconfigures the "reserved" pool (most restricted)
	// 2. reconfigutes the "default" pool (somewhat restricted)
	// 3. defines new user-defined pools.
	switch poolDef.Name {
	case "":
		// Case 0: bad name
		return podpoolsError("undefined or empty pool name")

	case reservedPool.Def.Name:
		// Case 1: reconfigure the "reserved" pool.
		// Forbid redefinition of CPU and Instances.
		if poolDef.CPU != "" || poolDef.Instances != "" {
			poolCount, cpusPerPool, err := parseInstancesCPUs(poolDef.Instances, poolDef.CPU, nonReservedCpuCount)
			if err != nil {
				return podpoolsError("pool %q: %w", poolDef.Name, err)
			}
			if poolCount != 1 {
				return podpoolsError("pool %q: cannot change the number of instances", poolDef.Name)
			}
			if cpusPerPool != reservedPool.CPUs.Size() {
				return podpoolsError("pool %q: number of CPUs is conflicting ReservedResources CPUs", poolDef.Name)
			}
		}
		reservedPool.Def.MaxPods = poolDef.MaxPods

	case defaultPool.Def.Name:
		// Case 2: reconfigure the "default" pool.
		// Allow redefinition of CPU but not Instances.
		if poolDef.CPU != "" || poolDef.Instances != "" {
			poolCount, cpusPerPool, err := parseInstancesCPUs(poolDef.Instances, poolDef.CPU, nonReservedCpuCount)
			if err != nil {
				return podpoolsError("pool %q: %w", poolDef.Name, err)
			}
			if poolCount != 1 {
				return podpoolsError("pool %q: cannot change the number of instances", poolDef.Name)
			}
			cpus, err := p.cpuAllocator.AllocateCpus(freeCpus, cpusPerPool, cpuallocator.PriorityNormal)
			if err != nil {
				return podpoolsError("could not allocate %d CPUs for pool %q: %w", cpusPerPool, poolDef.Name, err)
			}
			defaultPool.CPUs = cpus
		}
		defaultPool.Def.MaxPods = poolDef.MaxPods

	default:
		// Case 3: create new user-defined pool(s).
		poolCount, cpusPerPool, err := parseInstancesCPUs(poolDef.Instances, poolDef.CPU, nonReservedCpuCount)
		if err != nil {
			return podpoolsError("pool %q: %w", poolDef.Name, err)
		}
		if poolCount == 0 {
			return podpoolsError("pool %q: insufficient CPUs to create any instances", poolDef.Name)
		}
		if poolCount > 1 && poolDef.FillOrder == FillPacked && poolDef.MaxPods == 0 {
			return podpoolsError("pool %q: %d pool(s) unreachable due to unlimited pod capacity and FillOrder: %s", poolDef.Name, poolCount-1, poolDef.FillOrder)
		}
		log.Debugf("allocating %d out of %d non-reserved CPUs for %d %q pools", poolCount*cpusPerPool, nonReservedCpuCount, poolCount, poolDef.Name)
		for poolIndex := 0; poolIndex < poolCount; poolIndex++ {
			if cpusPerPool > freeCpus.Size() {
				return podpoolsError("insufficient CPUs when trying to allocate %d CPUs for pool %s[%d]", cpusPerPool, poolDef.Name, poolIndex)
			}
			cpus, err := p.cpuAllocator.AllocateCpus(freeCpus, cpusPerPool, cpuallocator.PriorityNormal)
			if err != nil {
				return podpoolsError("could not allocate %d CPUs for instance %d of pool %q: %w", cpusPerPool, poolIndex, poolDef.Name, err)
			}
			pool := Pool{
				Def:      poolDef,
				Instance: poolIndex,
				CPUs:     cpus,
			}
			*pools = append(*pools, &pool)
		}
	}
	return nil
}

// setConfig takes new pool configuration into use.
func (p *podpools) setConfig(ppoptions *PodpoolsOptions) error {
	// Instantiate pools for pods.
	pools := []*Pool{}
	// Built-in reserved pool.
	reservedPool := Pool{
		Def:  p.reservedPoolDef,
		CPUs: p.reserved,
	}
	pools = append(pools, &reservedPool)
	// Built-in default pool.
	// The default pool will use reserved CPUs by default. If CPUs
	// are left over after constructing user-defined pools, those
	// will be used as the Default pool instead.
	defaultPool := Pool{
		Def:  p.defaultPoolDef,
		CPUs: reservedPool.CPUs,
	}
	pools = append(pools, &defaultPool)
	// Apply pool definitions from configuration.
	freeCpus := p.allowed.Clone()
	freeCpus = freeCpus.Difference(p.reserved)
	nonReservedCpuCount := freeCpus.Size()
	userPoolDefs := 0
	for _, poolDef := range ppoptions.PoolDefs {
		if err := p.applyPoolDef(&pools, poolDef, &freeCpus, nonReservedCpuCount); err != nil {
			return err
		}
		if poolDef.Name != reservedPoolDefName && poolDef.Name != defaultPoolDefName {
			userPoolDefs += 1
		}
	}
	// Check if there are unallocated CPUs.
	if freeCpus.Size() > 0 {
		if defaultPool.CPUs.Intersection(reservedPool.CPUs).IsEmpty() {
			// User has reallocated "default" pool CPUs
			log.Debugf("%d unused CPUs are added to the default pool.", freeCpus.Size())
			defaultPool.CPUs = defaultPool.CPUs.Union(freeCpus)
		} else {
			log.Debugf("%d unused CPUs are used as the default pool.", freeCpus.Size())
			defaultPool.CPUs = freeCpus
		}
	}
	// Finish pool instance initialization.
	log.Infof("%s policy pools:", PolicyName)
	for index, pool := range pools {
		pool.Mems = p.closestMems(pool.CPUs)
		pool.PodIDs = make(map[string][]string)
		log.Infof("- pool %d: %s", index, pool)
	}
	// No errors in pool creation, take new configuration into use.
	log.Debugf("new %s configuration:\n%s", PolicyName, utils.DumpJSON(ppoptions))
	p.pools = pools
	p.ppoptions = *ppoptions
	// Warning on multiple user-defined pools.
	if userPoolDefs > 1 {
		log.Warnf("Multiple (%d) user-defined pool definitions on the node. kube-scheduler does not know which of the pools has CPUs left for new workloads, and may overbook pools on the node.", userPoolDefs)
	}
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

// parseInstancesCPUs parses the number of pool instances and the
// number of CPUs per pool instance from PoolDef Instances and CPUs
// fields.
func parseInstancesCPUs(is string, cs string, freeCpus int) (int, int, error) {
	if cs == "" {
		return 0, 0, podpoolsError("missing CPUs")
	}
	c64, err := strconv.ParseInt(cs, 0, 32)
	if err != nil || c64 <= 0 {
		return 0, 0, podpoolsError("invalid CPUs per pool: %q, integer > 1 expected", cs)
	}
	cpusPerPool := int(c64)
	// Supported Instances specifications:
	// 0. Instances is an empty string.
	//    Create 1 instance.
	// 1. Instances: N %
	//    Use at most N % of freeCpus for all PoolDef instances.
	//    The number of instances is floor(freeCpus * N/100 / cpusPerPool).
	// 2. Instances: N CPUs
	//    Use at most N CPUs for all PoolDef instances.
	//    The number of instances is floor(N / cpusPerPool).
	// 3. Instances: N
	//    Create N instances from PoolDef.
	var instances int
	switch {
	case is == "":
		instances = 1
	case strings.HasSuffix(is, "%"):
		tis := strings.TrimSpace(strings.TrimSuffix(is, "%"))
		i64, err := strconv.ParseInt(tis, 0, 32)
		if err != nil || i64 < 0 {
			return 0, 0, podpoolsError("invalid Instances: %q", is)
		}
		instances = freeCpus * int(i64) / 100 / cpusPerPool
	case strings.HasSuffix(strings.ToLower(is), "cpu"):
		// All these are equivalent: N(cpu|cpus|CPU|CPUs|CPUS) for any N > 0.
		// Handling "CPU" suffix is an alias for "CPUs".
		is = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(is), "cpu")) + "cpus"
		fallthrough
	case strings.HasSuffix(strings.ToLower(is), "cpus"):
		tis := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(is), "cpus"))
		i64, err := strconv.ParseInt(tis, 0, 32)
		if err != nil || i64 < 0 {
			return 0, 0, podpoolsError("invalid Instances: %q", is)
		}
		if i64 > int64(freeCpus) {
			return 0, 0, podpoolsError("insufficient CPUs: %d required for instances but %d is available", i64, freeCpus)
		}
		instances = int(i64) / cpusPerPool
	default:
		i64, err := strconv.ParseInt(is, 0, 32)
		if err != nil || i64 < 0 {
			return 0, 0, podpoolsError("invalid Instances: %q", is)
		}
		instances = int(i64)
	}
	return instances, cpusPerPool, nil
}

// availableMilliCPU returns mCPUs available in a pool.
func (p *podpools) availableMilliCPUs(pool *Pool) int64 {
	cpuAvail := int64(pool.CPUs.Size() * 1000)
	cpuRequested := int64(0)
	for podID := range pool.PodIDs {
		cpuRequested += p.getPodMilliCPU(podID)
	}
	return cpuAvail - cpuRequested
}

// assignContainer adds a container to a pool
func (p *podpools) assignContainer(c cache.Container, pool *Pool) {
	log.Infof("assigning container %s to pool %s", c.PrettyName(), pool)
	podID := c.GetPodID()
	pool.PodIDs[podID] = append(pool.PodIDs[podID], c.GetCacheID())
	p.pinCpuMem(c, pool.CPUs, pool.Mems)
}

// dismissContainer removes a container from a pool
func (p *podpools) dismissContainer(c cache.Container, pool *Pool) {
	podID := c.GetPodID()
	pool.PodIDs[podID] = removeString(pool.PodIDs[podID], c.GetCacheID())
}

// pinCpuMem pins container to CPUs and memory nodes if flagged
func (p *podpools) pinCpuMem(c cache.Container, cpus cpuset.CPUSet, mems sysfs.IDSet) {
	if p.ppoptions.PinCPU {
		log.Debugf("  - pinning to cpuset: %s", cpus)
		c.SetCpusetCpus(cpus.String())
		if reqCpu, ok := c.GetResourceRequirements().Requests[corev1.ResourceCPU]; ok {
			mCpu := int(reqCpu.MilliValue())
			c.SetCPUShares(int64(cache.MilliCPUToShares(mCpu)))
		}
	}
	if p.ppoptions.PinMemory {
		log.Debugf("  - pinning to memory %s", mems)
		c.SetCpusetMems(mems.String())
	}
}

// podpoolsError formats an error from this policy.
func podpoolsError(format string, args ...interface{}) error {
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
	policy.Register(PolicyName, PolicyDescription, CreatePodpoolsPolicy)
}
