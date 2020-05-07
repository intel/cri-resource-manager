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
	"bytes"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/agent"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/introspect"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/rdt"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
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
	// System provides system/HW/topology information
	System system.System
	// System state/cache
	Cache cache.Cache
	// Resource availibility constraint
	Available ConstraintSet
	// Resource reservation constraint
	Reserved ConstraintSet
	// Client interface to cri-resmgr agent
	AgentCli agent.Interface
}

// CreateFn is the type for functions used to create a policy instance.
type CreateFn func(*BackendOptions) Backend

const (
	// ExportedResources is the basename of the file container resources are exported to.
	ExportedResources = "resources.sh"
	// ExportSharedCPUs is the shell variable used to export shared container CPUs.
	ExportSharedCPUs = "SHARED_CPUS"
	// ExportIsolatedCPUs is the shell variable used to export isolated container CPUs.
	ExportIsolatedCPUs = "ISOLATED_CPUS"
	// ExportExclusiveCPUs is the shell variable used to export exclusive container CPUs.
	ExportExclusiveCPUs = "EXCLUSIVE_CPUS"
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
	// Rebalance tries an optimal allocation of resources for the current container.
	Rebalance() (bool, error)
	// ExportResourceData provides resource data to export for the container.
	ExportResourceData(cache.Container) map[string]string
	// Introspect provides data for external introspection.
	Introspect(*introspect.State)
}

// Policy is the exposed interface for container resource allocations decision making.
type Policy interface {
	// Start starts up policy, prepare for serving resource management requests.
	Start([]cache.Container, []cache.Container) error
	// Sync synchronizes the state of the active policy.
	Sync([]cache.Container, []cache.Container) error
	// AlocateResources allocates resources to a container.
	AllocateResources(cache.Container) error
	// ReleaseResources releases resources of a container.
	ReleaseResources(cache.Container) error
	// UpdateResources updates resource allocations of a container.
	UpdateResources(cache.Container) error
	// Rebalance tries to find an optimal allocation of resources for the current containers.
	Rebalance() (bool, error)
	// ExportResourceData exports/updates resource data for the container.
	ExportResourceData(cache.Container)
	// Introspect provides data for external introspection.
	Introspect() *introspect.State
	// Bypassed checks if local policy processing is effectively disabled/bypassed.
	Bypassed() bool
}

// Policy instance/state.
type policy struct {
	options Options            // policy options
	cache   cache.Cache        // system state cache
	active  Backend            // our active backend
	system  system.System      // system/HW/topology info
	inspsys *introspect.System // ditto for introspection
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
	sys, err := system.DiscoverSystem()
	if err != nil {
		return nil, policyError("failed to discover system topology: %v", err)
	}

	p := &policy{
		cache:   cache,
		system:  sys,
		options: *o,
	}

	if opt.Policy == NullPolicy {
		log.Info("activating '%s' policy (no active backend)", opt.Policy)
	} else {
		active, ok := backends[opt.Policy]
		if !ok {
			return nil, policyError("unknown policy '%s' requested", opt.Policy)
		}

		log.Info("activating '%s' policy...", active.name)

		if len(opt.Available) != 0 {
			log.Info("  with available resources:")
			for n, r := range opt.Available {
				log.Info("    - %s=%s", n, ConstraintToString(r))
			}
		}
		if len(opt.Reserved) != 0 {
			log.Info("  with reserved resources:")
			for n, r := range opt.Reserved {
				log.Info("    - %s=%s", n, ConstraintToString(r))
			}
		}

		if log.DebugEnabled() {
			logger.Get(opt.Policy).EnableDebug(true)
		}

		backendOpts := &BackendOptions{
			Cache:     p.cache,
			System:    p.system,
			Available: opt.Available,
			Reserved:  opt.Reserved,
			AgentCli:  o.AgentCli,
		}

		p.active = active.create(backendOpts)
	}

	return p, nil
}

// Start starts up policy, preparing it for resving requests.
func (p *policy) Start(add []cache.Container, del []cache.Container) error {
	if p.Bypassed() {
		log.Info("policy '%s' active, nothing to start...", opt.Policy)
		return nil
	}

	log.Info("starting policy '%s'...", p.active.Name())
	return p.active.Start(add, del)
}

func (p *policy) Bypassed() bool {
	return p.active == nil
}

