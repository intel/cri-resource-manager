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
	"errors"

	v1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/introspect"

	policyapi "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	idset "github.com/intel/goresctrl/pkg/utils"
)

const (
	// PolicyName is the name used to activate this policy implementation.
	PolicyName = "topology-aware"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "A policy for prototyping memory tiering."
	// PolicyPath is the path of this policy in the configuration hierarchy.
	PolicyPath = "policy." + PolicyName
	// AliasName is the 'memtier' alias name for this policy.
	AliasName = "memtier"
	// AliasPath is the 'memtier' alias configuration path for this policy.
	AliasPath = "policy." + AliasName

	// ColdStartDone is the event generated for the end of a container cold start period.
	ColdStartDone = "cold-start-done"
)

// allocations is our cache.Cachable for saving resource allocations in the cache.
type allocations struct {
	policy *policy
	grants map[string]Grant
}

// policy is our runtime state for this policy.
type policy struct {
	options      *policyapi.BackendOptions // options we were created or reconfigured with
	cache        cache.Cache               // pod/container cache
	sys          system.System             // system/HW topology info
	allowed      cpuset.CPUSet             // bounding set of CPUs we're allowed to use
	reserved     cpuset.CPUSet             // system-/kube-reserved CPUs
	reserveCnt   int                       // number of CPUs to reserve if given as resource.Quantity
	isolated     cpuset.CPUSet             // (our allowed set of) isolated CPUs
	nodes        map[string]Node           // pool nodes by name
	pools        []Node                    // pre-populated node slice for scoring, etc...
	root         Node                      // root of our pool/partition tree
	nodeCnt      int                       // number of pools
	depth        int                       // tree depth
	allocations  allocations               // container pool assignments
	cpuAllocator cpuallocator.CPUAllocator // CPU allocator used by the policy
	coldstartOff bool                      // coldstart forced off (have movable PMEM zones)
	isAlias      bool                      // whether started by referencing AliasName
}

// Make sure policy implements the policy.Backend interface.
var _ policyapi.Backend = &policy{}

// Whether we have coldstart forced off due to PMEM in movable memory zones.
var coldStartOff bool

// CreateTopologyAwarePolicy creates a new policy instance.
func CreateTopologyAwarePolicy(opts *policyapi.BackendOptions) policyapi.Backend {
	return createPolicy(opts, false)
}

// CreateMemtierPolicy creates a new policy instance, aliased as 'memtier'.
func CreateMemtierPolicy(opts *policyapi.BackendOptions) policyapi.Backend {
	return createPolicy(opts, true)
}

// createPolicy creates a new policy instance.
func createPolicy(opts *policyapi.BackendOptions, isAlias bool) policyapi.Backend {
	p := &policy{
		cache:        opts.Cache,
		sys:          opts.System,
		options:      opts,
		cpuAllocator: cpuallocator.NewCPUAllocator(opts.System),
		isAlias:      isAlias,
	}

	if isAlias {
		*opt = *aliasOpt
	}

	if err := p.initialize(); err != nil {
		log.Fatal("failed to initialize %s policy: %v", PolicyName, err)
	}

	p.registerImplicitAffinities()

	config.GetModule(policyapi.ConfigPath).AddNotify(p.configNotify)

	return p
}

// Name returns the name of this policy.
func (p *policy) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (p *policy) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (p *policy) Start(add []cache.Container, del []cache.Container) error {
	if err := p.restoreCache(); err != nil {
		return policyError("failed to start: %v", err)
	}

	// Turn coldstart forcibly off if we have movable non-DRAM memory.
	// Note that although this can change dynamically we only check it
	// during startup and trust users to either not fiddle with memory
	// or restart us if they do.
	p.checkColdstartOff()

	p.root.Dump("<post-start>")

	return p.Sync(add, del)
}

