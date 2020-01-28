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
	v1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"

	policyapi "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
)

const (
	// PolicyName is the symbol used to pull us in as a builtin policy.
	PolicyName = "topology-aware"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "A policy for HW-topology aware workload placement."
	// PolicyPath is the path of this policy in the configuration hierarchy.
	PolicyPath = "policy." + PolicyName
)

// allocations is our cache.Cachable for saving resource allocations in the cache.
type allocations struct {
	policy *policy
	CPU    map[string]CPUGrant
}

// TODO(rojkov): this is the interface of system.System we consume in this Go package. Should be moved to the package which is supposed to provide the interface.
type discoveredSystem interface {
	Node(system.ID) *system.Node
	Package(system.ID) *system.Package
	Offlined() cpuset.CPUSet
	Isolated() cpuset.CPUSet
	CPUSet() cpuset.CPUSet
	SocketCount() int
	NUMANodeCount() int
	PackageIDs() []system.ID
	NodeIDs() []system.ID
}

// policy is our runtime state for the topology aware policy.
type policy struct {
	options     policyapi.BackendOptions // options we were created or reconfigured with
	cache       cache.Cache              // pod/container cache
	sys         discoveredSystem         // system/HW topology info
	allowed     cpuset.CPUSet            // bounding set of CPUs we're allowed to use
	reserved    cpuset.CPUSet            // system-/kube-reserved CPUs
	reserveCnt  int                      // number of CPUs to reserve if given as resource.Quantity
	isolated    cpuset.CPUSet            // (our allowed set of) isolated CPUs
	nodes       map[string]Node          // pool nodes by name
	pools       []Node                   // pre-populated node slice for scoring, etc...
	root        Node                     // root of our pool/partition tree
	nodeCnt     int                      // number of pools
	depth       int                      // tree depth
	allocations allocations              // container pool assignments

}

// Make sure policy implements the policy.Backend interface.
var _ policyapi.Backend = &policy{}

// CreateTopologyAwarePolicy creates a new policy instance.
func CreateTopologyAwarePolicy(opts *policyapi.BackendOptions) policyapi.Backend {
	p := &policy{
		cache:   opts.Cache,
		sys:     opts.System,
		options: *opts,
	}

	p.nodes = make(map[string]Node)
	p.allocations = allocations{policy: p, CPU: make(map[string]CPUGrant, 32)}

	if err := p.checkConstraints(); err != nil {
		log.Fatal("failed to create topology-aware policy: %v", err)
	}

	if err := p.buildPoolsByTopology(); err != nil {
		log.Fatal("failed to create topology-aware policy: %v", err)
	}

	p.addImplicitAffinities()

	config.GetModule(PolicyPath).AddNotify(p.configNotify)

	p.root.Dump("<pre-start>")

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

	grant, err := p.allocatePool(container)
	if err != nil {
		return policyError("failed to allocate resources for %s: %v",
			container.PrettyName(), err)
	}

	if err := p.applyGrant(grant); err != nil {
		if _, _, err = p.releasePool(container); err != nil {
			log.Warn("failed to undo/release unapplicable grant %s: %v", grant, err)
			return policyError("failed to undo/release unapplicable grant %s: %v", grant, err)
		}
	}

	if err := p.updateSharedAllocations(grant); err != nil {
		log.Warn("failed to update shared allocations affected by %s: %v",
			container.PrettyName(), err)
	}

	p.root.Dump("<post-alloc>")

	return nil
}

// ReleaseResources is a resource release request for this policy.
func (p *policy) ReleaseResources(container cache.Container) error {
	log.Debug("releasing resources of %s...", container.PrettyName())

	grant, found, err := p.releasePool(container)
	if err != nil {
		return policyError("failed to release resources of %s: %v",
			container.PrettyName(), err)
	}

	if found {
		if err = p.updateSharedAllocations(grant); err != nil {
			log.Warn("failed to update shared allocations affected by %s: %v",
				container.PrettyName(), err)
		}
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

// ExportResourceData provides resource data to export for the container.
func (p *policy) ExportResourceData(c cache.Container) map[string]string {
	grant, ok := p.allocations.CPU[c.GetCacheID()]
	if !ok {
		return nil
	}

	data := map[string]string{}
	shared := grant.SharedCPUs().String()
	isolated := grant.ExclusiveCPUs().Intersection(grant.GetNode().GetCPU().IsolatedCPUs())
	exclusive := grant.ExclusiveCPUs().Difference(isolated).String()

	if shared != "" {
		data[policyapi.ExportSharedCPUs] = shared
	}
	if isolated.String() != "" {
		data[policyapi.ExportIsolatedCPUs] = isolated.String()
	}
	if exclusive != "" {
		data[policyapi.ExportExclusiveCPUs] = exclusive
	}

	return data
}

func (p *policy) configNotify(event config.Event, source config.Source) error {
	log.Info("configuration %s:", event)
	log.Info("  - pin containers to CPUs: %v", opt.PinCPU)
	log.Info("  - pin containers to memory: %v", opt.PinMemory)
	log.Info("  - prefer isolated CPUs: %v", opt.PreferIsolated)
	log.Info("  - prefer shared CPUs: %v", opt.PreferShared)

	// TODO: We probably should release and reallocate resources for all containers
	//   to honor the latest configuration. Depending on the changes that might be
	//   disruptive to some containers, so whether we do so or not should probably
	//   be part of the configuration as well.

	p.saveConfig()

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
	}

	return nil
}

func (p *policy) restoreCache() error {
	if !p.restoreConfig() {
		log.Warn("no saved configuration found in cache...")
		p.saveConfig()
	}

	if !p.restoreAllocations() {
		log.Warn("no allocations found in cache...")
		p.saveAllocations()
	} else {
		p.allocations.Dump(log.Info, "restored ")
	}

	return nil
}

// Register us as a policy implementation.
func init() {
	policyapi.Register(PolicyName, PolicyDescription, CreateTopologyAwarePolicy)
}
