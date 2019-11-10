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

package staticplus

import (
	"encoding/json"
	"fmt"
	"strconv"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	logger "github.com/intel/cri-resource-manager/pkg/log"

	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
)

const (
	// PolicyName is the symbol used to pull us in as a builtin policy.
	PolicyName = "static-plus"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "A simple policy supporting exclusive/pinned and shared allocations."
	// Cache key for storing container resource allocations.
	keyAllocations = "allocations"
	// Cache key for storing the shared pool.
	keySharedPool = "shared-pool"
	// keyPreferIsolated is the annotation used to mark pods preferring isolated CPUs.
	keyPreferIsolated = "prefer-isolated-cpus"
)

// Assignment tracks resource assignments for a single container.
type Assignment struct {
	exclusive cpuset.CPUSet // exclusively allocated cpus
	shared    int           // milli-cpus to allocated from shared cpus
}

// Allocations track all resources allocations by the static+ policy.
type Allocations map[string]*Assignment

// static-plus policy runtime state.
type staticplus struct {
	logger.Logger
	offline     cpuset.CPUSet // offlined cpus
	available   cpuset.CPUSet // bounding set of cpus available for us
	reserved    cpuset.CPUSet // pool (primarily) for system-/kube-tasks
	isolated    cpuset.CPUSet // primary pool for exclusive allocations
	allocations Allocations   // container cpu allocations
	sys         *sysfs.System // system/topologu information
	cache       cache.Cache   // system state/cache
	shared      cpuset.CPUSet // pool for fractional and shared allocations
}

// Make sure staticplus implements the policy backend interface.
var _ policy.Backend = &staticplus{}

// CreateStaticPlusPolicy creates a new policy instance.
func CreateStaticPlusPolicy(opts *policy.BackendOptions) policy.Backend {
	p := &staticplus{
		Logger: logger.NewLogger(PolicyName),
	}

	p.Info("creating policy...")

	if err := p.discoverSystemTopology(); err != nil {
		p.Fatal("failed to discover system/topology: %v", err)
	}

	if err := p.setupPools(opts.Available, opts.Reserved); err != nil {
		p.Fatal("failed to set up cpu pools: %v", err)
	}

	p.dumpPools()

	return p
}

// Name returns the name of this policy.
func (p *staticplus) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (p *staticplus) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (p *staticplus) Start(cch cache.Cache, add []cache.Container, del []cache.Container) error {
	p.cache = cch

	if err := p.restoreCache(); err != nil {
		return policyError("failed to start: %v", err)
	}

	if err := p.updatePools(); err != nil {
		return policyError("failed to start: %v", err)
	}

	return p.Sync(add, del)
}

// Sync synchronizes the state ofd this policy.
func (p *staticplus) Sync(add []cache.Container, del []cache.Container) error {
	p.Debug("synchronizing state...")
	for _, c := range del {
		p.ReleaseResources(c)
	}
	for _, c := range add {
		p.AllocateResources(c)
	}

	return nil
}

// AllocateResources allocates resources for the given container.
func (p *staticplus) AllocateResources(c cache.Container) error {
	var a *Assignment

	id := c.GetCacheID()

	p.Debug("allocating container %s...", id)

	if _, ok := p.allocations[id]; ok {
		return nil
	}

	a, err := p.assignCpus(c)
	if err != nil {
		return err
	}

	return p.addAssignment(c, a)
}

// ReleaseResources release resources assigned to the given container.
func (p *staticplus) ReleaseResources(c cache.Container) error {
	id := c.GetCacheID()

	p.Debug("releasing container %s...", id)

	a, ok := p.allocations[id]
	if !ok {
		return nil
	}

	return p.delAssignment(a, id)
}

// UpdateResources is a resource allocation update request for this policy.
func (p *staticplus) UpdateResources(c cache.Container) error {
	p.Debug("(not) updating container %s...", c.PrettyName())
	return nil
}

