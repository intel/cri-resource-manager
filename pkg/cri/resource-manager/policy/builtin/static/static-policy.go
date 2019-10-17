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

package static

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	logger "github.com/intel/cri-resource-manager/pkg/log"

	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	control "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/resource-control"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
)

const (
	// PolicyName is the symbol used to pull us in as a builtin policy.
	PolicyName = "static"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "A reimplementation of the static CPU Manager policy."
)

type static struct {
	logger.Logger

	config        string               // active configuration
	available     policy.ConstraintSet // resource availability constraints
	reserved      policy.ConstraintSet // system/kube-reservation constraints
	reservedCpus  cpuset.CPUSet        // CPUs reserved for system- and kube-tasks
	availableCpus cpuset.CPUSet        // CPUs free usable by this policy
	isolatedCpus  cpuset.CPUSet        // available CPUs isolated from normal scheduling
	sys           *sysfs.System        // system/topology information
	numHT         int                  // number of hyperthreads per core
	state         cache.Cache          // policy/state cache
	rdt           control.CriRdt       // RDT resource control interface
}

// Make sure static implements the policy backend interface.
var _ policy.Backend = &static{}

const (
	// keyPreferIsolated is the annotation used to mark pods preferring isolated CPUs.
	keyPreferIsolated = "prefer-isolated-cpus"
)

// NewStaticPolicy creates a new policy instance.
func NewStaticPolicy(opts *policy.BackendOptions) policy.Backend {
	s := &static{
		config:    opts.Config,
		Logger:    logger.NewLogger(PolicyName),
		available: opts.Available,
		reserved:  opts.Reserved,
		rdt:       opts.Rdt,
	}

	s.Info("creating policy...")

	sys, err := sysfs.DiscoverSystem()
	if err != nil {
		s.Fatal("failed to discover system topology: %v", err)
	}

	s.sys = sys
	s.numHT = sys.CPU(sysfs.ID(0)).ThreadCPUSet().Size()

	if err := s.checkConstraints(); err != nil {
		s.Fatal("cannot start with given constraints: %v", err)
	}

	return s
}

// Name returns the name of this policy.
func (s *static) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (s *static) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (s *static) Start(state cache.Cache, add []cache.Container, del []cache.Container) error {
	s.Debug("starting up...")

	if s.config != "" {
		err := s.SetConfig(s.config)
		if err != nil {
			return err
		}
	}

	if err := s.allocateReserved(); err != nil {
		return policyError("failed allocate reserved CPUs: %v", err)
	}

	s.Info("using reserved CPUs: %s", s.reservedCpus.String())
	s.Info("using available CPUs: %s", s.availableCpus.String())

	if err := s.validateState(state); err != nil {
		return policyError("failed to start with given cache/state: %v", err)
	}

	s.validateAssignments()

	return s.Sync(add, del)
}

// Sync synchronizes the active policy state.
func (s *static) Sync(add []cache.Container, del []cache.Container) error {
	s.Debug("synchronizing state...")
	for _, c := range del {
		s.ReleaseResources(c)
	}
	for _, c := range add {
		s.AllocateResources(c)
	}

	return nil
}

// AllocateResources is a resource allocation request for this policy.
func (s *static) AllocateResources(c cache.Container) error {
	s.Info("allocating resource for container %s...", c.PrettyName())

	container := c
	containerID := c.GetCacheID()
	pod, found := c.GetPod()
	if !found {
		return policyError("can't find pod for container %s", containerID)
	}

	err := s.AddContainer(pod, container, containerID)

	return err
}

// ReleaseResources is a resource release request for this policy.
func (s *static) ReleaseResources(c cache.Container) error {
	s.Info("releasing resources of container %s...", c.PrettyName())

	containerID := c.GetCacheID()
	err := s.RemoveContainer(containerID)

	return err
}

// UpdateResources is a resource allocation update request for this policy.
func (s *static) UpdateResources(c cache.Container) error {
	s.Debug("(not) updating container %s...", c.PrettyName())
	return nil
}