// Sync synchronizes the active policy state.
func (p *policy) Sync(add []cache.Container, del []cache.Container) error {
	return p.active.Sync(add, del)
}

// AllocateResources allocates resources for a container.
func (p *policy) AllocateResources(c cache.Container) error {
	return p.active.AllocateResources(c)
}

// ReleaseResources release resources of a container.
func (p *policy) ReleaseResources(c cache.Container) error {
	return p.active.ReleaseResources(c)
}

// UpdateResources updates resource allocations of a container.
func (p *policy) UpdateResources(c cache.Container) error {
	return p.active.UpdateResources(c)
}

// Rebalance tries to find a more optimal allocation of resources for the current containers.
func (p *policy) Rebalance() (bool, error) {
	return p.active.Rebalance()
}

// ExportResourceData exports/updates resource data for the container.
func (p *policy) ExportResourceData(c cache.Container) {
	var buf bytes.Buffer

	data := p.active.ExportResourceData(c)
	keys := []string{}
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := data[key]
		if _, err := buf.WriteString(fmt.Sprintf("%s=%q\n", key, value)); err != nil {
			log.Error("container %s: failed to export resource data (%s=%q)",
				c.PrettyName(), key, value)
			buf.Reset()
			break
		}
	}

	p.cache.WriteFile(c.GetCacheID(), ExportedResources, 0644, buf.Bytes())
}

// Introspect provides data for external introspection/visualization.
func (p *policy) Introspect() *introspect.State {
	pods := p.cache.GetPods()
	state := &introspect.State{Pods: make(map[string]*introspect.Pod, len(pods))}

	for _, p := range pods {
		containers := p.GetContainers()
		if len(containers) == 0 {
			continue
		}

		pod := &introspect.Pod{
			ID:         p.GetID(),
			UID:        p.GetUID(),
			Name:       p.GetName(),
			Containers: make(map[string]*introspect.Container, len(containers)),
		}

		for _, c := range containers {
			container := &introspect.Container{
				ID:      c.GetID(),
				Name:    c.GetName(),
				Command: c.GetCommand(),
				Args:    c.GetArgs(),
				Hints:   introspect.TopologyHints(c.GetTopologyHints()),
			}
			resources := c.GetResourceRequirements()
			if req, ok := resources.Requests[corev1.ResourceCPU]; ok {
				if value := req.MilliValue(); value > 0 {
					container.CPURequest = value
				}
			}
			if lim, ok := resources.Limits[corev1.ResourceCPU]; ok {
				if value := lim.MilliValue(); value > 0 {
					container.CPULimit = value
				}
			}
			if req, ok := resources.Requests[corev1.ResourceMemory]; ok {
				if value := req.Value(); value > 0 {
					container.MemoryRequest = value
				}
			}
			if lim, ok := resources.Limits[corev1.ResourceMemory]; ok {
				if value := lim.Value(); value > 0 {
					container.MemoryLimit = value
				}
			}
			pod.Containers[container.ID] = container
		}
		state.Pods[pod.ID] = pod
	}

	if p.inspsys == nil {
		sys := &introspect.System{
			Sockets: make(map[int]*introspect.Socket, p.system.PackageCount()),
			Nodes:   make(map[int]*introspect.Node, p.system.NUMANodeCount()),
		}
		for _, id := range p.system.PackageIDs() {
			pkg := p.system.Package(id)
			sys.Sockets[int(id)] = &introspect.Socket{ID: int(id), CPUs: pkg.CPUSet().String()}
		}
		for _, id := range p.system.NodeIDs() {
			node := p.system.Node(id)
			sys.Nodes[int(id)] = &introspect.Node{ID: int(id), CPUs: node.CPUSet().String()}
		}
		sys.Isolated = p.system.Isolated().String()
		sys.Offlined = p.system.Offlined().String()
		p.inspsys = sys
	}

	rdtClassNames := []string{}
	for _, rdtClass := range rdt.GetClasses() {
		rdtClassNames = append(rdtClassNames, rdtClass.Name())
	}
	p.inspsys.RDTClasses = rdtClassNames
	p.inspsys.Policy = opt.Policy

	state.System = p.inspsys
	if !p.Bypassed() {
		p.active.Introspect(state)
	}

	return state
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
