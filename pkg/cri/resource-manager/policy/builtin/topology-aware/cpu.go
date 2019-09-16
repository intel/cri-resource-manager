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
	"fmt"
	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

// CPUSupply represents avaialbe CPU capacity of a node.
type CPUSupply interface {
	// GetNode returns the node supplying this capacity.
	GetNode() Node
	// Clone creates a copy of this CPUSupply.
	Clone() CPUSupply
	// IsolatedCPUs returns the isolated cpuset in this supply.
	IsolatedCPUs() cpuset.CPUSet
	// SharableCPUs returns the sharable cpuset in this supply.
	SharableCPUs() cpuset.CPUSet
	// Granted returns the locally granted capacity in this supply.
	Granted() int
	// Cumulate cumulates the given supply into this one.
	Cumulate(CPUSupply)
	// AccountAllocate accounts for (removes) allocated exclusive capacity from the supply.
	AccountAllocate(CPUGrant)
	// AccountRelease accounts for (reinserts) released exclusive capacity into the supply.
	AccountRelease(CPUGrant)
	// Score calculates how well this supply fits/fulfills the given request.
	Score(CPURequest) float64
	// Allocate allocates CPU capacity from this supply and returns it as a grant.
	Allocate(CPURequest) (CPUGrant, error)
	// Release releases a previously allocated grant.
	Release(CPUGrant)
	// String returns a printable representation of this supply.
	String() string
}

// CPURequest represents a CPU resources requested by a container.
type CPURequest interface {
	// GetContainer returns the container requesting CPU capacity.
	GetContainer() cache.Container
	// String returns a printable representation of this request.
	String() string
}

// CPUGrant represents CPU capacity allocated to a container from a node.
type CPUGrant interface {
	// GetContainer returns the container CPU capacity is granted to.
	GetContainer() cache.Container
	// GetNode returns the node that granted CPU capacity to the container.
	GetNode() Node
	// ExclusiveCPUs returns the exclusively granted non-isolated cpuset.
	ExclusiveCPUs() cpuset.CPUSet
	// SharedCPUs returns the shared granted cpuset.
	SharedCPUs() cpuset.CPUSet
	// SharedPortion returns the amount of CPUs in milli-CPU granted.
	SharedPortion() int
	// IsolatedCpus returns the exclusively granted isolated cpuset.
	IsolatedCPUs() cpuset.CPUSet
	// String returns a printable representation of this grant.
	String() string
}

// cpuSupply implements our CpuSupply interface.
type cpuSupply struct {
	node     Node          // node supplying CPUs
	isolated cpuset.CPUSet // isolated CPUs at this node
	sharable cpuset.CPUSet // sharable CPUs at this node
	granted  int           // amount of sharable allocated
}

var _ CPUSupply = &cpuSupply{}

// cpuRequest implements our CpuRequest interface.
type cpuRequest struct {
	container cache.Container // container for this request
	full      int             // number of full CPUs requested
	fraction  int             // amount of fractional CPU requested
	exclusive int             // full CPUs requested
	shared    int             // partial CPU requested in milli-CPUs
	isolate   bool            // prefer isolated exclusive CPUs
	elevate   int             // displace allocation up in the tree
}

var _ CPURequest = &cpuRequest{}

// cpuGrant implements our CpuGrant interface.
type cpuGrant struct {
	container cache.Container // container CPU is granted to
	node      Node            // node CPU is supplied from
	exclusive cpuset.CPUSet   // exclusive CPUs
	portion   int             // milliCPUs granted from shared set
}

var _ CPUGrant = &cpuGrant{}

// newCPUSupply creates CPU supply for the given node, cpusets and existing grant.
func newCPUSupply(n Node, isolated, sharable cpuset.CPUSet, granted int) CPUSupply {
	return &cpuSupply{
		node:     n,
		isolated: isolated.Clone(),
		sharable: sharable.Clone(),
		granted:  granted,
	}
}

// GetNode returns the node supplying CPU.
func (cs *cpuSupply) GetNode() Node {
	return cs.node
}