// ExportResourceData provides resource data to export for the container.
func (s *static) ExportResourceData(c cache.Container, syntax policy.DataSyntax) []byte {
	data := ""

	cset, ok := s.GetCPUSet(c.GetCacheID())
	if !ok {
		cset = s.GetDefaultCPUSet()
		data += "SHARED_CPUS=\"" + cset.String() + "\"\n"
	} else {
		isolated := cset.Intersection(s.sys.Isolated())
		if isolated.String() != "" {
			data += "ISOLATED_CPUS=\"" + isolated.String() + "\"\n"
		}
		exclusive := cset.Difference(s.sys.Isolated())
		if exclusive.String() != "" {
			data += "EXCLUSIVE_CPUS=\"" + exclusive.String() + "\"\n"
		}
	}

	return []byte(data)
}

// PostStart allocates resources after container is started
func (s *static) PostStart(c cache.Container) error {
	if opt.Rdt == TristateOff {
		return nil
	} else if opt.Rdt == TristateOn && s.rdt == nil {
		return policyError("RDT required but not available")
	}
	if s.rdt != nil {
		pod, ok := c.GetPod()
		if !ok {
			return policyError("Pod of container %q not found", c.GetID())
		}
		qos := string(pod.GetQOSClass())

		s.Info("setting RDT class of container %q to %q", c.GetID(), qos)

		return s.rdt.SetContainerClass(c, qos)
	}
	return nil
}

// SetConfig sets the policy backend configuration
func (s *static) SetConfig(conf string) error {
	conf = strings.TrimSpace(conf)

	if conf == s.config {
		s.Info("no configuration changes")
		return nil
	}

	newConf, err := parseConfData([]byte(conf))
	if err != nil {
		return policyError("failed to parse configuration: %v", err)
	}

	if opt == *newConf {
		s.Info("no configuration changes")
		return nil
	}

	if newConf.Rdt == TristateOn && s.rdt == nil {
		return policyError("RDT requested but not available")
	}

	opt = *newConf

	if opt.RelaxedIsolation {
		s.Info("isolated exclusive CPUs: globally preferred (all pods)")
	} else {
		s.Info("isolated exclusive CPUs: per-pod (by annotation '%s')",
			kubernetes.ResmgrKey(keyPreferIsolated))
	}

	s.Info("rdt support set to %q", opt.Rdt.String())

	s.config = conf

	return nil
}

// assignableCPUs returns the set of unassigned CPUs minus the reserved set.
func (s *static) assignableCPUs(numCPUs int) cpuset.CPUSet {
	cset := s.GetDefaultCPUSet().Difference(s.reservedCpus)

	if cset.Size() < numCPUs && s.isolatedCpus.Size() > 0 {
		s.Warn("not enough non-isolated CPUs (%d) left for request (%d)",
			cset.Size(), numCPUs)
		cset = cset.Union(s.isolatedCpus)
	}

	return cset
}

// AddContainer is the CPU Manager static policy AddContainer function.
func (s *static) AddContainer(pod cache.Pod, container cache.Container, containerID string) error {
	if numCPUs := s.guaranteedCPUs(pod, container); numCPUs != 0 {
		s.Info("[cpumanager] static policy: AddContainer (pod: %s, container: %s, container id: %s)", pod.GetName(), container.GetName(), containerID)
		// container belongs in an exclusively allocated pool

		if _, ok := s.GetCPUSet(containerID); ok {
			s.Info("[cpumanager] static policy: container already present in state, skipping (container: %s, container id: %s)", container.GetName(), containerID)
			return nil
		}

		cpuset, err := s.allocateCPUs(numCPUs, containerID)
		if err != nil {
			s.Error("[cpumanager] unable to allocate %d CPUs (container id: %s, error: %v)", numCPUs, containerID, err)
			return err
		}
		s.Debug("setting cpuset of %s to allocated %s", containerID, cpuset)
		s.SetCPUSet(containerID, cpuset)
	}
	// container belongs in the shared pool (nothing to do; use default cpuset)
	return nil
}

