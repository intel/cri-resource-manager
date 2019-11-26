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

package policy

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/agent"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

// Domain represents a hardware resource domain that can be policied by a backend.
type Domain string

const (
	// DomainCPU is the CPU resource domain.
	DomainCPU Domain = "CPU"
	// DomainMemory is the memory resource domain.
	DomainMemory Domain = "Memory"
	// DomainHugePage is the hugepages resource domain.
	DomainHugePage Domain = "HugePages"
	// DomainCache is the CPU cache resource domain.
	DomainCache Domain = "Cache"
	// DomainMemoryBW is the memory resource bandwidth.
	DomainMemoryBW Domain = "MBW"
)

// Constraint describes constraint of one hardware domain
type Constraint interface{}

// ConstraintSet describes, per hardware domain, the resources available for a policy.
type ConstraintSet map[Domain]Constraint

// Options describes policy options
type Options struct {
	// Client interface to cri-resmgr agent
	AgentCli agent.Interface
}

// BackendOptions describes the options for a policy backend instance
type BackendOptions struct {
	// Resource availibility constraint
	Available ConstraintSet
	// Resource reservation constraint
	Reserved ConstraintSet
	// Client interface to cri-resmgr agent
	AgentCli agent.Interface
}

// CreateFn is the type for functions used to create a policy instance.
type CreateFn func(cache.Cache, *BackendOptions) Backend

// DataSyntax defines the syntax used to export data to a container.
type DataSyntax string

const (
	// ExportShell specifies shell (assignment) syntax.
	ExportShell DataSyntax = "shell"
	// ExportedResources is the name of the file container resources are exported to.
	ExportedResources = "resources.sh"
)

// Backend is the policy (decision making logic) interface exposed by implementations.
//
// A backends operates in a set of policy domains. Currently each policy domain
// corresponds to some particular hardware resource (CPU, memory, cache, etc).
//
type Backend interface {
	// Name gets the well-known name of this policy.
	Name() string
	// Description gives a verbose description about the policy implementation.
	Description() string
	// Start up and sycnhronizes the policy, using the given cache and resource constraints.
	Start([]cache.Container, []cache.Container) error
	// Sync synchronizes the policy, allocating/releasing the given containers.
	Sync([]cache.Container, []cache.Container) error
	// AllocateResources allocates resources to/for a container.
	AllocateResources(cache.Container) error
	// ReleaseResources release resources of a container.
	ReleaseResources(cache.Container) error
	// UpdateResources updates resource allocations of a container.
	UpdateResources(cache.Container) error
	// ExportResourceData provides resource data to export for the container.
	ExportResourceData(cache.Container, DataSyntax) []byte
}

// Policy is the exposed interface for container resource allocations decision making.
type Policy interface {
	// Start starts up policy, prepare for serving resource management requests.
	Start([]cache.Container, []cache.Container) error
	// PrepareDecisions prepares policy decisions.
	PrepareDecisions() error
	// QueryDecisions queries pending policy decisions.
	QueryDecisions() []cache.Container
	// CommitDecisions commits pending policy decisions.
	CommitDecisions() []cache.Container
	// AbortDecisions aborts (discard) pending policy decisions.
	AbortDecisions()
	// Sync synchronizes the state of the active policy.
	Sync([]cache.Container, []cache.Container) error
	// AlocateResources allocates resources to a container.
	AllocateResources(cache.Container) error
	// ReleaseResources releases resources of a container.
	ReleaseResources(cache.Container) error
	// UpdateResources updates resource allocations of a container.
	UpdateResources(cache.Container) error
}

// Policy instance/state.
type policy struct {
	cache   cache.Cache // system state cache
	backend Backend     // our active backend
}

// backend is a registered Backend.
type backend struct {
	name        string   // unqiue backend name
	description string   // verbose backend description
	create      CreateFn // backend creation function
}

// Out logger instance.
var log logger.Logger = logger.NewLogger("policy")

// Registered backends.
var backends = make(map[string]*backend)

// ActivePolicy returns the name of the policy to be activated.
func ActivePolicy() string {
	return opt.Policy
}