// Sync synchronizes the state of this policy.
func (p *policy) Sync(add []cache.Container, del []cache.Container) error {
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
func (p *policy) AllocateResources(container cache.Container) error {
	log.Debug("allocating resources for %s...", container.PrettyName())

	grant, err := p.allocatePool(container, "")
	if err != nil {
		return policyError("failed to allocate resources for %s: %v",
			container.PrettyName(), err)
	}
	p.applyGrant(grant)
	p.updateSharedAllocations(&grant)

	p.root.Dump("<post-alloc>")

	return nil
}

// ReleaseResources is a resource release request for this policy.
func (p *policy) ReleaseResources(container cache.Container) error {
	log.Debug("releasing resources of %s...", container.PrettyName())

	if grant, found := p.releasePool(container); found {
		p.updateSharedAllocations(&grant)
	}

	p.root.Dump("<post-release>")

	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (p *policy) UpdateResources(c cache.Container) error {
	log.Debug("(not) updating container %s...", c.PrettyName())
	return nil
}

// Rebalance tries to find an optimal allocation of resources for the current containers.
func (p *policy) Rebalance() (bool, error) {
	var errors error

	containers := p.cache.GetContainers()
	movable := []cache.Container{}

	for _, c := range containers {
		if c.GetQOSClass() != v1.PodQOSGuaranteed {
			p.ReleaseResources(c)
			movable = append(movable, c)
		}
	}

	for _, c := range movable {
		if err := p.AllocateResources(c); err != nil {
			if errors == nil {
				errors = err
			} else {
				errors = policyError("%v, %v", errors, err)
			}
		}
	}

	return true, errors
}

// HandleEvent handles policy-specific events.
func (p *policy) HandleEvent(e *events.Policy) (bool, error) {
	log.Debug("received policy event %s.%s with data %v...", e.Source, e.Type, e.Data)

	switch e.Type {
	case events.ContainerStarted:
		c, ok := e.Data.(cache.Container)
		if !ok {
			return false, policyError("%s event: expecting cache.Container Data, got %T",
				e.Type, e.Data)
		}
		log.Info("triggering coldstart period (if necessary) for %s", c.PrettyName())
		return false, p.triggerColdStart(c)
	case ColdStartDone:
		id, ok := e.Data.(string)
		if !ok {
			return false, policyError("%s event: expecting container ID Data, got %T",
				e.Type, e.Data)
		}
		c, ok := p.cache.LookupContainer(id)
		if !ok {
			// TODO: This is probably a race condition. Should we return nil error here?
			return false, policyError("%s event: failed to lookup container %s", id)
		}
		log.Info("finishing coldstart period for %s", c.PrettyName())
		return p.finishColdStart(c)
	}
	return false, nil
}

// Introspect provides data for external introspection.
func (p *policy) Introspect(state *introspect.State) {
	pools := make(map[string]*introspect.Pool, len(p.pools))
	for _, node := range p.nodes {
		cpus := node.GetSupply()
		pool := &introspect.Pool{
			Name:   node.Name(),
			CPUs:   cpus.SharableCPUs().Union(cpus.IsolatedCPUs()).String(),
			Memory: node.GetMemset(memoryAll).String(),
		}
		if parent := node.Parent(); !parent.IsNil() {
			pool.Parent = parent.Name()
		}
		if children := node.Children(); len(children) > 0 {
			pool.Children = make([]string, 0, len(children))
			for _, c := range children {
				pool.Children = append(pool.Children, c.Name())
			}
		}
		pools[pool.Name] = pool
	}
	state.Pools = pools

	assignments := make(map[string]*introspect.Assignment, len(p.allocations.grants))
	for _, g := range p.allocations.grants {
		a := &introspect.Assignment{
			ContainerID:   g.GetContainer().GetID(),
			CPUShare:      g.SharedPortion(),
			ExclusiveCPUs: g.ExclusiveCPUs().Union(g.IsolatedCPUs()).String(),
			Pool:          g.GetCPUNode().Name(),
		}
		if g.SharedPortion() > 0 || a.ExclusiveCPUs == "" {
			a.SharedCPUs = g.SharedCPUs().String()
		}
		assignments[a.ContainerID] = a
	}
	state.Assignments = assignments
}

// DescribeMetrics generates policy-specific prometheus metrics data descriptors.
func (p *policy) DescribeMetrics() []*prometheus.Desc {
	return nil
}

// PollMetrics provides policy metrics for monitoring.
func (p *policy) PollMetrics() policyapi.Metrics {
	return nil
}

// CollectMetrics generates prometheus metrics from cached/polled policy-specific metrics data.
func (p *policy) CollectMetrics(policyapi.Metrics) ([]prometheus.Metric, error) {
	return nil, nil
}

// ExportResourceData provides resource data to export for the container.
func (p *policy) ExportResourceData(c cache.Container) map[string]string {
	grant, ok := p.allocations.grants[c.GetCacheID()]
	if !ok {
		return nil
	}

	data := map[string]string{}
	shared := grant.SharedCPUs().String()
	isolated := grant.ExclusiveCPUs().Intersection(grant.GetCPUNode().GetSupply().IsolatedCPUs())
	exclusive := grant.ExclusiveCPUs().Difference(isolated).String()

	if grant.SharedPortion() > 0 && shared != "" {
		data[policyapi.ExportSharedCPUs] = shared
	}
	if isolated.String() != "" {
		data[policyapi.ExportIsolatedCPUs] = isolated.String()
	}
	if exclusive != "" {
		data[policyapi.ExportExclusiveCPUs] = exclusive
	}

	mems := grant.Memset()
	dram := idset.NewIDSet()
	pmem := idset.NewIDSet()
	hbm := idset.NewIDSet()
	for _, id := range mems.SortedMembers() {
		node := p.sys.Node(id)
		switch node.GetMemoryType() {
		case system.MemoryTypeDRAM:
			dram.Add(id)
		case system.MemoryTypePMEM:
			pmem.Add(id)
			/*
				case system.MemoryTypeHBM:
					hbm.Add(id)
			*/
		}
	}
	data["ALL_MEMS"] = mems.String()
	if dram.Size() > 0 {
		data["DRAM_MEMS"] = dram.String()
	}
	if pmem.Size() > 0 {
		data["PMEM_MEMS"] = pmem.String()
	}
	if hbm.Size() > 0 {
		data["HBM_MEMS"] = hbm.String()
	}

	return data
}

// reallocateResources reallocates the given containers using the given pool hints
func (p *policy) reallocateResources(containers []cache.Container, pools map[string]string) error {
	errs := []error{}

	log.Info("reallocating resources...")

	cache.SortContainers(containers)

	for _, c := range containers {
		p.releasePool(c)
	}
	for _, c := range containers {
		log.Debug("reallocating resources for %s...", c.PrettyName())

		grant, err := p.allocatePool(c, pools[c.GetCacheID()])
		if err != nil {
			errs = append(errs, err)
		} else {
			p.applyGrant(grant)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	p.updateSharedAllocations(nil)

	p.root.Dump("<post-realloc>")

	return nil
}

func (p *policy) configNotify(event config.Event, source config.Source) error {
	policyName := PolicyName
	if p.isAlias {
		policyName = AliasName
		*opt = *aliasOpt
	}
	log.Info("%s configuration %s:", policyName, event)
	log.Info("  - pin containers to CPUs: %v", opt.PinCPU)
	log.Info("  - pin containers to memory: %v", opt.PinMemory)
	log.Info("  - prefer isolated CPUs: %v", opt.PreferIsolated)
	log.Info("  - prefer shared CPUs: %v", opt.PreferShared)
	log.Info("  - reserved pool namespaces: %v", opt.ReservedPoolNamespaces)

	var allowed, reserved cpuset.CPUSet
	var reinit bool

	if cpus, ok := p.options.Available[policyapi.DomainCPU]; ok {
		if cset, ok := cpus.(cpuset.CPUSet); ok {
			allowed = cset
		}
	}
	if cpus, ok := p.options.Reserved[policyapi.DomainCPU]; ok {
		switch v := cpus.(type) {
		case cpuset.CPUSet:
			reserved = v
		case resapi.Quantity:
			reserveCnt := (int(v.MilliValue()) + 999) / 1000
			if reserveCnt != p.reserveCnt {
				log.Warn("CPU reservation has changed (%v, was %v)",
					reserveCnt, p.reserveCnt)
				reinit = true
			}
		}
	}

	if !allowed.Equals(p.allowed) {
		if !(allowed.Size() == 0 && p.allowed.Size() == 0) {
			log.Warn("allowed cpuset changed (%s, was %s)",
				allowed.String(), p.allowed.String())
			reinit = true
		}
	}
	if !reserved.Equals(p.reserved) {
		if !(reserved.Size() == 0 && p.reserved.Size() == 0) {
			log.Warn("reserved cpuset changed (%s, was %s)",
				reserved.String(), p.reserved.String())
			reinit = true
		}
	}

	//
	// Notes:
	//   If the allowed or reserved resources have changed, we need to
	//   rebuild our pool hierarchy using the updated constraints and
	//   also update the existing allocations accordingly. We do this
	//   first reinitializing the policy then reloading the allocations
	//   from the cache. If we fail, we restore the original state of
	//   the policy and reject the new configuration.
	//

	if reinit {
		log.Warn("reinitializing %s policy...", PolicyName)

		savedPolicy := *p
		allocations := savedPolicy.allocations.clone()

		if err := p.initialize(); err != nil {
			*p = savedPolicy
			return policyError("failed to reconfigure: %v", err)
		}

		for _, grant := range allocations.grants {
			if err := grant.RefetchNodes(); err != nil {
				*p = savedPolicy
				return policyError("failed to reconfigure: %v", err)
			}
		}

		log.Warn("updating existing allocations...")
		if err := p.restoreAllocations(&allocations); err != nil {
			*p = savedPolicy
			return policyError("failed to reconfigure: %v", err)
		}

		p.root.Dump("<post-config>")
	}

	return nil
}

// Initialize or reinitialize the policy.
func (p *policy) initialize() error {
	p.nodes = nil
	p.pools = nil
	p.root = nil
	p.nodeCnt = 0
	p.depth = 0
	p.allocations = p.newAllocations()

	if err := p.checkConstraints(); err != nil {
		return err
	}

	if err := p.buildPoolsByTopology(); err != nil {
		return err
	}

	return nil
}

// Check the constraints passed to us.
func (p *policy) checkConstraints() error {
	if c, ok := p.options.Available[policyapi.DomainCPU]; ok {
		p.allowed = c.(cpuset.CPUSet)
	} else {
		// default to all online cpus
		p.allowed = p.sys.CPUSet().Difference(p.sys.Offlined())
	}

	p.isolated = p.sys.Isolated().Intersection(p.allowed)

	c, ok := p.options.Reserved[policyapi.DomainCPU]
	if !ok {
		return policyError("cannot start without CPU reservation")
	}

	switch c.(type) {
	case cpuset.CPUSet:
		p.reserved = c.(cpuset.CPUSet)
		// check that all reserved CPUs are in the allowed set
		if !p.reserved.Difference(p.allowed).IsEmpty() {
			return policyError("invalid reserved cpuset %s, some CPUs (%s) are not "+
				"part of the online allowed cpuset (%s)", p.reserved,
				p.reserved.Difference(p.allowed), p.allowed)
		}
		// check that none of the reserved CPUs are isolated
		if !p.reserved.Intersection(p.isolated).IsEmpty() {
			return policyError("invalid reserved cpuset %s, some CPUs (%s) are also isolated",
				p.reserved.Intersection(p.isolated))
		}

	case resapi.Quantity:
		qty := c.(resapi.Quantity)
		p.reserveCnt = (int(qty.MilliValue()) + 999) / 1000
		// Use CpuAllocator to pick reserved CPUs among
		// allowed ones. Because using those CPUs is allowed,
		// they remain (they are put back) in the allowed set.
		cset, err := p.cpuAllocator.AllocateCpus(&p.allowed, p.reserveCnt, cpuallocator.PriorityNormal)
		p.allowed = p.allowed.Union(cset)
		if err != nil {
			log.Fatal("cannot reserve %dm CPUs for ReservedResources from AvailableResources: %s", qty.MilliValue(), err)
		}
		p.reserved = cset
	}

	if p.reserved.IsEmpty() {
		return policyError("cannot start without CPU reservation")
	}

	return nil
}

func (p *policy) restoreCache() error {
	allocations := p.newAllocations()
	if p.cache.GetPolicyEntry(keyAllocations, &allocations) {
		if err := p.restoreAllocations(&allocations); err != nil {
			return policyError("failed to restore allocations from cache: %v", err)
		}
		p.allocations.Dump(log.Info, "restored ")
	}
	p.saveAllocations()

	return nil
}

func (p *policy) checkColdstartOff() {
	for _, id := range p.sys.NodeIDs() {
		node := p.sys.Node(id)
		if node.GetMemoryType() == system.MemoryTypePMEM {
			if !node.HasNormalMemory() {
				coldStartOff = true
				log.Error("coldstart forced off: NUMA node #%d does not have normal memory", id)
				return
			}
		}
	}
}

// newAllocations returns a new initialized empty set of allocations.
func (p *policy) newAllocations() allocations {
	return allocations{policy: p, grants: make(map[string]Grant)}
}

// clone creates a copy of the allocation.
func (a *allocations) clone() allocations {
	o := allocations{policy: a.policy, grants: make(map[string]Grant)}
	for id, grant := range a.grants {
		o.grants[id] = grant.Clone()
	}
	return o
}

// getContainerPoolHints creates container pool hints for the current grants.
func (a *allocations) getContainerPoolHints() ([]cache.Container, map[string]string) {
	containers := make([]cache.Container, 0, len(a.grants))
	hints := make(map[string]string)
	for _, grant := range a.grants {
		c := grant.GetContainer()
		containers = append(containers, c)
		hints[c.GetCacheID()] = grant.GetCPUNode().Name()
	}
	return containers, hints
}

// Register us as a policy implementation.
func init() {
	policyapi.Register(PolicyName, PolicyDescription, CreateTopologyAwarePolicy)
	policyapi.Register(AliasName, PolicyDescription, CreateMemtierPolicy)
}