// RemoveContainer is the CPU Manager static policy RemoveContainer function.
func (s *static) RemoveContainer(containerID string) error {
	s.Info("[cpumanager] static policy: RemoveContainer (container id: %s)", containerID)
	if toRelease, ok := s.GetCPUSet(containerID); ok {
		s.Delete(containerID)
		isolated := toRelease.Intersection(s.sys.Isolated())
		ordinary := toRelease.Difference(isolated)

		// Mutate the shared pool, adding released cpus.
		s.SetDefaultCPUSet(s.GetDefaultCPUSet().Union(ordinary))
		s.isolatedCpus = s.isolatedCpus.Union(isolated)
	}
	return nil
}

// Notes:
//   By default we assume workloads are not isolation-aware. We
//   only allocate isolated CPUs exclusively to containers if
//
//     - we globally prefer isolated exclusive CPUs, or
//     - the pod prefers isolated exclusive CPUs, or
//     - the container asks a single hyperthread worth of CPU, or
//     - the container asks for a full core worth of CPU
//
//   For the full core worth of CPU case, if the result of the
//   allocation is not a single full core, we fall back to taking
//   ordinary CPUs, unless isolated ones are explicitly preferred.

// cpuPreference checks if isolated CPUs should be tried and are preferred for an allocation.
func (s *static) cpuPreference(containerID string, numCPUs int) (bool, bool) {
	var try, prefer bool

	// Check if we prefer isolated CPUs (globally of per this containers pod).
	if opt.RelaxedIsolation {
		prefer = true
	} else {
		if c, ok := s.state.LookupContainer(containerID); ok {
			p, found := c.GetPod()
			if !found {
				s.Warn("can't find pod for container %s", c.GetID())
				return false, false
			}

			if value, ok := p.GetResmgrAnnotation(keyPreferIsolated); ok {
				if isolated, err := strconv.ParseBool(value); isolated {
					prefer = true
				} else {
					if err != nil {
						s.Error("invalid annotation '%s' on container %s, expecting boolean: %v",
							keyPreferIsolated, c.PrettyName(), err)
					}
				}
			}
		}
	}

	if prefer {
		return true, true
	}

	// For a single HT of CPU or a full core of CPU we always try isolated CPUs.
	if (numCPUs == 1 || numCPUs == s.numHT) && s.isolatedCpus.Size() >= numCPUs {
		try = true
	}

	return try, prefer
}

// allocateOrdinaryCPUs tries to take a number of non-isolated CPUs.
func (s *static) allocateOrdinaryCPUs(numCPUs int) (cpuset.CPUSet, error) {
	assignable := s.assignableCPUs(numCPUs)
	result, err := takeByTopology(assignable, numCPUs)

	if err != nil {
		return cpuset.NewCPUSet(), err
	}

	s.Info("allocated %d ordinary CPUs: %s", numCPUs, result.String())

	return result, nil
}

// allocateIsolatedCPUs tries to take a number of isolated CPUs, falling back to ordinary ones.
func (s *static) allocateIsolatedCPUs(numCPUs int, prefer bool) (cpuset.CPUSet, error) {
	result, err := takeByTopology(s.isolatedCpus, numCPUs)

	switch {
	case err != nil:
		s.Info("falling back to %d ordinary CPUs", numCPUs)
		return s.allocateOrdinaryCPUs(numCPUs)
	case numCPUs == 1 || prefer:
		s.Info("allocated %d isolated CPUs: %s", numCPUs, result.String())
		return result, nil
	case s.fullCore(result):
		s.Info("allocated %d isolated CPUs: %s", numCPUs, result.String())
		return result, nil
	default:
		s.Info("falling back to %d ordinary CPUs", numCPUs)
		return s.allocateOrdinaryCPUs(numCPUs)
	}
}