// NewPolicy creates a policy instance using the selected backend.
func NewPolicy(cache cache.Cache, o *Options) (Policy, error) {
	if opt.Policy == NullPolicy {
		return nil, nil
	}

	backend, ok := backends[opt.Policy]
	if !ok {
		return nil, policyError("unknown policy '%s'", opt.Policy)
	}

	p := &policy{
		cache: cache,
	}

	log.Info("creating new policy '%s'...", backend.name)
	if len(opt.Available) != 0 {
		log.Info("  with resource availability constraints:")
		for d := range opt.Available {
			log.Info("    - %s=%s", d, ConstraintToString(opt.Available[d]))
		}
	}

	if len(opt.Reserved) != 0 {
		log.Info("  with resource reservation constraints:")
		for d := range opt.Reserved {
			log.Info("    - %s=%s", d, ConstraintToString(opt.Reserved[d]))
		}
	}

	if log.DebugEnabled() {
		log.Debug("*** enabling debugging for %s", opt.Policy)
		logger.Get(opt.Policy).EnableDebug(true)
	} else {
		log.Debug("*** leaving debugging for %s alone", opt.Policy)
	}

	backendOpts := &BackendOptions{
		Available: opt.Available,
		Reserved:  opt.Reserved,
		AgentCli:  o.AgentCli,
	}
	p.backend = backend.create(p.cache, backendOpts)

	return p, nil
}

// Start starts up policy, preparing it for resving requests.
func (p *policy) Start(add []cache.Container, del []cache.Container) error {
	if opt.Policy == NullPolicy {
		return nil
	}

	log.Info("starting policy '%s'...", p.backend.Name())

	// Notes:
	//   Start() also creates an implicit transaction. This allows the backend
	//   to make decisions, attempting to adjust existing container allocations
	//   to any potential configuration changes since the previous startup. The
	//   caller is reponsible for querying and enforcing decisions.

	p.PrepareDecisions()

	if err := p.backend.Start(add, del); err != nil {
		p.AbortDecisions()
		return err
	}

	return nil
}

// PrepareDecisions prepares a policy decision making round.
func (p *policy) PrepareDecisions() error {
	return p.cache.StartTransaction()
}

// QueryDecisions queries pending policy decisions.
func (p *policy) QueryDecisions() []cache.Container {
	return p.cache.QueryTransaction()
}

// CommitDecisions commits pending policy decisions.
func (p *policy) CommitDecisions() []cache.Container {
	updated := p.cache.CommitTransaction()

	for _, c := range updated {
		if data := p.backend.ExportResourceData(c, ExportShell); data != nil {
			p.cache.WriteFile(c.GetCacheID(), ExportedResources, 0644, data)
		}
	}

	return updated
}

// AbortDecisions reverts changes made in the current policy decision making round.
func (p *policy) AbortDecisions() {
	p.cache.AbortTransaction()
}

// Sync synchronizes the active policy state.
func (p *policy) Sync(add []cache.Container, del []cache.Container) error {
	return p.backend.Sync(add, del)
}

// AllocateResources allocates resources for a container.
func (p *policy) AllocateResources(c cache.Container) error {
	return p.backend.AllocateResources(c)
}

// ReleaseResources release resources of a container.
func (p *policy) ReleaseResources(c cache.Container) error {
	return p.backend.ReleaseResources(c)
}

// UpdateResources updates resource allocations of a container.
func (p *policy) UpdateResources(c cache.Container) error {
	return p.backend.UpdateResources(c)
}

// Register registers a policy backend.
func Register(name, description string, create CreateFn) error {
	log.Info("registering policy '%s'...", name)

	if o, ok := backends[name]; ok {
		return policyError("policy %s already registered (%s)", name, o.description)
	}

	backends[name] = &backend{
		name:        name,
		description: description,
		create:      create,
	}

	return nil
}

// ConstraintToString returns the given constraint as a string.
func ConstraintToString(value Constraint) string {
	switch value.(type) {
	case cpuset.CPUSet:
		return "#" + value.(cpuset.CPUSet).String()
	case int:
		return strconv.Itoa(value.(int))
	case string:
		return value.(string)
	case resource.Quantity:
		qty := value.(resource.Quantity)
		return qty.String()
	default:
		return fmt.Sprintf("<???(type:%T)>", value)
	}
}
