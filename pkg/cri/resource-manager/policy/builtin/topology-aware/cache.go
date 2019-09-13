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
	"encoding/json"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	keyAllocations = "allocations"
	keyConfig      = "config"
)

func (p *policy) saveAllocations() {
	p.cache.SetPolicyEntry(keyAllocations, cache.Cachable(&p.allocations))
	p.cache.Save()
}

func (p *policy) restoreAllocations() bool {
	return p.cache.GetPolicyEntry(keyAllocations, &p.allocations)
}

func (p *policy) saveConfig() error {
	cached := cachedOptions{Options: opt}
	p.cache.SetPolicyEntry(keyConfig, cache.Cachable(&cached))
	p.cache.Save()
	return nil
}

func (p *policy) restoreConfig() bool {
	cached := cachedOptions{}
	if !p.cache.GetPolicyEntry(keyConfig, &cached) {
		return false
	}

	// Notes:
	//   We merge the restored configuration, which is our fallback/default one until we
	//   get an up-to-date one from the agent, into the current one, which might container
	//   explicitly overridden defaults. In the merged configuration we want to keep the
	//   explicitly overridden defaults intact, again until we receive an up-to-date one.
	//   However, any new fake hints we merge into the restored configuration and save it.
	restored := &cached.Options
	opt.mergeFakeHints(restored)
	restored.Hints = opt.Hints
	opt.Hints = restored.Hints
	p.saveConfig()

	if opt.IsExplicit(optPinCpu) {
		restored.PinCpu = opt.PinCpu
	}
	if opt.IsExplicit(optPinMem) {
		restored.PinMem = opt.PinMem
	}
	if opt.IsExplicit(optPreferIsolated) {
		restored.PreferIsolated = opt.PreferIsolated
	}
	if opt.IsExplicit(optPreferShared) {
		restored.PreferShared = opt.PreferShared
	}

	opt = *restored

	return true
}

type cachedGrant struct {
	Exclusive string
	Part      int
	Container string
	Pool      string
}

func newCachedGrant(cg CpuGrant) *cachedGrant {
	ccg := &cachedGrant{}
	ccg.Exclusive = cg.ExclusiveCpus().String()
	ccg.Part = cg.SharedPortion()
	ccg.Container = cg.GetContainer().GetCacheId()
	ccg.Pool = cg.GetNode().Name()

	return ccg
}

func (ccg *cachedGrant) ToCpuGrant(policy *policy) (CpuGrant, error) {
	node, ok := policy.nodes[ccg.Pool]
	if !ok {
		return nil, policyError("cache error: failed to restore %v, unknown pool/node", *ccg)
	}
	container, ok := policy.cache.LookupContainer(ccg.Container)
	if !ok {
		return nil, policyError("cache error: failed to restore %v, unknown container", *ccg)
	}

	return newCpuGrant(
		node,
		container,
		cpuset.MustParse(ccg.Exclusive),
		ccg.Part,
	), nil
}

func (cg *cpuGrant) MarshalJSON() ([]byte, error) {
	return json.Marshal(newCachedGrant(cg))
}

func (cg *cpuGrant) UnmarshalJSON(data []byte) error {
	ccg := cachedGrant{}

	if err := json.Unmarshal(data, &ccg); err != nil {
		return policyError("failed to restore cpuGrant: %v", err)
	}

	cg.exclusive = cpuset.MustParse(ccg.Exclusive)

	return nil
}

func (a *allocations) MarshalJSON() ([]byte, error) {
	cgrants := make(map[string]*cachedGrant)
	for id, cg := range a.Cpu {
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

	a.Cpu = make(map[string]CpuGrant, 32)
	for id, ccg := range cgrants {
		a.Cpu[id], err = ccg.ToCpuGrant(a.policy)
		if err != nil {
			log.Error("removing unresolvable cached grant %v: %v", *ccg, err)
			delete(a.Cpu, id)
		} else {
			log.Debug("resolved cache grant: %v", a.Cpu[id].String())
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

	a.Cpu = make(map[string]CpuGrant, 32)
	for id, cg := range from.Cpu {
		a.Cpu[id] = cg
	}
}

func (a *allocations) Dump(logfn func(format string, args ...interface{}), prefix string) {
	for _, cg := range a.Cpu {
		logfn(prefix+"%s", cg.String())
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