// fullCore checks if the CPUs in a cpuset consume a full single core.
func (s *static) fullCore(cset cpuset.CPUSet) bool {
	if cset.Size() != s.numHT {
		return false
	}
	coreID := -1
	for _, cpu := range cset.ToSlice() {
		id := s.sys.CPU(sysfs.ID(cpu)).CoreID()
		switch {
		case coreID < 0:
			coreID = int(id)
		case coreID != int(id):
			return false
		}
	}

	return true
}

// allocateCPUs allocates the requested number of CPUs.
func (s *static) allocateCPUs(numCPUs int, containerID string) (cpuset.CPUSet, error) {
	var result cpuset.CPUSet
	var err error

	s.Info("[cpumanager] allocateCpus: (numCPUs: %d)", numCPUs)

	if try, prefer := s.cpuPreference(containerID, numCPUs); !try {
		result, err = s.allocateOrdinaryCPUs(numCPUs)
	} else {
		result, err = s.allocateIsolatedCPUs(numCPUs, prefer)
	}

	if err != nil {
		return result, err
	}

	// Remove allocated CPUs from the shared and/or isolated CPUSet.
	s.SetDefaultCPUSet(s.GetDefaultCPUSet().Difference(result))
	s.isolatedCpus = s.isolatedCpus.Difference(result)

	s.Info("[cpumanager] allocateCPUs: returning \"%v\"", result)
	return result, nil
}

func (s *static) guaranteedCPUs(pod cache.Pod, container cache.Container) int {
	qos := pod.GetQOSClass()

	s.Debug("* QoS class for pod %s (%s) is %s", pod.GetID(), pod.GetName(), qos)

	if qos != v1.PodQOSGuaranteed {
		return 0
	}
	cpuQuantity := container.GetResourceRequirements().Requests[v1.ResourceCPU]
	if cpuQuantity.Value()*1000 != cpuQuantity.MilliValue() {
		return 0
	}
	// Safe downcast to do for all systems with < 2.1 billion CPUs.
	// Per the language spec, `int` is guaranteed to be at least 32 bits wide.
	// https://golang.org/ref/spec#Numeric_types
	return int(cpuQuantity.Value())
}

// Check our allocations constraints.
func (s *static) checkConstraints() error {
	online := s.sys.CPUSet().Difference(s.sys.Offlined())
	isolated := s.sys.Isolated().Intersection(online)
	online = online.Difference(isolated)

	cpus, ok := s.available[policy.DomainCPU]
	if !ok {
		s.availableCpus = online
	} else {
		switch cpus.(type) {
		case cpuset.CPUSet:
			s.availableCpus = cpus.(cpuset.CPUSet).Intersection(online)
		default:
			return policyError("invalid type for available CPU set: %T", cpus)
		}
	}

	s.isolatedCpus = isolated
	s.Info("system isolated CPUs: %s", s.isolatedCpus)

	return nil
}

// Allocate the requested reserved cpus.
func (s *static) allocateReserved() error {
	var err error
	var reserved cpuset.CPUSet

	cpus, ok := s.reserved[policy.DomainCPU]
	if !ok {
		return policyError("static policy cannot start without reserved CPUs")
	}

	switch cpus.(type) {
	case cpuset.CPUSet:
		reserved = cpus.(cpuset.CPUSet)
		if !reserved.Intersection(s.availableCpus).Equals(reserved) {
			return policyError("some reserved CPUs (%s) are unavailable",
				reserved.Difference(s.availableCpus).String())
		}
	case resource.Quantity:
		qty := cpus.(resource.Quantity)
		count := (int(qty.MilliValue()) + 999) / 1000
		from := s.availableCpus.Clone()
		if reserved, err = takeByTopology(from, count); err != nil {
			return policyError("failed to reserve %d CPUs: %v", cpus.(int), err)
		}
	}

	s.reservedCpus = reserved

	return nil
}