// ExportResourceData provides resource data to export for the container.
func (p *staticplus) ExportResourceData(c cache.Container, syntax policy.DataSyntax) []byte {
	id := c.GetCacheID()
	a, ok := p.allocations[id]
	if !ok {
		// Hmm...
		p.Warn("can't find allocation for container %s", id)
	}

	data := ""
	if a.shared != 0 {
		data = "SHARED_CPUS=\"" + p.shared.String() + "\"\n"
	}
	if a != nil && !a.exclusive.IsEmpty() {
		isolated := a.exclusive.Intersection(p.sys.Isolated())
		if isolated.String() != "" {
			data += "ISOLATED_CPUS=\"" + isolated.String() + "\"\n"
		}
		exclusive := a.exclusive.Difference(p.sys.Isolated())
		if exclusive.String() != "" {
			data += "EXCLUSIVE_CPUS=\"" + exclusive.String() + "\"\n"
		}
	}

	return []byte(data)
}

func (p *staticplus) PostStart(cch cache.Container) error {
	/*
		if p.rdt != nil {
			pod, ok := cch.GetPod()
			if !ok {
				return policyError("Pod of container %q not found", cch.GetID())
			}
			qos := string(pod.GetQOSClass())

			p.Info("setting RDT class of container %q to %q", cch.GetID(), qos)

			return p.rdt.SetContainerClass(cch, qos)
		}
	*/
	return nil
}

// SetConfig sets the policy backend configuration
func (p *staticplus) SetConfig(string) error {
	return nil
}

// policyError creates a formatted policy-specific error.
func policyError(format string, args ...interface{}) error {
	return fmt.Errorf(PolicyName+": "+format, args...)
}

// discoverSystemTopology discovers system hardware/topology.
func (p *staticplus) discoverSystemTopology() error {
	p.Info("discovering system topology...")

	sys, err := sysfs.DiscoverSystem()
	if err != nil {
		return policyError("failed to discover system/topology: %v", err)
	}

	p.sys = sys
	return nil
}

// setupPools sets up the pools we allocate resources from.
func (p *staticplus) setupPools(available, reserved policy.ConstraintSet) error {
	// Set up three disjoint CPU pools for allocating CPU to containers. These
	// three pools are:
	//
	//   1) reserved pool: kube- and system-tasks
	//        Pods in the kube-system namespace are assigned to this pool. The
	//        size of this pool is the requested reservation rounded up to the
	//        closest integer. Any unused fractional part of this pool is used
	//        as a shared pool if the shared pool ever gets fully allocated.
	//
	//   2) isolated pool: primary exclusive allocations
	//        Exclusive CPU allocations are primarily done from this pool. Pods
	//        that request at least 1 full CPU get their exclusive (integer)
	//        CPU shares allocated from this pool unless the pool has already
	//        been exhausted (in which case we try to slice off exclusive CPUs
	//        from the shared pool).
	//
	//   3) shared pool: shared allocations, secondary exclusive allocations
	//        Shared CPU allocations are served from this pool. Pods fractional
	//        CPU shares are allocated from this pool. If the isolated pool has
	//        been exhausted exclusive allocations are sliced off from this
	//        pool. If this pool has been fully allocated, shared allocations
	//        are oversubscribed to the reserved pool.

	p.offline = p.sys.Offlined()

	cpus, ok := available[policy.DomainCPU]
	if !ok {
		p.available = p.sys.CPUSet().Difference(p.offline)
	} else {
		p.available = cpus.(cpuset.CPUSet).Difference(p.offline)
	}

	p.isolated = p.sys.Isolated().Intersection(p.available)
	p.available = p.available.Difference(p.isolated)

	cpus, ok = reserved[policy.DomainCPU]
	if !ok {
		return policyError("cannot start without any reserved CPUs")
	}

	switch cpus.(type) {
	case cpuset.CPUSet:
		p.reserved = cpus.(cpuset.CPUSet).Intersection(p.available)
		if !p.reserved.Equals(cpus.(cpuset.CPUSet)) {
			return policyError("part of the reserved CPUs (%s) are not available: %s",
				cpus.(cpuset.CPUSet).String(), cpus.(cpuset.CPUSet).Difference(p.available))
		}
		p.available = p.available.Difference(p.reserved)

	case resource.Quantity:
		var err error
		qty := cpus.(resource.Quantity)
		count := (int(qty.MilliValue()) + 999) / 1000
		if count < 2 && p.available.Contains(0) {
			p.reserved = cpuset.NewCPUSet(0)
			p.available = p.available.Difference(p.reserved)
		} else {
			p.reserved, err = takeCPUs(&p.available, nil, count)
			if err != nil {
				return policyError("failed to reserve %d CPUs from %s: %v",
					count, p.available.String())
			}
		}
	}

	p.shared = p.available

	return nil
}

