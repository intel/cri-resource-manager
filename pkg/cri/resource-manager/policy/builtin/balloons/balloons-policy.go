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

package balloons

import (
	"fmt"
	"path/filepath"

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
	PolicyName = "balloons"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "Flexible pools with per-pool CPU parameters"
	// PolicyPath is the path of this policy in the configuration hierarchy.
	PolicyPath = "policy." + PolicyName
	// balloonKey is a pod annotation key, the value is a pod balloon name.
	balloonKey = "balloon." + PolicyName + "." + kubernetes.ResmgrKeyNamespace
	// reservedBalloonDefName is the name in the reserved balloon definition.
	reservedBalloonDefName = "reserved"
	// defaultBalloonDefName is the name in the default balloon definition.
	defaultBalloonDefName = "default"
)

// balloons contains configuration and runtime attributes of the balloons policy
type balloons struct {
	options   *policyapi.BackendOptions // configuration common to all policies
	bpoptions BalloonsOptions           // balloons-specific configuration
	cch       cache.Cache               // cri-resmgr cache
	allowed   cpuset.CPUSet             // bounding set of CPUs we're allowed to use
	reserved  cpuset.CPUSet             // system-/kube-reserved CPUs
	freeCpus  cpuset.CPUSet             // CPUs to be included in growing or new ballons

	reservedBalloonDef *BalloonDef // built-in definition of the reserved balloon
	defaultBalloonDef  *BalloonDef // built-in definition of the default balloon
	balloons           []*Balloon  // balloon instances: reserved, default and user-defined

	cpuAllocator cpuallocator.CPUAllocator // CPU allocator used by the policy
}

// Balloon contains attributes of a balloon instance
type Balloon struct {
	// Def is the definition from which this balloon instance is created.
	Def *BalloonDef
	// Instance is the index of this balloon instance, starting from
	// zero for every balloon definition.
	Instance int
	// Cpus is the set of CPUs exclusive to this balloon instance only.
	Cpus cpuset.CPUSet
	// Mems is the set of memory nodes with minimal access delay
	// from CPUs.
	Mems idset.IDSet
	// PodIDs maps pod ID to list of container IDs.
	// - len(PodIDs) is the number of pods in the balloon.
	// - len(PodIDs[podID]) is the number of containers of podID
	//   currently assigned to the balloon.
	PodIDs map[string][]string
}

var log logger.Logger = logger.NewLogger("policy")

// String is a stringer for a balloon.
func (bln Balloon) String() string {
	return fmt.Sprintf("%s{Cpus:%s, Mems:%s}", bln.PrettyName(), bln.Cpus, bln.Mems)
}

// PrettyName returns a unique name for a balloon.
func (bln Balloon) PrettyName() string {
	return fmt.Sprintf("%s[%d]", bln.Def.Name, bln.Instance)
}

// ContainerIDs returns IDs of containers assigned in a balloon.
// (Using cache.Container.GetCacheID()'s)
func (bln Balloon) ContainerIDs() []string {
	cIDs := []string{}
	for _, ctrIDs := range bln.PodIDs {
		cIDs = append(cIDs, ctrIDs...)
	}
	return cIDs
}

// ContainerCount returns the number of containers in a balloon.
func (bln Balloon) ContainerCount() int {
	count := 0
	for _, ctrIDs := range bln.PodIDs {
		count += len(ctrIDs)
	}
	return count
}

func (bln Balloon) AvailMilliCpus() int {
	return bln.Cpus.Size() * 1000
}

func (bln Balloon) MaxAvailMilliCpus() int {
	return bln.Def.MaxCpus * 1000
}