// Clone clones the given CPU supply.
func (cs *cpuSupply) Clone() CPUSupply {
	return newCPUSupply(cs.node, cs.isolated, cs.sharable, cs.granted)
}

// IsolatedCpus returns the isolated CPUSet of this supply.
func (cs *cpuSupply) IsolatedCPUs() cpuset.CPUSet {
	return cs.isolated.Clone()
}

// SharableCpus returns the sharable CPUSet of this supply.
func (cs *cpuSupply) SharableCPUs() cpuset.CPUSet {
	return cs.sharable.Clone()
}

// Granted returns the locally granted sharable CPU capacity.
func (cs *cpuSupply) Granted() int {
	return cs.granted
}

// Cumulate more CPU to supply.
func (cs *cpuSupply) Cumulate(more CPUSupply) {
	mcs := more.(*cpuSupply)

	cs.isolated = cs.isolated.Union(mcs.isolated)
	cs.sharable = cs.sharable.Union(mcs.sharable)
	cs.granted += mcs.granted
}

// AccountAllocate accounts for (removes) allocated exclusive capacity from the supply.
func (cs *cpuSupply) AccountAllocate(g CPUGrant) {
	if cs.node.IsSameNode(g.GetNode()) {
		return
	}
	exclusive := g.ExclusiveCPUs()
	cs.isolated = cs.isolated.Difference(exclusive)
	cs.sharable = cs.sharable.Difference(exclusive)
}

// AccountRelease accounts for (reinserts) released exclusive capacity into the supply.
func (cs *cpuSupply) AccountRelease(g CPUGrant) {
	if cs.node.IsSameNode(g.GetNode()) {
		return
	}

	ncs := cs.node.GetCPU()
	nodecpus := ncs.IsolatedCPUs().Union(ncs.SharableCPUs())
	grantcpus := g.ExclusiveCPUs().Intersection(nodecpus)

	isolated := grantcpus.Intersection(ncs.IsolatedCPUs())
	sharable := grantcpus.Intersection(ncs.SharableCPUs())
	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)
}

// Score calculates the fitting score of a request for this supply.
func (cs *cpuSupply) Score(r CPURequest) float64 {
	var score, hscore float64

	cr := r.(*cpuRequest)
	hints := cr.container.GetTopologyHints()

	pod, _ := cr.container.GetPod()
	fakekey := pod.GetName() + ":" + cr.container.GetName()
	if fake, ok := opt.Hints[fakekey]; ok {
		for src, fh := range fake {
			hints[src] = fh
		}
	}
	if fake, ok := opt.Hints[cr.container.GetName()]; ok {
		for src, fh := range fake {
			hints[src] = fh
		}
	}

	log.Debug("* scoring %s (for %s)", cs.String(), cr.String())
	if len(hints) > 0 {
		log.Debug("  - with topology hints:")
		for _, h := range hints {
			log.Debug("   %s", h.String())
		}
	} else {
		log.Debug("  - without topology hints...")
	}

	full, part, isolate := cr.full, cr.fraction, cr.isolate

	if full == 0 && part == 0 {
		part = 1
		isolate = false
	}

	granted := cs.node.GrantedCPU()
	hisolated := cs.maskByHints(cs.isolated, hints)
	hsharable := cs.maskByHints(cs.sharable, hints)
	hgranted := float64(hsharable.Size()) / float64(cs.sharable.Size()) * float64(granted)

	// hint-score is how big portion of the request we can do in a hint-satisfying manner

	switch {
	// no exclusive capacity
	case isolate && (cs.isolated.Size() < full && 1000*cs.sharable.Size()-granted < 1000*full):
		log.Debug("  - no spare isolated or slicable exclusive capacity: score 0.0")
		score = 0.0

		// no shared capacity
	case 1000*cs.sharable.Size()-granted < part:
		log.Debug("  - no spare shared capacity: score 0.0")
		score = 0.0

		// perfect isolated exclusive fit
	case isolate && (full > 0 && part == 0 && cs.isolated.Size() >= full):
		log.Debug("  - perfect isolated exclusive fit: score 1.0")
		score = 1.0
		hscore = float64(hisolated.Size()) / float64(full)

		// perfect shared-only fit
	case full == 0 && 1000*cs.sharable.Size()-granted >= part:
		log.Debug("  - perfect sharable fit: score 1.0")
		score = 1.0
		if part == 0 {
			hscore = 1.0
		} else {
			hscore = (float64(hsharable.Size()) - hgranted) / float64(part)
		}

		// perfect isolated + shared fit
	case isolate && (full > 0 && part > 0 &&
		cs.isolated.Size() >= full && 1000*cs.sharable.Size()-granted >= part):
		log.Debug("  - perfect isolated + shared fit: score 1.0")
		score = 1.0

		hscore = (float64(1000*hisolated.Size()+1000*hsharable.Size()) - hgranted) /
			float64(1000*full+part)

		// will need to slice off sharable capacity for exclusive usage:
	default:
		log.Debug("  - need to slice of %d sharable CPUs", full)
		score = 1.0

		hscore = (float64(1000*hsharable.Size()) - hgranted - float64(1000*full-part)) /
			float64(1000*full+part)
	}

	if hscore > 1 {
		hscore = 1
	}
	if hscore < 0 {
		hscore = 0
	}

	log.Debug("  => score: %f, hint-score: %f => score: %f", score, hscore, score*hscore)

	score = score * hscore

	return score
}