// Restore saved policy state from the cache.
func (p *staticplus) restoreCache() error {
	if !p.cache.GetPolicyEntry(keySharedPool, &p.shared) {
		p.Warn("initializing empty policy state...")

		p.shared = p.available
		p.allocations = make(Allocations)
		p.cache.SetPolicyEntry(keySharedPool, &p.shared)
		p.cache.SetPolicyEntry(keyAllocations,
			cache.Cachable(&cachedAllocations{a: p.allocations}))
	} else {
		p.Info("restoring cached policy state...")

		ca := cachedAllocations{}
		if !p.cache.GetPolicyEntry(keyAllocations, &ca) {
			return policyError("failed to restore state from cache, no allocations")
		}
		p.allocations = ca.a
	}

	p.dumpPools()
	p.dumpAllocations()

	return nil
}

// requestedCpus calculates the exclusive and shared cpu allocations for a container.
func (p *staticplus) requestedCpus(c cache.Container) (int, int) {
	cpuReq, ok := c.GetResourceRequirements().Requests[v1.ResourceCPU]
	if !ok {
		return 0, 0
	}

	full := int(cpuReq.MilliValue()) / 1000
	part := int(cpuReq.MilliValue()) - 1000*full

	return full, part
}

// optOutFromIsolated checks if a container prefers (to opt out from) isolated CPUs.
func (p *staticplus) optOutFromIsolation(c cache.Container) bool {
	preferIsolated := true

	if pod, found := c.GetPod(); !found {
		p.Warn("can't find pod for container %s", c.PrettyName())
	} else {
		if value, ok := pod.GetResmgrAnnotation(keyPreferIsolated); ok {
			if isolated, err := strconv.ParseBool(value); !isolated {
				if err != nil {
					p.Error("invalid annotation '%s' on container %s, expecting boolean: %v",
						keyPreferIsolated, c.PrettyName(), err)
				} else {
					p.Info("container %s is opted-out from isolation", c.PrettyName())
				}
				preferIsolated = false
			} else {
				p.Info("container %s explicitly opted-in for isolation", c.PrettyName())
			}
		} else {
			p.Info("container %s goes with default isolation", c.PrettyName())
		}
	}

	return !preferIsolated
}

// assignCpus allocates cpus for a containers.
func (p *staticplus) assignCpus(c cache.Container) (*Assignment, error) {
	full, part := p.requestedCpus(c)

	// system containers always share (the reserved) cpus
	if c.GetNamespace() == metav1.NamespaceSystem {
		return &Assignment{shared: 1000*full + part}, nil
	}

	// assign to the shared pool if less than a single cpu was requested
	if full == 0 {
		return &Assignment{shared: part}, nil
	}

	// if there is capacity in the isolated pool, slice cpus off from it
	if p.isolated.Size() >= full && !p.optOutFromIsolation(c) {
		cpus, err := takeCPUs(&p.isolated, nil, full)
		if err != nil {
			return nil, policyError("failed to allocate %d isolated CPUs: %v",
				full, err)
		}
		return &Assignment{exclusive: cpus, shared: part}, nil
	}

	// otherwise, try to slice off cpus from the shared pool
	if p.shared.Size() >= full {
		cpus, err := takeCPUs(&p.shared, nil, full)
		if err != nil {
			return nil, policyError("failed to allocate %d exclusive CPUs: %v",
				full, err)
		}
		return &Assignment{exclusive: cpus, shared: part}, nil
	}

	// we're screwed, not enough cpu in either isolated or shared pool
	return nil, policyError("failed to allocate %d exclusive CPUs: %s",
		full, "not enough capacity")
}