// CreateBalloonsPolicy creates a new policy instance.
func CreateBalloonsPolicy(policyOptions *policy.BackendOptions) policy.Backend {
	p := &balloons{
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
	if err := p.setConfig(balloonsOptions); err != nil {
		log.Fatal("failed to create %s policy: %v", PolicyName, err)
	}

	pkgcfg.GetModule(PolicyPath).AddNotify(p.configNotify)

	return p
}

// Name returns the name of this policy.
func (p *balloons) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (p *balloons) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (p *balloons) Start(add []cache.Container, del []cache.Container) error {
	log.Info("%s policy started", PolicyName)
	// reassign all containers
	return p.Sync(p.cch.GetContainers(), nil)
}

// Sync synchronizes the active policy state.
func (p *balloons) Sync(add []cache.Container, del []cache.Container) error {
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
func (p *balloons) AllocateResources(c cache.Container) error {
	log.Debug("allocating resources for container %s...", c.PrettyName())
	bln, err := p.allocateBalloon(c)
	if err != nil {
		return balloonsError("balloon allocation for container %s failed: %w", c.PrettyName(), err)
	}
	if bln == nil {
		return balloonsError("no suitable balloons found for container %s", c.PrettyName())
	}
	// Resize selected balloon to fit the new container, unless it
	// uses the ReservedResources CPUs, which is a fixed set.
	reqMilliCpus := p.containerRequestedMilliCpus(c.GetCacheID()) + p.requestedMilliCpus(bln)
	// Even if all containers in a balloon request is 0 mCPU in
	// total (all are BestEffort, for example), force the size of
	// the balloon to be enough for at least 1 mCPU
	// request. Otherwise balloon's cpuset becomes empty, which in
	// would mean no CPU pinning and balloon's containers would
	// run on any CPUs.
	if bln.AvailMilliCpus() < max(1, reqMilliCpus) {
		p.resizeBalloon(bln, max(1, reqMilliCpus))
	}
	p.assignContainer(c, bln)
	if log.DebugEnabled() {
		log.Debug(p.dumpBalloon(bln))
	}
	return nil
}

// ReleaseResources is a resource release request for this policy.
func (p *balloons) ReleaseResources(c cache.Container) error {
	log.Debug("releasing container %s...", c.PrettyName())
	if bln := p.balloonByContainer(c); bln != nil {
		p.dismissContainer(c, bln)
		if log.DebugEnabled() {
			log.Debug(p.dumpBalloon(bln))
		}
		if bln.ContainerCount() == 0 {
			// Deflate the balloon completely before
			// freeing it.
			p.resizeBalloon(bln, 0)
			log.Debug("all containers removed, free balloon allocation %s", bln.PrettyName())
			p.freeBalloon(bln)
		} else {
			// Make sure that the balloon will have at
			// least 1 CPU to run remaining containers.
			p.resizeBalloon(bln, max(1, p.requestedMilliCpus(bln)))
		}
	} else {
		log.Debug("ReleaseResources: balloon-less container %s, nothing to release", c.PrettyName())
	}
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (p *balloons) UpdateResources(c cache.Container) error {
	log.Debug("(not) updating container %s...", c.PrettyName())
	return nil
}

// Rebalance tries to find an optimal allocation of resources for the current containers.
func (p *balloons) Rebalance() (bool, error) {
	log.Debug("(not) rebalancing containers...")
	return false, nil
}

// HandleEvent handles policy-specific events.
func (p *balloons) HandleEvent(*events.Policy) (bool, error) {
	log.Debug("(not) handling event...")
	return false, nil
}

// ExportResourceData provides resource data to export for the container.
func (p *balloons) ExportResourceData(c cache.Container) map[string]string {
	return nil
}

// Introspect provides data for external introspection.
func (p *balloons) Introspect(*introspect.State) {
	return
}

// balloonByContainer returns a balloon that contains a container.
func (p *balloons) balloonByContainer(c cache.Container) *Balloon {
	podID := c.GetPodID()
	cID := c.GetCacheID()
	for _, bln := range p.balloons {
		for _, ctrID := range bln.PodIDs[podID] {
			if ctrID == cID {
				return bln
			}
		}
	}
	return nil
}

// balloonsByNamespace returns balloons that contain containers in a
// namespace.
func (p *balloons) balloonsByNamespace(namespace string) []*Balloon {
	blns := []*Balloon{}
	for _, bln := range p.balloons {
		for podID, ctrIDs := range bln.PodIDs {
			if len(ctrIDs) == 0 {
				continue
			}
			pod, ok := p.cch.LookupPod(podID)
			if !ok {
				continue
			}
			if pod.GetNamespace() == namespace {
				blns = append(blns, bln)
				break
			}
		}
	}
	return blns
}

// balloonsByPod returns balloons that contain any container of a pod.
func (p *balloons) balloonsByPod(pod cache.Pod) []*Balloon {
	podID := pod.GetID()
	blns := []*Balloon{}
	for _, bln := range p.balloons {
		if _, ok := bln.PodIDs[podID]; ok {
			blns = append(blns, bln)
		}
	}
	return blns
}

// balloonsByDef returns list of balloons instantiated from a balloon
// definition.
func (p *balloons) balloonsByDef(blnDef *BalloonDef) []*Balloon {
	balloons := []*Balloon{}
	for _, balloon := range p.balloons {
		if balloon.Def == blnDef {
			balloons = append(balloons, balloon)
		}
	}
	return balloons
}

// balloonDefByName returns a balloon definition with a name.
func (p *balloons) balloonDefByName(defName string) *BalloonDef {
	if defName == "reserved" {
		return p.reservedBalloonDef
	}
	if defName == "default" {
		return p.defaultBalloonDef
	}
	for _, blnDef := range p.bpoptions.BalloonDefs {
		if blnDef.Name == defName {
			return blnDef
		}
	}
	return nil
}

func (p *balloons) chooseBalloonDef(c cache.Container) (*BalloonDef, error) {
	var blnDef *BalloonDef
	// BalloonDef is defined by annotation?
	if blnDefName, ok := c.GetEffectiveAnnotation(balloonKey); ok {
		blnDef = p.balloonDefByName(blnDefName)
		if blnDef == nil {
			return nil, balloonsError("no balloon for annotation %q", blnDefName)
		}
		return blnDef, nil
	}

	// BalloonDef is defined by a special namespace (kube-system +
	// ReservedPoolNamespaces)?
	if namespaceMatches(c.GetNamespace(), append(p.bpoptions.ReservedPoolNamespaces, metav1.NamespaceSystem)) {
		return p.balloons[0].Def, nil
	}

	// BalloonDef is defined by the namespace.
	for _, blnDef := range append([]*BalloonDef{p.reservedBalloonDef, p.defaultBalloonDef}, p.bpoptions.BalloonDefs...) {
		if namespaceMatches(c.GetNamespace(), blnDef.Namespaces) {
			return blnDef, nil
		}
	}

	// Fallback to the default balloon.
	return p.defaultBalloonDef, nil
}

func (p *balloons) containerRequestedMilliCpus(contID string) int {
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

// requestedMilliCpus sums up and returns CPU requests of all
// containers assigned to a balloon.
func (p *balloons) requestedMilliCpus(bln *Balloon) int {
	cpuRequested := 0
	for _, cID := range bln.ContainerIDs() {
		cpuRequested += p.containerRequestedMilliCpus(cID)
	}
	return cpuRequested
}

// freeMilliCpus returns free CPU resources in a balloon without
// inflating the balloon.
func (p *balloons) freeMilliCpus(bln *Balloon) int {
	return bln.AvailMilliCpus() - p.requestedMilliCpus(bln)
}

// maxFreeMilliCpus returns free CPU resources in a balloon when it is
// inflated as large as possible.
func (p *balloons) maxFreeMilliCpus(bln *Balloon) int {
	return bln.MaxAvailMilliCpus() - p.requestedMilliCpus(bln)
}

// largest helps finding the largest element and value in a slice.
// Input the length of a slice and a function that returns the
// magnitude of given element in the slice as int.
func largest(sliceLen int, valueOf func(i int) int) (int, int) {
	largestIndex := -1
	largestValue := 0
	for index := 0; index < sliceLen; index++ {
		value := valueOf(index)
		if largestIndex == -1 || value > largestValue {
			largestIndex = index
			largestValue = value
		}
	}
	return largestIndex, largestValue
}

// resetCpuClass resets CPU configurations globally. All balloons can
// be ignored, their CPU configurations will be applied later.
func (p *balloons) resetCpuClass() error {
	// Usual inputs:
	// - p.allowed (cpuset.CPUset): all CPUs available for this
	//   policy.
	// - p.IdleCpuClass (string): CPU class for allowed CPUs.
	//
	// Other inputs, if needed:
	// - p.reserved (cpuset.CPUset): CPUs of ReservedResources
	//   (typically for kube-system containers).
	//
	// Note: p.useCpuClass(balloon) will be called before assigning
	// containers on the balloon, including the reserved balloon.
	//
	// TODO: don't depend on cpu controller directly
	cpucontrol.Assign(p.cch, p.bpoptions.IdleCpuClass, p.allowed.ToSliceNoSort()...)
	log.Debugf("resetCpuClass available: %s; reserved: %s", p.allowed, p.reserved)
	return nil
}

// useCpuClass configures CPUs of a balloon.
func (p *balloons) useCpuClass(bln *Balloon) error {
	// Usual inputs:
	// - CPUs that cpuallocator has reserved for this balloon:
	//   bln.Cpus (cpuset.CPUSet).
	// - User-defined CPU configuration for CPUs of balloon of this type:
	//   bln.Def.CpuClass (string).
	// - Current configuration(?): feel free to add data
	//   structure for this. For instance policy-global p.cpuConfs,
	//   or balloon-local bln.cpuConfs.
	//
	// Other input examples, if needed:
	// - Requested CPU resources by all containers in the balloon:
	//   p.requestedMilliCpus(bln).
	// - Free CPU resources in the balloon: p.freeMilliCpus(bln).
	// - Number of assigned containers: bln.ContainerCount().
	// - Container details: access p.cch with bln.ContainerIDs().
	// - User-defined CPU AllocatorPriority: bln.Def.AllocatorPriority.
	// - All existing balloon instances: p.balloons.
	// - CPU configurations by user: bln.Def.CpuClass (for bln in p.balloons)
	cpucontrol.Assign(p.cch, bln.Def.CpuClass, bln.Cpus.ToSliceNoSort()...)
	log.Debugf("useCpuClass Cpus: %s; CpuClass: %s", bln.Cpus, bln.Def.CpuClass)
	return nil
}

// forgetCpuClass is called when CPUs of a balloon are released from duty.
func (p *balloons) forgetCpuClass(bln *Balloon) {
	// Use p.IdleCpuClass for bln.Cpus.
	// Usual inputs: see useCpuClass
	cpucontrol.Assign(p.cch, p.bpoptions.IdleCpuClass, bln.Cpus.ToSliceNoSort()...)
	log.Debugf("forgetCpuClass Cpus: %s; CpuClass: %s", bln.Cpus, bln.Def.CpuClass)
}

func (p *balloons) newBalloon(blnDef *BalloonDef, confCpus bool) (*Balloon, error) {
	var cpus cpuset.CPUSet
	var err error
	blnsOfDef := p.balloonsByDef(blnDef)
	// Allowed to create new balloon instance from blnDef?
	if blnDef.MaxBalloons > 0 && blnDef.MaxBalloons <= len(blnsOfDef) {
		return nil, balloonsError("cannot create new %q balloon, MaxBalloons limit (%d) reached", blnDef.Name, blnDef.MaxBalloons)
	}
	// Find the first unused balloon instance index.
	freeInstance := 0
	for freeInstance = 0; freeInstance < len(blnsOfDef); freeInstance++ {
		isFree := true
		for _, bln := range blnsOfDef {
			if bln.Instance == freeInstance {
				isFree = false
				break
			}
		}
		if isFree {
			break
		}
	}
	// Allocate CPUs
	if blnDef == p.reservedBalloonDef ||
		(blnDef == p.defaultBalloonDef && blnDef.MinCpus == 0 && blnDef.MaxCpus == 0) {
		// The reserved balloon uses ReservedResources CPUs.
		// So does the default balloon unless its CPU counts are tweaked.
		cpus = p.reserved
	} else {
		cpus, err = p.cpuAllocator.AllocateCpus(&p.freeCpus, blnDef.MinCpus, blnDef.AllocatorPriority)
		if err != nil {
			return nil, balloonsError("could not allocate %d MinCpus for balloon %s[%d]: %w", blnDef.MinCpus, blnDef.Name, freeInstance, err)
		}
	}
	bln := &Balloon{
		Def:      blnDef,
		Instance: freeInstance,
		PodIDs:   make(map[string][]string),
		Cpus:     cpus,
		Mems:     p.closestMems(cpus),
	}
	if confCpus {
		if err = p.useCpuClass(bln); err != nil {
			log.Errorf("failed to apply CPU configuration to new balloon %s[%d] (cpus: %s): %w", blnDef.Name, freeInstance, cpus, err)
			return nil, err
		}
	}
	return bln, nil
}

// deleteBalloon removes an empty balloon.
func (p *balloons) deleteBalloon(bln *Balloon) {
	log.Debugf("deleting balloon %s", bln)
	remainingBalloons := []*Balloon{}
	for _, b := range p.balloons {
		if b != bln {
			remainingBalloons = append(remainingBalloons, b)
		}
	}
	p.balloons = remainingBalloons
	p.forgetCpuClass(bln)
	p.freeCpus = p.freeCpus.Union(bln.Cpus)
	p.cpuAllocator.ReleaseCpus(&bln.Cpus, bln.Cpus.Size(), bln.Def.AllocatorPriority)
}

// freeBalloon clears a balloon and deletes it if allowed.
func (p *balloons) freeBalloon(bln *Balloon) {
	bln.PodIDs = make(map[string][]string)
	blnsSameDef := p.balloonsByDef(bln.Def)
	if len(blnsSameDef) > bln.Def.MinBalloons {
		p.deleteBalloon(bln)
	}
}

func (p *balloons) chooseBalloonInstance(blnDef *BalloonDef, fm FillMethod, c cache.Container) (*Balloon, error) {
	// If assigning to the reserved or the default balloon, fill
	// method is ignored: always fill the chosen balloon.
	if blnDef == p.balloons[0].Def {
		return p.balloons[0], nil
	}
	if blnDef == p.balloons[1].Def {
		return p.balloons[1], nil
	}
	reqMilliCpus := p.containerRequestedMilliCpus(c.GetCacheID())
	// Handle fill methods that do not use existing instances of
	// balloonDef.
	switch fm {
	case FillReservedBalloon:
		return p.balloons[0], nil
	case FillDefaultBalloon:
		return p.balloons[1], nil
	case FillNewBalloon, FillNewBalloonMust:
		// Choosing an existing balloon without containers is
		// preferred over instantiating a new balloon.
		for _, bln := range p.balloonsByDef(blnDef) {
			if len(bln.PodIDs) == 0 {
				return bln, nil
			}
		}
		if newBln, err := p.newBalloon(blnDef, true); err == nil {
			p.balloons = append(p.balloons, newBln)
			return newBln, nil
		} else {
			if fm == FillNewBalloonMust {
				return nil, err
			}
			return nil, nil
		}
	case FillSameNamespace:
		for _, bln := range p.balloonsByNamespace(c.GetNamespace()) {
			if bln.Def == blnDef && p.maxFreeMilliCpus(bln) >= reqMilliCpus {
				return bln, nil
			}
		}
		return nil, nil
	case FillSamePod:
		if pod, ok := c.GetPod(); ok {
			for _, bln := range p.balloonsByPod(pod) {
				if p.maxFreeMilliCpus(bln) >= reqMilliCpus {
					return bln, nil
				}
			}
			return nil, nil
		} else {
			return nil, balloonsError("fill method %s failed: cannot find pod for container %s", fm, c.PrettyName())
		}
	}
	// Handle fill methods that need existing instances of
	// balloonDef, and fail if there are no instances.
	balloons := p.balloonsByDef(blnDef)
	if len(balloons) == 0 {
		return nil, nil
	}
	switch fm {
	case FillBalanced:
		// Are there balloons where the container would fit
		// without inflating the balloon?
		blnIdx, freeMilliCpus := largest(len(balloons), func(i int) int {
			return p.freeMilliCpus(balloons[i])
		})
		if freeMilliCpus >= reqMilliCpus {
			return balloons[blnIdx], nil
		}
	case FillBalancedInflate:
		// Are there balloons where the container would fit
		// after inflating the balloon?
		blnIdx, maxFreeMilliCpus := largest(len(balloons), func(i int) int {
			return p.maxFreeMilliCpus(balloons[i])
		})
		if maxFreeMilliCpus >= reqMilliCpus {
			return balloons[blnIdx], nil
		}
	default:
		return nil, balloonsError("balloon type fill method not implemented: %s", fm)
	}
	// No error, but balloon type remains undecided in this assign method.
	return nil, nil
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

// allocateBalloon returns a balloon allocated for a container.
func (p *balloons) allocateBalloon(c cache.Container) (*Balloon, error) {
	blnDef, err := p.chooseBalloonDef(c)
	if err != nil {
		return nil, err
	}
	if blnDef == nil {
		return nil, balloonsError("no applicable balloon type found")
	}

	bln, err := p.allocateBalloonOfDef(blnDef, c)
	if err != nil {
		return nil, err
	}
	if bln == nil {
		return nil, balloonsError("no suitable balloon instance available")
	}
	return bln, nil
}

// allocateBalloonOfDef returns a balloon instantiated from a
// definition for a container.
func (p *balloons) allocateBalloonOfDef(blnDef *BalloonDef, c cache.Container) (*Balloon, error) {
	if blnDef == p.reservedBalloonDef {
		return p.balloons[0], nil
	}
	if blnDef == p.defaultBalloonDef {
		return p.balloons[1], nil
	}

	fillChain := []FillMethod{}
	if !blnDef.PreferSpreadingPods {
		fillChain = append(fillChain, FillSamePod)
	}
	if blnDef.PreferPerNamespaceBalloon {
		fillChain = append(fillChain, FillSameNamespace, FillNewBalloon)
	}
	if blnDef.PreferNewBalloons {
		fillChain = append(fillChain, FillNewBalloon, FillBalanced, FillBalancedInflate)
	} else {
		fillChain = append(fillChain, FillBalanced, FillBalancedInflate, FillNewBalloon)
	}
	for _, fillMethod := range fillChain {
		bln, err := p.chooseBalloonInstance(blnDef, fillMethod, c)
		if err != nil {
			log.Debugf("fill method %q prevents allocation: %w", fillMethod, err)
			return nil, err
		}
		if bln == nil {
			log.Debugf("fill method %q not applicable", fillMethod)
			continue
		}
		log.Debugf("fill method %q suggests balloon instance %v", fillMethod, bln)
		return bln, nil
	}
	return nil, nil
}

// dumpBalloon dumps balloon contents in detail.
func (p *balloons) dumpBalloon(bln *Balloon) string {
	conts := []string{}
	pods := []string{}
	for podID, contIDs := range bln.PodIDs {
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
	s := fmt.Sprintf("Balloon %s{Cpus: %s; Mems: %s; mCPU used: %d; capacity: %d; max. capacity: %d; pods: %s; conts: %s}",
		bln.PrettyName(),
		bln.Cpus,
		bln.Mems,
		p.requestedMilliCpus(bln),
		bln.AvailMilliCpus(),
		bln.MaxAvailMilliCpus(),
		pods,
		conts)
	return s
}

// getPodMilliCPU returns mCPUs requested by podID.
func (p *balloons) getPodMilliCPU(podID string) int64 {
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

// changesBalloons returns true if two balloons policy configurations
// may lead into different balloon instances or workload assignment.
func changesBalloons(opts0, opts1 *BalloonsOptions) bool {
	if opts0 == nil && opts1 == nil {
		return false
	}
	if opts0 == nil || opts1 == nil {
		return true
	}
	if len(opts0.BalloonDefs) != len(opts1.BalloonDefs) {
		return true
	}
	o0 := opts0.DeepCopy()
	o1 := opts1.DeepCopy()
	// Ignore differences in CPU class names. Every other change
	// potentially changes balloons or workloads.
	o0.IdleCpuClass = ""
	o1.IdleCpuClass = ""
	for i := range o0.BalloonDefs {
		o0.BalloonDefs[i].CpuClass = ""
		o1.BalloonDefs[i].CpuClass = ""
	}
	return utils.DumpJSON(o0) != utils.DumpJSON(o1)
}

// changesCpuClasses returns true if two balloons policy
// configurations can lead to using different CPU classes on
// corresponding balloon instances. Calling changesCpuClasses(o0, o1)
// makes sense only if changesBalloons(o0, o1) has returned false.
func changesCpuClasses(opts0, opts1 *BalloonsOptions) bool {
	if opts0 == nil && opts1 == nil {
		return false
	}
	if opts0 == nil || opts1 == nil {
		return true
	}
	if opts0.IdleCpuClass != opts1.IdleCpuClass {
		return true
	}
	if len(opts0.BalloonDefs) != len(opts1.BalloonDefs) {
		return true
	}
	for i := range opts0.BalloonDefs {
		if opts0.BalloonDefs[i].CpuClass != opts1.BalloonDefs[i].CpuClass {
			return true
		}
	}
	return false
}

// configNotify applies new configuration.
func (p *balloons) configNotify(event pkgcfg.Event, source pkgcfg.Source) error {
	log.Info("configuration %s", event)
	defer log.Debug("effective configuration:\n%s\n", utils.DumpJSON(p.bpoptions))
	newBalloonsOptions := balloonsOptions.DeepCopy()
	if !changesBalloons(&p.bpoptions, newBalloonsOptions) {
		if !changesCpuClasses(&p.bpoptions, newBalloonsOptions) {
			log.Info("no configuration changes")
		} else {
			log.Info("configuration changes only on CPU classes")
			// Update new CPU classes to existing balloon
			// definitions. The same BalloonDef instances
			// must be kept in use, because each Balloon
			// instance holds a direct reference to its
			// BalloonDef.
			for i := range p.bpoptions.BalloonDefs {
				p.bpoptions.BalloonDefs[i].CpuClass = newBalloonsOptions.BalloonDefs[i].CpuClass
			}
			// (Re)configures all CPUs in balloons.
			p.resetCpuClass()
			for _, bln := range p.balloons {
				p.useCpuClass(bln)
			}
		}
		return nil
	}
	if err := p.setConfig(newBalloonsOptions); err != nil {
		log.Error("config update failed: %v", err)
		return err
	}
	log.Info("config updated successfully")
	p.Sync(p.cch.GetContainers(), p.cch.GetContainers())
	return nil
}

// applyBalloonDef creates user-defined balloons or reconfigures built-in
// balloons according to the blnDef. Does not initialize balloon CPUs.
func (p *balloons) applyBalloonDef(balloons *[]*Balloon, blnDef *BalloonDef, freeCpus *cpuset.CPUSet) error {
	if len(*balloons) < 2 {
		return balloonsError("internal error: reserved and default balloons missing, cannot apply balloon definitions")
	}
	reservedBalloon := (*balloons)[0]
	defaultBalloon := (*balloons)[1]
	// Every BalloonDef does one of the following:
	// 1. reconfigures the "reserved" balloon (most restricted)
	// 2. reconfigures the "default" balloon (somewhat restricted)
	// 3. defines new user-defined balloons.
	switch blnDef.Name {
	case "":
		// Case 0: bad name
		return balloonsError("undefined or empty balloon name")
	case reservedBalloon.Def.Name:
		// Case 1: reconfigure the "reserved" balloon.
		if blnDef.MinCpus != 0 {
			return balloonsError("cannot reconfigure the reserved balloon MinCpus, specified in ReservedResources CPUs")
		}
		if blnDef.MaxCpus != 0 {
			return balloonsError("cannot reconfigure the reserved balloon MaxCpus, specified in ReservedResources CPUs")
		}
		if blnDef.MinBalloons != 0 {
			return balloonsError("cannot reconfigure the reserved balloon MinBalloons")
		}
		p.reservedBalloonDef.AllocatorPriority = blnDef.AllocatorPriority
		p.reservedBalloonDef.CpuClass = blnDef.CpuClass
		p.reservedBalloonDef.Namespaces = blnDef.Namespaces
	case defaultBalloon.Def.Name:
		// Case 2: reconfigure the "default" balloon.
		defaultUsesReservedCpus := true
		if blnDef.MinCpus != 0 || blnDef.MaxCpus != 0 {
			defaultUsesReservedCpus = false
		}
		if blnDef.MinBalloons != 0 {
			return balloonsError("cannot reconfigure the default balloon MinBalloons")
		}
		p.defaultBalloonDef.MinCpus = blnDef.MinCpus
		p.defaultBalloonDef.MaxCpus = blnDef.MaxCpus
		p.defaultBalloonDef.AllocatorPriority = blnDef.AllocatorPriority
		p.defaultBalloonDef.CpuClass = blnDef.CpuClass
		p.defaultBalloonDef.Namespaces = blnDef.Namespaces
		if !defaultUsesReservedCpus {
			// Overwrite existing default balloon instance
			// that uses reserved CPUs with a balloon that
			// uses its own CPUs.
			newDefaultBln, err := p.newBalloon(p.defaultBalloonDef, false)
			if err != nil {
				return balloonsError("cannot create new default balloon: %w", err)
			}
			newDefaultBln.Instance = 0
			(*balloons)[1] = newDefaultBln
		}
	default:
		// Case 3: create minimum amount (MinBalloons) of each user-defined balloons.
		for allocPrio := cpuallocator.CPUPriority(0); allocPrio < cpuallocator.NumCPUPriorities; allocPrio++ {
			if blnDef.AllocatorPriority != allocPrio {
				continue
			}
			for blnIdx := 0; blnIdx < blnDef.MinBalloons; blnIdx++ {
				newBln, err := p.newBalloon(blnDef, false)
				if err != nil {
					return err
				}
				if newBln == nil {
					return balloonsError("failed to create balloon '%s[%d]' as required by MinBalloons=%d", blnDef.Name, blnIdx, blnDef.MinBalloons)
				}
				*balloons = append(*balloons, newBln)
			}
		}
	}
	return nil
}

// setConfig takes new balloon configuration into use.
func (p *balloons) setConfig(bpoptions *BalloonsOptions) error {
	// TODO: revert allocations (p.freeCpus) to old ones if the
	// configuration is invalid. Currently bad configuration
	// leaves a mess in bookkeeping.

	// Create the default reserved and default balloon
	// definitions. Some properties of these definitions may be
	// altered by user configuration.
	p.reservedBalloonDef = &BalloonDef{
		Name:              reservedBalloonDefName,
		MinBalloons:       1,
		AllocatorPriority: 3,
	}
	p.defaultBalloonDef = &BalloonDef{
		Name:              defaultBalloonDefName,
		MinBalloons:       1,
		AllocatorPriority: 3,
	}
	p.balloons = []*Balloon{}
	p.freeCpus = p.allowed.Clone()
	p.freeCpus = p.freeCpus.Difference(p.reserved)
	// Instantiate built-in reserved and default balloons.
	reservedBalloon, err := p.newBalloon(p.reservedBalloonDef, false)
	if err != nil {
		return err
	}
	p.balloons = append(p.balloons, reservedBalloon)
	defaultBalloon, err := p.newBalloon(p.defaultBalloonDef, false)
	if err != nil {
		return err
	}
	p.balloons = append(p.balloons, defaultBalloon)
	// First apply customizations to built-in balloons: "reserved"
	// and "default".
	for _, blnDef := range bpoptions.BalloonDefs {
		if blnDef.Name != reservedBalloonDefName && blnDef.Name != defaultBalloonDefName {
			continue
		}
		if err := p.applyBalloonDef(&p.balloons, blnDef, &p.freeCpus); err != nil {
			return err
		}
	}
	// Apply all user balloon definitions, skip already customized
	// "reserved" and "default" balloons.
	for _, blnDef := range bpoptions.BalloonDefs {
		if blnDef.Name == reservedBalloonDefName || blnDef.Name == defaultBalloonDefName {
			continue
		}
		if err := p.applyBalloonDef(&p.balloons, blnDef, &p.freeCpus); err != nil {
			return err
		}
	}
	// Finish balloon instance initialization.
	log.Info("%s policy balloons:", PolicyName)
	for blnIdx, bln := range p.balloons {
		log.Info("- balloon %d: %s", blnIdx, bln)
	}
	// No errors in balloon creation, take new configuration into use.
	p.bpoptions = *bpoptions
	// (Re)configures all CPUs in balloons.
	p.resetCpuClass()
	for _, bln := range p.balloons {
		p.useCpuClass(bln)
	}
	return nil
}

// closestMems returns memory node IDs good for pinning containers
// that run on given CPUs
func (p *balloons) closestMems(cpus cpuset.CPUSet) idset.IDSet {
	mems := idset.NewIDSet()
	sys := p.options.System
	for _, nodeID := range sys.NodeIDs() {
		if !cpus.Intersection(sys.Node(nodeID).CPUSet()).IsEmpty() {
			mems.Add(nodeID)
		}
	}
	return mems
}

// filterBalloons returns balloons for which the test function returns true
func filterBalloons(balloons []*Balloon, test func(*Balloon) bool) (ret []*Balloon) {
	for _, bln := range balloons {
		if test(bln) {
			ret = append(ret, bln)
		}
	}
	return
}

// availableMilliCPU returns mCPUs available in a balloon.
func (p *balloons) availableMilliCpus(balloon *Balloon) int64 {
	cpuAvail := int64(balloon.Cpus.Size() * 1000)
	cpuRequested := int64(0)
	for podID := range balloon.PodIDs {
		cpuRequested += p.getPodMilliCPU(podID)
	}
	return cpuAvail - cpuRequested
}

// resizeBalloon changes the CPUs allocated for a balloon, if allowed.
func (p *balloons) resizeBalloon(bln *Balloon, newMilliCpus int) error {
	if bln.Cpus.Equals(p.reserved) {
		log.Debugf("not resizing %s to %d mCPU, using fixed CPUs", bln, newMilliCpus)
		return nil
	}
	oldCpuCount := bln.Cpus.Size()
	newCpuCount := (newMilliCpus + 999) / 1000
	if bln.Def.MaxCpus > 0 && newCpuCount > bln.Def.MaxCpus {
		newCpuCount = bln.Def.MaxCpus
	}
	if bln.Def.MinCpus > 0 && newCpuCount < bln.Def.MinCpus {
		newCpuCount = bln.Def.MinCpus
	}
	log.Debugf("resize %s to capacity %d: CPUs from %d to %d. freecpus: %#s", bln, newMilliCpus, oldCpuCount, newCpuCount, p.freeCpus)
	if oldCpuCount == newCpuCount {
		return nil
	}
	p.forgetCpuClass(bln)
	defer p.useCpuClass(bln)
	if newCpuCount > oldCpuCount {
		oldCpus := bln.Cpus.Clone()
		keptCpus, err := p.cpuAllocator.ReleaseCpus(&oldCpus, oldCpuCount, bln.Def.AllocatorPriority)
		if err != nil || keptCpus.Size() != 0 {
			return balloonsError("resize/inflate: releasing %d CPUs from %s failed: %w (kept: %s)", oldCpuCount, bln, err, keptCpus)
		}
		p.freeCpus = p.freeCpus.Union(bln.Cpus)
		newCpus, err := p.cpuAllocator.AllocateCpus(&p.freeCpus, newCpuCount, bln.Def.AllocatorPriority)
		if err != nil {
			return balloonsError("resize/inflate: allocating %d CPUs for %s failed: %w", newCpuCount, bln, err)
		}
		bln.Cpus = newCpus
	} else {
		keptCpus, err := p.cpuAllocator.ReleaseCpus(&bln.Cpus, oldCpuCount-newCpuCount, bln.Def.AllocatorPriority)
		if err != nil || keptCpus.Size() != newCpuCount {
			return balloonsError("resize/deflate: releasing %d CPUs from %s failed: %w (kept: %s)", oldCpuCount-newCpuCount, bln, err, keptCpus)
		}
		log.Debugf("freeCpus: %s, bln.Cpus: %s, keptCpus: %s", p.freeCpus, bln.Cpus, keptCpus)
		p.freeCpus = p.freeCpus.Union(bln.Cpus)
		bln.Cpus = keptCpus
		log.Debugf("new freeCpus: %s, new bln.Cpus: %s", p.freeCpus, bln.Cpus)
	}
	log.Debugf("resize successful: %s, freecpus: %#s", bln, p.freeCpus)
	bln.Mems = p.closestMems(bln.Cpus)
	for _, cID := range bln.ContainerIDs() {
		if c, ok := p.cch.LookupContainer(cID); ok {
			p.pinCpuMem(c, bln.Cpus, bln.Mems)
		}
	}
	return nil
}

// assignContainer adds a container to a balloon
func (p *balloons) assignContainer(c cache.Container, bln *Balloon) {
	log.Info("assigning container %s to balloon %s", c.PrettyName(), bln)
	// TODO: inflate the balloon (add CPUs / reconfigure balloons)
	// if necessary
	podID := c.GetPodID()
	bln.PodIDs[podID] = append(bln.PodIDs[podID], c.GetCacheID())
	p.pinCpuMem(c, bln.Cpus, bln.Mems)
}

// dismissContainer removes a container from a balloon
func (p *balloons) dismissContainer(c cache.Container, bln *Balloon) {
	podID := c.GetPodID()
	bln.PodIDs[podID] = removeString(bln.PodIDs[podID], c.GetCacheID())
	if len(bln.PodIDs[podID]) == 0 {
		delete(bln.PodIDs, podID)
	}
}

// pinCpuMem pins container to CPUs and memory nodes if flagged
func (p *balloons) pinCpuMem(c cache.Container, cpus cpuset.CPUSet, mems idset.IDSet) {
	if p.bpoptions.PinCPU == nil || *p.bpoptions.PinCPU {
		log.Debug("  - pinning %s to cpuset: %s", c.PrettyName(), cpus)
		c.SetCpusetCpus(cpus.String())
		if reqCpu, ok := c.GetResourceRequirements().Requests[corev1.ResourceCPU]; ok {
			mCpu := int(reqCpu.MilliValue())
			c.SetCPUShares(int64(cache.MilliCPUToShares(mCpu)))
		}
	}
	if p.bpoptions.PinMemory == nil || *p.bpoptions.PinMemory {
		log.Debug("  - pinning %s to memory %s", c.PrettyName(), mems)
		c.SetCpusetMems(mems.String())
	}
}

// balloonsError formats an error from this policy.
func balloonsError(format string, args ...interface{}) error {
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Register us as a policy implementation.
func init() {
	policy.Register(PolicyName, PolicyDescription, CreateBalloonsPolicy)
}
