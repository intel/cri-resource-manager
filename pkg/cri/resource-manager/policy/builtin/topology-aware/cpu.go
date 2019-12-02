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
	GetNode() *node
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
	// GetScore calculates how well this supply fits/fulfills the given request.
	GetScore(CPURequest) CPUScore
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

	// FullCPUs return the number of full CPUs requested.
	FullCPUs() int
	// CPUFraction returns the amount of fractional milli-CPU requested.
	CPUFraction() int
	// Isolate returns whether isolated CPUs are preferred for this request.
	Isolate() bool
	// Elevate returns the requested elevation/allocation displacement for this request.
	Elevate() int
}

// CPUGrant represents CPU capacity allocated to a container from a node.
type CPUGrant interface {
	// GetContainer returns the container CPU capacity is granted to.
	GetContainer() cache.Container
	// GetNode returns the node that granted CPU capacity to the container.
	GetNode() *node
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

// CPUScore represents how well a supply can satisfy a request.
type CPUScore interface {
	// Calculate the actual score from the collected parameters.
	Eval() float64
	// CPUSupply returns the supply associated with this score.
	CPUSupply() CPUSupply
	// CPURequest returns the request associated with this score.
	CPURequest() CPURequest

	IsolatedCapacity() int
	SharedCapacity() int
	Colocated() int
	HintScores() map[string]float64

	String() string
}

// cpuSupply implements our CPUSupply interface.
type cpuSupply struct {
	node     *node         // node supplying CPUs
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
	isolate   bool            // prefer isolated exclusive CPUs

	// elevate indicates how much to elevate the actual allocation of the
	// container in the tree of pools. Or in other words how many levels to
	// go up in the tree starting at the best fitting pool, before assigning
	// the container to an actual pool. Currently ignored.
	elevate int
}

var _ CPURequest = &cpuRequest{}

// cpuGrant implements our CpuGrant interface.
type cpuGrant struct {
	container cache.Container // container CPU is granted to
	node      *node           // node CPU is supplied from
	exclusive cpuset.CPUSet   // exclusive CPUs
	portion   int             // milliCPUs granted from shared set
}

var _ CPUGrant = &cpuGrant{}

// cpuScore implements our CPUScore interface.
type cpuScore struct {
	supply    CPUSupply          // CPU supply (node)
	request   CPURequest         // CPU request (container)
	isolated  int                // remaining isolated CPUs
	shared    int                // remaining shared capacity
	colocated int                // number of colocated containers
	hints     map[string]float64 // hint scores
}

var _ CPUScore = &cpuScore{}

// newCPUSupply creates CPU supply for the given node, cpusets and existing grant.
func newCPUSupply(n *node, isolated, sharable cpuset.CPUSet, granted int) CPUSupply {
	return &cpuSupply{
		node:     n,
		isolated: isolated.Clone(),
		sharable: sharable.Clone(),
		granted:  granted,
	}
}

// GetNode returns the node supplying CPU.
func (cs *cpuSupply) GetNode() *node {
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
	if cs.node.isSameNode(g.GetNode()) {
		return
	}
	exclusive := g.ExclusiveCPUs()
	cs.isolated = cs.isolated.Difference(exclusive)
	cs.sharable = cs.sharable.Difference(exclusive)
}

// AccountRelease accounts for (reinserts) released exclusive capacity into the supply.
func (cs *cpuSupply) AccountRelease(g CPUGrant) {
	if cs.node.isSameNode(g.GetNode()) {
		return
	}

	ncs := cs.node.nodecpu.Clone()
	nodecpus := ncs.IsolatedCPUs().Union(ncs.SharableCPUs())
	grantcpus := g.ExclusiveCPUs().Intersection(nodecpus)

	isolated := grantcpus.Intersection(ncs.IsolatedCPUs())
	sharable := grantcpus.Intersection(ncs.SharableCPUs())
	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)
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
				cr.full, cs.isolated, cs.node.name)
		}

	case cr.full > 0 && (1000*cs.sharable.Size()-cs.granted)/1000 > cr.full:
		exclusive, err = takeCPUs(&cs.sharable, nil, cr.full)
		if err != nil {
			return nil, policyError("internal error: "+
				"can't slice %d exclusive CPUs from %s(-%d) of %s",
				cr.full, cs.sharable, cs.granted, cs.node.name)
		}
	}

	// allocate requested portion of the sharable set
	if cr.fraction > 0 {
		if 1000*cs.sharable.Size()-cs.granted < cr.fraction {
			return nil, policyError("internal error: "+
				"not enough sharable CPU for %d in %s(-%d) of %s",
				cr.fraction, cs.sharable, cs.granted, cs.node.name)
		}
		cs.granted += cr.fraction
	}

	grant := newCPUGrant(cs.node, cr.GetContainer(), exclusive, cr.fraction)

	cs.node.depthFirst(func(n *node) error {
		n.freecpu.AccountAllocate(grant)
		return nil
	})

	return grant, nil
}