// addAssignment updates container allocations for a newly added container assignment.
func (p *staticplus) addAssignment(c cache.Container, a *Assignment) error {
	switch {
	// always assign system containers to the reserved pool
	case c.GetNamespace() == metav1.NamespaceSystem:
		c.SetCpusetCpus(p.reserved.String())
		c.SetCPUShares(int64(MilliCPUToShares(a.shared)))

		p.Info("system container %s allocated (%d mCPU) to reserved pool %s",
			c.PrettyName(), a.shared, p.reserved.String())

		// for shared-only assignments, it's enough to update the container
	case a.exclusive.IsEmpty():
		c.SetCpusetCpus(p.shared.String())
		c.SetCPUShares(int64(MilliCPUToShares(a.shared)))

		p.Info("container %s allocated (%d mCPU) to shared pool %s",
			c.PrettyName(), a.shared, p.shared.String())

		// isolated, sliced-off exclusive, or mixed allocation
	default:
		var kind string
		var isolated bool
		if isolated = !a.exclusive.Intersection(p.sys.Isolated()).IsEmpty(); isolated {
			kind = "isolated"
		} else {
			kind = "exclusive"
		}
		if a.shared != 0 {
			c.SetCpusetCpus(a.exclusive.Union(p.shared).String())
			c.SetCPUShares(int64(MilliCPUToShares(a.shared)))
			p.Info("container %s allocated to %s (%s) and shared (%d mCPU) pool %s",
				c.PrettyName(), kind, a.exclusive.String(), a.shared, p.shared.String())
		} else {
			c.SetCpusetCpus(a.exclusive.String())
			c.SetCPUShares(int64(MilliCPUToShares(1000 * a.exclusive.Size())))
			p.Info("container %s allocated to %s CPUs %s", c.PrettyName(),
				kind, a.exclusive.String())
		}

		// for sliced-off exclusive we might need to update other containers shared allocations
		if !a.exclusive.IsEmpty() && a.exclusive.Intersection(p.sys.Isolated()).IsEmpty() {
			if err := p.updateSharedAllocations(); err != nil {
				return err
			}
		}
	}

	p.allocations[c.GetCacheID()] = a

	p.cache.SetPolicyEntry(keySharedPool, p.shared)
	p.cache.SetPolicyEntry(keyAllocations,
		cache.Cachable(&cachedAllocations{a: p.allocations}))

	return nil
}

// delAssignment updates container allocations for a deleted container assignment.
func (p *staticplus) delAssignment(a *Assignment, id string) error {
	delete(p.allocations, id)

	switch {
	// for shared-only allocations there is not much to do...
	case a.exclusive.IsEmpty():
		p.Info("freed shared-only (%d mCPU) allocations of container %s",
			a.shared, id)

		// for isolated exclusive cpus, return them to the pool
	case !a.exclusive.Intersection(p.sys.Isolated()).IsEmpty():
		p.isolated = p.isolated.Union(a.exclusive)

		p.Info("freed isolated allocations (%s) of container %s",
			a.exclusive.String(), id)

		// for cpus sliced off the shared pool, return then and update others
	default:
		p.shared = p.shared.Union(a.exclusive)

		p.Info("freed exclusive allocations (%s) of container %s",
			a.exclusive.String(), id)

		if err := p.updateSharedAllocations(); err != nil {
			return err
		}
	}

	p.cache.SetPolicyEntry(keySharedPool, p.shared)
	p.cache.SetPolicyEntry(keyAllocations,
		cache.Cachable(&cachedAllocations{a: p.allocations}))

	return nil
}