// Allocate allocates a grant from the supply.
func (cs *cpuSupply) Allocate(r CPURequest) (CPUGrant, error) {
	var exclusive cpuset.CPUSet
	var err error

	cr := r.(*cpuRequest)

	// allocate isolated exclusive CPUs or slice them off the sharable set
	switch {
	case cr.full > 0 && cs.isolated.Size() >= cr.full:
		exclusive, err = takeCPUs(&cs.isolated, nil, cr.full)
		if err != nil {
			return nil, policyError("internal error: "+
				"can't allocate %d exclusive CPUs from %s of %s",
				cr.full, cs.isolated.String(), cs.node.Name())
		}

	case cr.full > 0 && (1000*cs.sharable.Size()-cs.granted)/1000 > cr.full:
		exclusive, err = takeCPUs(&cs.sharable, nil, cr.full)
		if err != nil {
			return nil, policyError("internal error: "+
				"can't slice %d exclusive CPUs from %s(-%d) of %s",
				cr.full, cs.sharable.String(), cs.granted, cs.node.Name())
		}
	}

	// allocate requested portion of the sharable set
	if cr.fraction > 0 {
		if 1000*cs.sharable.Size()-cs.granted < cr.fraction {
			return nil, policyError("internal error: "+
				"not enough sharable CPU for %d in %s(-%d) of %s",
				cr.fraction, cs.sharable.String(), cs.granted, cs.node.Name())
		}
		cs.granted += cr.fraction
	}

	grant := newCPUGrant(cs.node, cr.GetContainer(), exclusive, cr.fraction)

	cs.node.DepthFirst(func(n Node) error {
		n.FreeCPU().AccountAllocate(grant)
		return nil
	})

	return grant, nil
}

// Release returns CPU from the given grant to the supply.
func (cs *cpuSupply) Release(g CPUGrant) {
	isolated := g.ExclusiveCPUs().Intersection(cs.node.GetCPU().IsolatedCPUs())
	sharable := g.ExclusiveCPUs().Difference(isolated)

	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)
	cs.granted -= g.SharedPortion()

	cs.node.DepthFirst(func(n Node) error {
		n.FreeCPU().AccountRelease(g)
		return nil
	})
}

// String returns the CPU supply as a string.
func (cs *cpuSupply) String() string {
	none, isolated, sharable, sep := "-", "", "", ""

	if !cs.isolated.IsEmpty() {
		isolated = fmt.Sprintf("isolated:%s", cs.isolated.String())
		sep = ", "
		none = ""
	}
	if !cs.sharable.IsEmpty() {
		sharable = fmt.Sprintf("%ssharable:%s (granted:%d, free: %d)", sep,
			cs.sharable.String(), cs.granted, 1000*cs.sharable.Size()-cs.granted)
		none = ""
	}

	return "<" + cs.node.Name() + " CPU: " + none + isolated + sharable + ">"
}

