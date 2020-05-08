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

package memtier

import (
	"encoding/json"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
)

const (
	keyAllocations = "allocations"
	keyConfig      = "config"
)

func (p *policy) saveAllocations() {
	p.cache.SetPolicyEntry(keyAllocations, cache.Cachable(&p.allocations))
	p.cache.Save()
}

func (p *policy) restoreAllocations() error {
	// Get the allocations map.
	if !p.cache.GetPolicyEntry(keyAllocations, &p.allocations) {
		return nil
	}

	//
	// Based on the allocations
	//   1) update the free supply of the grant's pool to account for the grant
	//   2) set the extra memory allocations to the nodes below the grant in the tree
	//
	// We assume (for now) that the allocations are correct and the workloads don't
	// need moving.
	//
	for id, grant := range p.allocations.grants {
		pool := grant.GetCPUNode()

		log.Info("updating pool %s for container %s CPU grant", pool.Name(), id)
		supply := pool.FreeSupply()
		if err := supply.Reserve(grant); err != nil {
			return err
		}

		log.Info("updating pool %s for container %s extra memory", pool.Name(), id)
		pool = grant.GetMemoryNode()
		if err := supply.ReserveMemory(grant); err != nil {
			return err
		}
	}
	return nil
}

func (p *policy) saveConfig() error {
	cached := cachedOptions{Options: *opt}
	p.cache.SetPolicyEntry(keyConfig, cache.Cachable(&cached))
	p.cache.Save()
	return nil
}

func (p *policy) restoreConfig() bool {
	cached := cachedOptions{}
	if !p.cache.GetPolicyEntry(keyConfig, &cached) {
		return false
	}

	*opt = cached.Options
	return true
}

type cachedGrant struct {
	Exclusive   string
	Part        int
	Container   string
	Pool        string
	MemoryPool  string
	MemType     memoryType
	Memset      system.IDSet
	MemoryLimit memoryMap
	ColdStart   int
}

func newCachedGrant(cg Grant) *cachedGrant {
	ccg := &cachedGrant{}
	ccg.Exclusive = cg.ExclusiveCPUs().String()
	ccg.Part = cg.SharedPortion()
	ccg.Container = cg.GetContainer().GetCacheID()
	ccg.Pool = cg.GetCPUNode().Name()
	ccg.MemoryPool = cg.GetMemoryNode().Name()
	ccg.MemType = cg.MemoryType()
	ccg.Memset = cg.Memset().Clone()

	ccg.MemoryLimit = make(memoryMap)
	for key, value := range cg.MemLimit() {
		ccg.MemoryLimit[key] = value
	}

	ccg.ColdStart = cg.ColdStart()

	return ccg
}

func (ccg *cachedGrant) ToGrant(policy *policy) (Grant, error) {
	node, ok := policy.nodes[ccg.Pool]
	if !ok {
		return nil, policyError("cache error: failed to restore %v, unknown pool/node", *ccg)
	}
	container, ok := policy.cache.LookupContainer(ccg.Container)
	if !ok {
		return nil, policyError("cache error: failed to restore %v, unknown container", *ccg)
	}

	g := newGrant(
		node,
		container,
		cpuset.MustParse(ccg.Exclusive),
		ccg.Part,
		ccg.MemType,
		ccg.MemType,
		ccg.MemoryLimit,
		ccg.ColdStart,
	)

	if g.Memset().String() != ccg.Memset.String() {
		log.Error("cache error: mismatch in stored/recalculated memset: %s != %s",
			ccg.Memset, g.Memset())
	}

	return g, nil
}

func (cg *grant) MarshalJSON() ([]byte, error) {
	return json.Marshal(newCachedGrant(cg))
}

func (cg *grant) UnmarshalJSON(data []byte) error {
	ccg := cachedGrant{}

	if err := json.Unmarshal(data, &ccg); err != nil {
		return policyError("failed to restore grant: %v", err)
	}

	cg.exclusive = cpuset.MustParse(ccg.Exclusive)

	return nil
}

func (a *allocations) MarshalJSON() ([]byte, error) {
	cgrants := make(map[string]*cachedGrant)
	for id, cg := range a.grants {
		cgrants[id] = newCachedGrant(cg)
	}

	return json.Marshal(cgrants)
}

func (a *allocations) UnmarshalJSON(data []byte) error {
	var err error

	cgrants := make(map[string]*cachedGrant)
	if err := json.Unmarshal(data, &cgrants); err != nil {
		return policyError("failed to restore allocations: %v", err)
	}

	a.grants = make(map[string]Grant, 32)
	for id, ccg := range cgrants {
		a.grants[id], err = ccg.ToGrant(a.policy)
		if err != nil {
			log.Error("removing unresolvable cached grant %v: %v", *ccg, err)
			delete(a.grants, id)
		} else {
			log.Debug("resolved cache grant: %v", a.grants[id].String())
		}
	}

	return nil
}

func (a *allocations) Get() interface{} {
	return a
}

func (a *allocations) Set(value interface{}) {
	var from *allocations

	switch value.(type) {
	case allocations:
		v := value.(allocations)
		from = &v
	case *allocations:
		from = value.(*allocations)
	}

	a.grants = make(map[string]Grant, 32)
	for id, cg := range from.grants {
		a.grants[id] = cg
	}
}

func (a *allocations) Dump(logfn func(format string, args ...interface{}), prefix string) {
	for _, cg := range a.grants {
		logfn(prefix+"%s", cg)
	}
}

type cachedOptions struct {
	Options options
}

func (o *cachedOptions) Get() interface{} {
	return o
}

func (o *cachedOptions) Set(value interface{}) {
	switch value.(type) {
	case options:
		o.Options = value.(cachedOptions).Options
	case *options:
		o.Options = value.(*cachedOptions).Options
	}
}
