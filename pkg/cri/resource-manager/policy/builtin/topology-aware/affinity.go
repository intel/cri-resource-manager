// Copyright Intel Corporation. All Rights Reserved.
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
	"github.com/intel/cri-resource-manager/pkg/apis/resmgr"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
)

// Calculate pool affinities for the given container.
func (p *policy) calculatePoolAffinities(container cache.Container) (map[int]int32, error) {
	log.Debug("=> calculating pool affinities...")

	affinities, err := p.calculateContainerAffinity(container)
	if err != nil {
		return nil, err
	}

	result := make(map[int]int32, len(p.nodes))
	for id, w := range affinities {
		grant, ok := p.allocations.grants[id]
		if !ok {
			continue
		}
		node := grant.GetCPUNode()
		result[node.NodeID()] += w

		// TODO: calculate affinity for memory here too?
	}

	return result, nil
}

// Calculate affinity of this container (against all other containers).
func (p *policy) calculateContainerAffinity(container cache.Container) (map[string]int32, error) {
	log.Debug("* calculating affinity for container %s...", container.PrettyName())

	ca, err := container.GetAffinity()
	if err != nil {
		return nil, err
	}

	result := make(map[string]int32)
	for _, a := range ca {
		for id, w := range p.cache.EvaluateAffinity(a) {
			result[id] += w
		}
	}

	// self-affinity does not make sense, so remove any
	delete(result, container.GetCacheID())

	log.Debug("  => affinity: %v", result)

	return result, nil
}

// Register our policy-specific implicit affinities with the Cache.
func (p *policy) registerImplicitAffinities() error {
	affinities := []struct {
		name     string
		disabled bool
		affinity cache.ImplicitAffinity
	}{
		{
			name: "AVX512-pull/push",
			affinity: func(c cache.Container, hasExplicit bool) *cache.Affinity {
				_, tagged := c.GetTag(cache.TagAVX512)
				if tagged {
					return cache.GlobalAffinity("tags/"+cache.TagAVX512, 5)
				}
				return cache.GlobalAntiAffinity("tags/"+cache.TagAVX512, 5)
			},
		},
		{
			name:     "colocate-pods",
			disabled: !opt.ColocatePods,
			affinity: func(c cache.Container, hasExplicit bool) *cache.Affinity {
				if hasExplicit {
					return nil
				}
				pod, ok := c.GetPod()
				if !ok {
					log.Error("failed to inject pod-colocation affinity, can't find pod")
					return nil
				}
				return &cache.Affinity{
					Scope: pod.ScopeExpression(),
					Match: &resmgr.Expression{
						Op: resmgr.AlwaysTrue,
					},
					Weight: 10,
				}
			},
		},
		{
			name:     "colocate-namespaces",
			disabled: !opt.ColocateNamespaces,
			affinity: func(c cache.Container, hasExplicit bool) *cache.Affinity {
				if hasExplicit {
					return nil
				}
				return &cache.Affinity{
					Scope: &resmgr.Expression{
						Op: resmgr.AlwaysTrue,
					},
					Match: &resmgr.Expression{
						Key: resmgr.KeyNamespace,
						Op:  resmgr.Equals,
						Values: []string{
							c.GetNamespace(),
						},
					},
					Weight: 10,
				}
			},
		},
	}

	enabled := map[string]cache.ImplicitAffinity{}
	for _, a := range affinities {
		if a.disabled {
			log.Info("implicit affinity %s is disabled", a.name)
			continue
		}
		enabled[PolicyName+":"+a.name] = a.affinity
	}

	if err := p.cache.AddImplicitAffinities(enabled); err != nil {
		return policyError("failed to register implicit affinities: %v", err)
	}

	return nil
}
