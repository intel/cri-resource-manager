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
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/agent"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
	control "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/resource-control"
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
	// Rdt control interface
	Rdt control.CriRdt
}

// BackendOptions describes the options for a policy backend instance
type BackendOptions struct {
	// Resource availibility constraint
	Available ConstraintSet
	// Resource reservation constraint
	Reserved ConstraintSet
	// Client interface to cri-resmgr agent
	AgentCli agent.Interface
	// Rdt control interface
	Rdt control.CriRdt
}

// CreateFn is the type for functions used to create a policy instance.
type CreateFn func(*BackendOptions) Backend

// DataSyntax defines the syntax used to export data to a container.
type DataSyntax string

const (
	// ExportShell specifies shell (assignment) syntax.
	ExportShell DataSyntax = "shell"
	// ExportedResources is the name of the file container resources are exported to.
	ExportedResources = "resources.sh"
)

// Implementation attaches metadata (name, etc.) to a backend creation function.
type Implementation interface {
	// Name returns the well-known name for this policy.
	Name() string
	// Description returns a verbose description for this policy.
	Description() string
	// CreateFn creates an instance of this policy.
	CreateFn() CreateFn
}

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
	Start(cache.Cache, []cache.Container, []cache.Container) error
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
	// PostStart allocates resources after container is started
	PostStart(cache.Container) error
	// SetConfig sets the policy backend configuration
	SetConfig(string) error
}

// Policy is the exposed interface for container resource allocations decision making.
type Policy interface {
	// Start starts up policy, prepare for serving resource management requests.
	Start(cache.Cache, []cache.Container, []cache.Container) error
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
	// PostStart allocates resources after container is started
	PostStart(cache.Container) error
	// SetConfig sets the policy configuration
	SetConfig(*config.RawConfig) error
}

// Policy instance/state.
type policy struct {
	logger.Logger
	cache   cache.Cache // system state cache
	backend Backend     // our active backend
}

// ActivePolicy returns the name of the policy to be activated.
func ActivePolicy() string {
	return opt.Policy
}

// NewPolicy creates a policy instance using the selected backend.
func NewPolicy(o *Options) (Policy, error) {
	if opt.Policy == NullPolicy {
		return nil, nil
	}

	backend, ok := policies[opt.Policy]
	if !ok {
		return nil, policyError("unknown policy '%s'", opt.Policy)
	}

	p := &policy{
		Logger: logger.NewLogger("policy"),
	}

	p.Info("creating new policy '%s'...", backend.Name())
	if len(opt.Available) != 0 {
		p.Info("  with resource availability constraints:")
		for d := range opt.Available {
			p.Info("    - %s=%s", d, ConstraintToString(opt.Available[d]))
		}
	}

	if len(opt.Reserved) != 0 {
		p.Info("  with resource reservation constraints:")
		for d := range opt.Reserved {
			p.Info("    - %s=%s", d, ConstraintToString(opt.Reserved[d]))
		}
	}

	if p.DebugEnabled() {
		p.Debug("*** enabling debugging for %s", opt.Policy)
		logger.Get(opt.Policy).EnableDebug(true)
	} else {
		p.Debug("*** leaving debugging for %s alone", opt.Policy)
	}

	backendOpts := &BackendOptions{
		Available: opt.Available,
		Reserved:  opt.Reserved,
		AgentCli:  o.AgentCli,
		Rdt:       o.Rdt,
	}
	p.backend = backend.CreateFn()(backendOpts)

	return p, nil
}

// Start starts up policy, preparing it for resving requests.
func (p *policy) Start(cch cache.Cache, add []cache.Container, del []cache.Container) error {
	if opt.Policy == NullPolicy {
		return nil
	}

	if p.cache != nil {
		return policyError("policy %s has already been started", p.backend.Name())
	}

	p.Info("starting policy '%s'...", p.backend.Name())
	p.cache = cch

	// Notes:
	//   Start() also creates an implicit transaction. This allows the backend
	//   to make decisions, attempting to adjust existing container allocations
	//   to any potential configuration changes since the previous startup. The
	//   caller is reponsible for querying and enforcing decisions.

	p.PrepareDecisions()

	if err := p.backend.Start(p.cache, add, del); err != nil {
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

func (p *policy) PostStart(c cache.Container) error {
	return p.backend.PostStart(c)
}

// SetConfig updates the configuration of policy backend(s)
func (p *policy) SetConfig(conf *config.RawConfig) error {
	p.Info("updating configuration for policy %s...", p.backend.Name())

	c := extractPolicyConfig(p.backend.Name(), conf)
	err := p.backend.SetConfig(c)

	if err != nil {
		p.Error("failed to update configuration: %v", err)
		return policyError("failed to update configuration for policy %s: %v",
			p.backend.Name(), err)
	}

	p.Info("configuration update OK")

	return nil
}

// Register registers a policy implementation.
func Register(p Implementation) error {
	log := logger.Get("policy")
	name := p.Name()

	if p.CreateFn() == nil {
		return policyError("policy '%s' has a nil instantiation function", name)
	}

	log.Info("registering policy '%s'...", name)

	if _, ok := policies[name]; ok {
		return policyError("policy '%s' already registered", name)
	}

	policies[name] = p

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

// extractPolicyConfig gets the policy/node specific configuration from the
// full cri-resmgr raw config
func extractPolicyConfig(policyName string, rawConfig *config.RawConfig) string {
	config := defaultPolicyConfig(policyName)

	// Go through keys in allconfig (map) and try to find the policy configuration
	// Scheme for the policy config key is policy.<policy name>[.<node name>]
	if rawConfig != nil {
		for k, v := range rawConfig.Data {
			split := strings.SplitN(k, ".", 3)
			if split[0] == "policy" && len(split) > 1 && split[1] == policyName {
				if len(split) == 2 {
					config = v
				} else if split[2] == rawConfig.NodeName {
					config = v
					break
				}
			}
		}
	}

	return config
}

// Return the default configuration for the specified policy.
func defaultPolicyConfig(policyName string) string {
	// Just a stub for now, always returning empty.
	return ""
}