// Validate the cache/state supplied for starting.
func (s *static) validateState(state cache.Cache) error {
	s.state = state

	tmpAssignments := s.GetCPUAssignments()
	tmpDefaultCPUset := s.GetDefaultCPUSet()
	allCPUs := s.availableCpus.Clone()
	isolated := s.isolatedCpus.Clone()

	// Default cpuset cannot be empty when assignments exist
	if tmpDefaultCPUset.IsEmpty() {
		if len(tmpAssignments) != 0 {
			return fmt.Errorf("default cpuset cannot be empty")
		}

		// state is empty initialize
		s.SetDefaultCPUSet(allCPUs)

		return nil
	}

	// State has already been initialized from file (is not empty)
	// 1. Check if the reserved cpuset is not part of default cpuset because:
	// - kube/system reserved have changed (increased) - may lead to some containers not being able to start
	// - user tampered with file
	if !s.reservedCpus.Intersection(tmpDefaultCPUset).Equals(s.reservedCpus) {
		return fmt.Errorf("not all reserved cpus: \"%s\" are present in defaultCpuSet: \"%s\"",
			s.reservedCpus.String(), tmpDefaultCPUset.String())
	}

	// 2. Check if state for static policy is consistent
	for cID, cset := range tmpAssignments {
		// None of the cpu in DEFAULT cset should be in s.assignments
		if !tmpDefaultCPUset.Intersection(cset).IsEmpty() {
			return fmt.Errorf("container id: %s cpuset: \"%s\" overlaps with default cpuset \"%s\"",
				cID, cset.String(), tmpDefaultCPUset.String())
		}

		// Remove any potentially taken isolated CPUs from the available isolated set.
		s.isolatedCpus = s.isolatedCpus.Difference(cset)
	}

	s.Info("available (unallocated) isolated CPUs: %s", s.isolatedCpus)

	// 3. It's possible that the set of available CPUs has changed since
	// the state was written. This can be due to for example
	// offlining a CPU when kubelet is not running. If this happens,
	// CPU manager will run into trouble when later it tries to
	// assign non-existent CPUs to containers. Validate that the
	// topology that was received during CPU manager startup matches with
	// the set of CPUs stored in the state.
	totalKnownCPUs := tmpDefaultCPUset.Clone()

	for _, cset := range tmpAssignments {
		totalKnownCPUs = totalKnownCPUs.Union(cset)
	}
	if !totalKnownCPUs.Equals(allCPUs) {
		if totalKnownCPUs.IsSubsetOf(allCPUs.Union(isolated)) {
			return nil
		}
		return fmt.Errorf("current available CPUs \"%s\" are not a superset of CPUs in state \"%s\"",
			allCPUs.Union(isolated).String(), totalKnownCPUs.String())
	}

	return nil
}

// Topology-aware-like allocation wrapper.
func takeByTopology(available cpuset.CPUSet, numCPUs int) (cpuset.CPUSet, error) {
	from := &available
	cset, err := cpuallocator.AllocateCpus(from, numCPUs)
	if err != nil {
		return cset, err
	}

	return cset, err
}

// Validate static assignments, purge stale ones.
func (s *static) validateAssignments() {
	// Instead of relying/waiting for an external reconcilation loop to
	// clean up stale container/assignments, we do it ourselves upon startup.

	ca := s.GetCPUAssignments()
	for id, cset := range ca {
		if _, ok := s.state.LookupContainer(id); !ok {
			s.Info("Removing stale assignment of container %s (cpus %s)",
				id, cset.String())
			s.RemoveContainer(id)
		}
	}
}

// policyError creates a policy-specific formatted error
func policyError(format string, args ...interface{}) error {
	return fmt.Errorf(PolicyName+": "+format, args...)
}

//
// Kubelet CPU Manager / policy_static.go adaptation
//
// A set of rudimentary functions to get policy_static.go up and running
// with small enough changes that the code (above) remains recognisable
// for those who are already familiar with the original. These functions
// basically implements a CPU Manager state-like interface on top of our
// cache.