// Release returns CPU from the given grant to the supply.
func (cs *cpuSupply) Release(g CPUGrant) {
	isolated := g.ExclusiveCPUs().Intersection(cs.node.nodecpu.Clone().IsolatedCPUs())
	sharable := g.ExclusiveCPUs().Difference(isolated)

	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)
	cs.granted -= g.SharedPortion()

	cs.node.depthFirst(func(n *node) error {
		n.freecpu.AccountRelease(g)
		return nil
	})
}

// String returns the CPU supply as a string.
func (cs *cpuSupply) String() string {
	none, isolated, sharable, sep := "-", "", "", ""

	if !cs.isolated.IsEmpty() {
		isolated = fmt.Sprintf("isolated:%s", cs.isolated)
		sep = ", "
		none = ""
	}
	if !cs.sharable.IsEmpty() {
		sharable = fmt.Sprintf("%ssharable:%s (granted:%d, free: %d)", sep,
			cs.sharable, cs.granted, 1000*cs.sharable.Size()-cs.granted)
		none = ""
	}

	return "<" + cs.node.name + " CPU: " + none + isolated + sharable + ">"
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
		return fmt.Sprintf("<CPU request " + cr.container.PrettyName() + ": ->")

	case cr.full > 0 && cr.fraction > 0:
		return fmt.Sprintf("<CPU request "+cr.container.PrettyName()+": "+
			"%sfull: %d, shared: %d>", isolated, cr.full, cr.fraction)

	case cr.full > 0:
		return fmt.Sprintf("<CPU request "+
			cr.container.PrettyName()+": %sfull: %d>", isolated, cr.full)

	default:
		return fmt.Sprintf("<CPU request "+
			cr.container.PrettyName()+": shared: %d>", cr.fraction)
	}
}

// FullCPUs return the number of full CPUs requested.
func (cr *cpuRequest) FullCPUs() int {
	return cr.full
}

// CPUFraction returns the amount of fractional milli-CPU requested.
func (cr *cpuRequest) CPUFraction() int {
	return cr.fraction
}

// Isolate returns whether isolated CPUs are preferred for this request.
func (cr *cpuRequest) Isolate() bool {
	return cr.isolate
}

// Elevate returns the requested elevation/allocation displacement for this request.
func (cr *cpuRequest) Elevate() int {
	return cr.elevate
}