// updateSharedAllocations updates containers with shared allocations.
func (p *staticplus) updateSharedAllocations() error {
	avail := 1000 * p.shared.Size()

	for id, ca := range p.allocations {
		cac, ok := p.cache.LookupContainer(id)
		if !ok {
			p.Warn("can't find allocated container %s", id)
			// remove and recalculate shared CPUs
			p.delAssignment(ca, id)
			return p.updateSharedAllocations()
		}

		if !ca.exclusive.Intersection(p.sys.Isolated()).IsEmpty() && ca.shared == 0 {
			continue
		}

		cset := p.shared.Union(ca.exclusive)

		if avail <= 0 {
			cset = cset.Union(p.reserved)
			p.Warn("out of free shared (%s) capacity, using reserved pool (%s) as well",
				p.shared.String(), p.reserved.String())
		}

		if cac.GetCpusetCpus() != cset.String() {
			cac.SetCpusetCpus(cset.String())

			p.Info("container %s reallocated to exclusive (%s) and shared (%d mCPU) pool %s",
				cac.PrettyName(), ca.exclusive.String(), ca.shared, cset.String())
		}

		avail -= ca.shared
	}

	if avail < 0 {
		p.Warn("not enough free capacity in shared pool (%s): lacking %d mCPU",
			p.shared.String(), -avail)
	} else {
		p.Info("free shared (%s) capacity left: %d mCPU", p.shared.String(), avail)
	}

	return nil
}

// updatePools updates the pools according to the current asignments.
func (p *staticplus) updatePools() error {
	for id, ca := range p.allocations {
		if ca.exclusive.IsEmpty() {
			continue
		}

		isolated := ca.exclusive.Intersection(p.sys.Isolated())
		excshare := ca.exclusive.Difference(isolated)

		if !isolated.IsEmpty() && !excshare.IsEmpty() {
			return policyError("container %s has exclusive isolated (%s) and shareable (%s) cpus",
				id, isolated.String(), excshare.String())
		}

		p.isolated = p.isolated.Difference(isolated)
		p.shared = p.shared.Difference(excshare)
	}

	if err := p.updateSharedAllocations(); err != nil {
		return err
	}

	p.cache.SetPolicyEntry(keySharedPool, p.shared)
	p.cache.SetPolicyEntry(keyAllocations,
		cache.Cachable(&cachedAllocations{a: p.allocations}))

	return nil
}

// dumpPools dumps the current state of pools.
func (p *staticplus) dumpPools() {
	p.Info("current CPU pools:")
	offline := p.offline.String()
	if offline == "" {
		offline = "<none>"
	}
	isolated := p.isolated.String()
	if isolated == "" {
		isolated = "<none>"
	}

	p.Info("  offline:  %s", offline)
	p.Info("  reserved: %s", p.reserved.String())
	p.Info("  shared:   %s", p.shared.String())
	p.Info("  isolated: %s", isolated)

}

// dumpAllocations dumps the current allocations.
func (p *staticplus) dumpAllocations() {
	p.Info("container CPU allocations:")
	switch {
	case p.allocations == nil:
		p.Info("  <nil>")
	case len(p.allocations) == 0:
		p.Info("  <none>")
	default:
		for id, ca := range p.allocations {
			e := ca.exclusive.String()
			if e == "" {
				e = "<none>"
			}
			p.Info("  %s: exclusive: %s, shared: %d milli-cpu", id, e, ca.shared)
		}
	}
}

// Take up to cnt CPUs from a given CPU set to another.
func takeCPUs(from, to *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	cset, err := cpuallocator.AllocateCpus(from, cnt)
	if err != nil {
		return cset, err
	}

	if to != nil {
		*to = to.Union(cset)
	}

	return cset, err
}

//
// Cachable data types for storing private static-plus policy data in the cache.
//

// CachedAllocations implements Cache.Cachable boilerplate for Allocations.
type CachedAllocations interface {
	cache.Cachable
}

type cachedAllocations struct {
	a Allocations
}

var _ cache.Cachable = &cachedAllocations{}

var _ json.Marshaler = &cachedAllocations{}
var _ json.Unmarshaler = &cachedAllocations{}

func (ca *cachedAllocations) Get() interface{} {
	return *ca
}

func (ca *cachedAllocations) Set(value interface{}) {
	switch value.(type) {
	case cachedAllocations:
		ca.a = value.(cachedAllocations).a
	case *cachedAllocations:
		ca.a = value.(*cachedAllocations).a
	}
}