// ContainerCPUAssignments assigns CPU sets per container id.
type ContainerCPUAssignments map[string]cpuset.CPUSet

//
// Cache keys for storing the default cpuset (one for containers
// without exclusive allocations) and static assignments (cpusets
// for containers with exclusive allocations).

const (
	keyAssignments = "CPUAssignments"
	keyDefaultCPUs = "DefaultCPUSet"
)

// GetCPUAssignments gets the current CPU assignments from our state.
func (s *static) GetCPUAssignments() ContainerCPUAssignments {
	var ca map[string]cpuset.CPUSet

	if !s.state.GetPolicyEntry(keyAssignments, &ca) {
		s.Error("no cached CPU assignments")
	}

	if ca == nil {
		ca = make(map[string]cpuset.CPUSet)
		s.state.SetPolicyEntry(keyAssignments, ca)
	}

	return ca
}

// SetCPUAssginments sets the current CPU assignments in our state.
func (s *static) SetCPUAssignments(ca ContainerCPUAssignments) {
	s.state.SetPolicyEntry(keyAssignments, map[string]cpuset.CPUSet(ca))
}

// GetDefaultCPUSet gets the current default CPUSet from our state.
func (s *static) GetDefaultCPUSet() cpuset.CPUSet {
	var cset cpuset.CPUSet

	if !s.state.GetPolicyEntry(keyDefaultCPUs, &cset) {
		s.Error("no cached default CPU set")
	}

	return cset
}

// SetDefaultCPUSet sets the current default CPUSet in our state.
func (s *static) SetDefaultCPUSet(cset cpuset.CPUSet) {
	s.state.SetPolicyEntry(keyDefaultCPUs, cset)

	// update cpuset for containers with default assignment
	ca := s.GetCPUAssignments()
	for _, id := range s.state.GetContainerCacheIds() {
		if _, ok := ca[id]; !ok {
			s.SetCpusetCpus(id, cset.String())
		}
	}
}

// GetCPUSet gets the CPUSet for a container from our state.
func (s *static) GetCPUSet(containerID string) (cpuset.CPUSet, bool) {
	ca := s.GetCPUAssignments()
	cset, ok := ca[containerID]

	return cset.Clone(), ok
}

// SetCPUSet sets the CPUSet for a container in our state.
func (s *static) SetCPUSet(containerID string, cset cpuset.CPUSet) {
	ca := s.GetCPUAssignments()
	ca[containerID] = cset

	s.SetCPUAssignments(ca)
	s.SetCpusetCpus(containerID, cset.String())
}

// Delete deletes the given container from our state.
func (s *static) Delete(containerID string) {
	s.Debug("deleting container %s from assignments", containerID)

	ca := s.GetCPUAssignments()
	delete(ca, containerID)

	s.SetCPUAssignments(ca)
}

// SetCPUSetCpus updates cpuset.cpus for a container.
func (s *static) SetCpusetCpus(id, value string) error {
	c, ok := s.state.LookupContainer(id)
	if !ok {
		return policyError("can't find container '%s'", id)
	}

	c.SetCpusetCpus(value)
	s.Info("container %s: CpusetCpus set to %s", c.PrettyName(), value)

	return nil
}

//
// Automatically register us as a policy implementation.
//

// Implementation is the implementation we register with the policy module.
type Implementation func(*policy.BackendOptions) policy.Backend

// Name returns the name of this policy implementation.
func (i Implementation) Name() string {
	return PolicyName
}

// Description returns the desccription of this policy implementation.
func (i Implementation) Description() string {
	return PolicyDescription
}

// CreateFn returns the functions used to instantiate this policy.
func (i Implementation) CreateFn() policy.CreateFn {
	return policy.CreateFn(i)
}

var _ policy.Implementation = Implementation(nil)

func init() {
	policy.Register(Implementation(NewStaticPolicy))
}