// Score collects data for scoring this supply wrt. the given request.
func (cs *cpuSupply) GetScore(request CPURequest) CPUScore {
	score := &cpuScore{
		supply:  cs,
		request: request,
	}

	cr := request.(*cpuRequest)
	full, part := cr.full, cr.fraction
	if full == 0 && part == 0 {
		part = 1
	}

	// calculate free shared capacity
	score.shared = 1000*cs.sharable.Size() - cs.node.grantedCPU()

	// calculate isolated node capacity CPU
	if cr.isolate {
		score.isolated = cs.isolated.Size() - full
	}

	// if we don't want isolated or there is not enough, calculate slicable capacity
	if !cr.isolate || score.isolated < 0 {
		score.shared -= 1000 * full
	}

	// calculate fractional capacity
	score.shared -= part

	// calculate colocation score
	for _, grant := range cs.node.policy.allocations.CPU {
		if grant.GetNode().isSameNode(cs.node) {
			score.colocated++
		}
	}

	// calculate real hint scores
	hints := cr.container.GetTopologyHints()
	score.hints = make(map[string]float64, len(hints))

	for provider, hint := range cr.container.GetTopologyHints() {
		log.Debug(" - evaluating topology hint %s", hint)
		score.hints[provider] = cs.node.hintScore(hint)
	}

	// calculate any fake hint scores
	pod, _ := cr.container.GetPod()
	key := pod.GetName() + ":" + cr.container.GetName()
	if fakeHints, ok := opt.FakeHints[key]; ok {
		for provider, hint := range fakeHints {
			log.Debug(" - evaluating fake hint %s", hint)
			score.hints[provider] = cs.node.hintScore(hint)
		}
	}
	if fakeHints, ok := opt.FakeHints[cr.container.GetName()]; ok {
		for provider, hint := range fakeHints {
			log.Debug(" - evaluating fake hint %s", hint)
			score.hints[provider] = cs.node.hintScore(hint)
		}
	}

	return score
}

// Eval...
func (score *cpuScore) Eval() float64 {
	return 1.0
}

func (score *cpuScore) CPUSupply() CPUSupply {
	return score.supply
}

func (score *cpuScore) CPURequest() CPURequest {
	return score.request
}

func (score *cpuScore) IsolatedCapacity() int {
	return score.isolated
}

func (score *cpuScore) SharedCapacity() int {
	return score.shared
}

func (score *cpuScore) Colocated() int {
	return score.colocated
}

func (score *cpuScore) HintScores() map[string]float64 {
	return score.hints
}

func (score *cpuScore) String() string {
	return fmt.Sprintf("<CPU score: node %s, isolated:%d, shared:%d, colocated:%d, hints: %v>",
		score.supply.GetNode().name, score.isolated, score.shared, score.colocated, score.hints)
}

// newCPUGrant creates a CPU grant from the given node for the container.
func newCPUGrant(n *node, c cache.Container, exclusive cpuset.CPUSet, portion int) CPUGrant {
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
func (cg *cpuGrant) GetNode() *node {
	return cg.node
}

// ExclusiveCPUs returns the non-isolated exclusive CPUSet in this grant.
func (cg *cpuGrant) ExclusiveCPUs() cpuset.CPUSet {
	return cg.exclusive
}

// SharedCPUs returns the shared CPUSet in this grant.
func (cg *cpuGrant) SharedCPUs() cpuset.CPUSet {
	return cg.node.nodecpu.Clone().SharableCPUs()
}

// SharedPortion returns the milli-CPU allocation for the shared CPUSet in this grant.
func (cg *cpuGrant) SharedPortion() int {
	return cg.portion
}

// ExclusiveCPUs returns the isolated exclusive CPUSet in this grant.
func (cg *cpuGrant) IsolatedCPUs() cpuset.CPUSet {
	return cg.node.nodecpu.Clone().IsolatedCPUs().Intersection(cg.exclusive)
}

// String returns a printable representation of the CPU grant.
func (cg *cpuGrant) String() string {
	var isolated, exclusive, shared, sep string

	isol := cg.IsolatedCPUs()
	if !isol.IsEmpty() {
		isolated = fmt.Sprintf("isolated: %s", isol)
		sep = ", "
	}
	if !cg.exclusive.IsEmpty() {
		exclusive = fmt.Sprintf("%sexclusive: %s", sep, cg.exclusive)
		sep = ", "
	}
	if cg.portion > 0 {
		shared = fmt.Sprintf("%sshared: %s (%d milli-CPU)", sep,
			cg.node.freecpu.SharableCPUs(), cg.portion)
	}

	return fmt.Sprintf("<CPU grant for %s from %s: %s%s%s>",
		cg.container.PrettyName(), cg.node.name, isolated, exclusive, shared)
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