type marshallableAssignment struct {
	Exclusive string
	Shared    int
}

func (ca *cachedAllocations) MarshalJSON() ([]byte, error) {
	dst := make(map[string]*marshallableAssignment)
	for id, r := range ca.a {
		dst[id] = &marshallableAssignment{
			Exclusive: r.exclusive.String(),
			Shared:    r.shared,
		}
	}

	return json.Marshal(dst)
}

func (ca *cachedAllocations) UnmarshalJSON(data []byte) error {
	var err error

	dst := make(map[string]*marshallableAssignment)
	if err = json.Unmarshal(data, &dst); err != nil {
		return err
	}

	ca.a = make(map[string]*Assignment)
	for id, r := range dst {
		if r == nil {
			continue
		}
		cset, err := cpuset.Parse(r.Exclusive)
		if err != nil {
			return policyError("failed to unmarshal cpuset '%s': %v",
				r.Exclusive, err)
		}
		ca.a[id] = &Assignment{
			exclusive: cset,
			shared:    r.Shared,
		}
	}

	return nil
}

//
// Functions for calculating CFS cpu.shares and cpu.cfs_quota_us.
//
//   Notes: These functions are almost verbatim taken from the kubelet
//   code (from k8s.io/kubernetes/pkg/kubelet/cm/helpers_linux.go).
//   Since these are exported there, we could try to import them, set the
//   related feature gates (kubefeatures.CPUCFSQuotaPeriod) for ourselves
//   into the desired positions (disabled most probably for now) and use
//   the imported code.
//
const (
	MinShares     = 2
	SharesPerCPU  = 1024
	MilliCPUToCPU = 1000

	// 100000 is equivalent to 100ms
	QuotaPeriod    = 100000
	MinQuotaPeriod = 1000
)

// MilliCPUToQuota converts milliCPU to CFS quota and period values.
func MilliCPUToQuota(milliCPU int64, period int64) (quota int64) {
	// CFS quota is measured in two values:
	//  - cfs_period_us=100ms (the amount of time to measure usage across given by period)
	//  - cfs_quota=20ms (the amount of cpu time allowed to be used across a period)
	// so in the above example, you are limited to 20% of a single CPU
	// for multi-cpu environments, you just scale equivalent amounts
	// see https://www.kernel.org/doc/Documentation/scheduler/sched-bwc.txt for details

	if milliCPU == 0 {
		return
	}

	if true /*!utilfeature.DefaultFeatureGate.Enabled(kubefeatures.CPUCFSQuotaPeriod)*/ {
		period = QuotaPeriod
	}

	// we then convert your milliCPU to a value normalized over a period
	quota = (milliCPU * period) / MilliCPUToCPU

	// quota needs to be a minimum of 1ms.
	if quota < MinQuotaPeriod {
		quota = MinQuotaPeriod
	}
	return
}

// MilliCPUToShares converts the milliCPU to CFS shares.
func MilliCPUToShares(milliCPU int) int64 {
	if milliCPU == 0 {
		// Docker converts zero milliCPU to unset, which maps to kernel default
		// for unset: 1024. Return 2 here to really match kernel default for
		// zero milliCPU.
		return MinShares
	}
	// Conceptually (milliCPU / milliCPUToCPU) * sharesPerCPU, but factored to improve rounding.
	shares := (milliCPU * SharesPerCPU) / MilliCPUToCPU
	if shares < MinShares {
		return MinShares
	}
	return int64(shares)
}

//
// Automatically register us as a policy implementation.
//

// Implementation is the implementation we register with the policy module.
type Implementation func(*policy.BackendOptions) policy.Backend

// Name returns the name of this policy implementation.
func (n Implementation) Name() string {
	return PolicyName
}

// Description returns the desccription of this policy implementation.
func (n Implementation) Description() string {
	return PolicyDescription
}

// CreateFn returns the functions used to instantiate this policy.
func (n Implementation) CreateFn() policy.CreateFn {
	return policy.CreateFn(n)
}

var _ policy.Implementation = Implementation(nil)

func init() {
	policy.Register(Implementation(CreateStaticPlusPolicy))
}