// newCPURequest creates a new CPU request for the given container.
func newCPURequest(container cache.Container) CPURequest {
	pod, _ := container.GetPod()
	full, fraction, isolate, elevate := cpuAllocationPreferences(pod, container)

	return &cpuRequest{
		container: container,
		full:      full,
		fraction:  fraction,
		isolate:   isolate,
		elevate:   elevate,
	}
}

// GetContainer returns the container requesting CPU.
func (cr *cpuRequest) GetContainer() cache.Container {
	return cr.container
}

// String returns aprintable representation of the CPU request.
func (cr *cpuRequest) String() string {
	isolated := map[bool]string{false: "", true: "isolated "}[cr.isolate]
	switch {
	case cr.full == 0 && cr.fraction == 0:
		return fmt.Sprintf("<CPU request " + cr.container.GetCacheID() + ": ->")

	case cr.full > 0 && cr.fraction > 0:
		return fmt.Sprintf("<CPU request "+cr.container.GetCacheID()+": "+
			"%sfull: %d, shared: %d>", isolated, cr.full, cr.fraction)

	case cr.full > 0:
		return fmt.Sprintf("<CPU request "+
			cr.container.GetCacheID()+": %sfull: %d>", isolated, cr.full)

	default:
		return fmt.Sprintf("<CPU request "+
			cr.container.GetCacheID()+": shared: %d>", cr.fraction)
	}
}

// newCPUGrant creates a CPU grant from the given node for the container.
func newCPUGrant(n Node, c cache.Container, exclusive cpuset.CPUSet, portion int) CPUGrant {
	return &cpuGrant{
		node:      n,
		container: c,
		exclusive: exclusive,
		portion:   portion,
	}
}

// GetContainer returns the container this grant is valid for.
func (cg *cpuGrant) GetContainer() cache.Container {
	return cg.container
}

// GetNode returns the Node this grant is allocated to.
func (cg *cpuGrant) GetNode() Node {
	return cg.node
}

// ExclusiveCPUs returns the non-isolated exclusive CPUSet in this grant.
func (cg *cpuGrant) ExclusiveCPUs() cpuset.CPUSet {
	return cg.exclusive
}

// SharedCPUs returns the shared CPUSet in this grant.
func (cg *cpuGrant) SharedCPUs() cpuset.CPUSet {
	return cg.node.GetCPU().SharableCPUs()
	//return cg.shared
}

// SharedPortion returns the milli-CPU allocation for the shared CPUSet in this grant.
func (cg *cpuGrant) SharedPortion() int {
	return cg.portion
}

// ExclusiveCPUs returns the isolated exclusive CPUSet in this grant.
func (cg *cpuGrant) IsolatedCPUs() cpuset.CPUSet {
	return cg.node.GetCPU().IsolatedCPUs().Intersection(cg.exclusive)
}

// String returns a printable representation of the CPU grant.
func (cg *cpuGrant) String() string {
	var isolated, exclusive, shared, sep string

	isol := cg.IsolatedCPUs()
	if !isol.IsEmpty() {
		isolated = fmt.Sprintf("isolated: %s", isol.String())
		sep = ", "
	}
	if !cg.exclusive.IsEmpty() {
		exclusive = fmt.Sprintf("%sexclusive: %s", sep, cg.exclusive.String())
		sep = ", "
	}
	if cg.portion > 0 {
		shared = fmt.Sprintf("%sshared: %s (%d milli-CPU)", sep,
			cg.node.FreeCPU().SharableCPUs().String(), cg.portion)
	}

	return fmt.Sprintf("<CPU grant for %s from %s: %s%s%s>",
		cg.container.GetCacheID(), cg.node.Name(), isolated, exclusive, shared)
}

// takeCPUs takes up to cnt CPUs from a given CPU set to another.
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
