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

package cache

import (
	"fmt"
	"sigs.k8s.io/yaml"

	"github.com/intel/cri-resource-manager/pkg/apis/resmgr"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
)

const (
	// annotation key for specifying container affinity rules
	keyAffinity = "affinity"
	// annotation key for specifying container anti-affinity rules
	keyAntiAffinity = "anti-affinity"
)

// Expression is used to describe affinity container scope and matching criteria.
type Expression struct {
	resmgr.Expression
}

// simpleAffinity is an alternative, simplified syntax for intra-pod container affinity.
type simpleAffinity map[string][]string

// PodContainerAffinity defines a set of per-container affinities and anti-affinities.
type podContainerAffinity map[string][]*Affinity

// Affinity specifies a single container affinity.
type Affinity struct {
	Scope  *resmgr.Expression `json:"scope,omitempty"`  // scope for evaluating this affinity
	Match  *resmgr.Expression `json:"match"`            // affinity expression
	Weight int32              `json:"weight,omitempty"` // (optional) weight for this affinity
}

// Operator defines the possible operators for an Expression.
type Operator string

const (
	// Equals tests for equality with a single value.
	Equals Operator = "Equals"
	// NotEqual test for inequality with a single value.
	NotEqual Operator = "NotEqual"
	// In tests if the key's value is one of the specified set.
	In Operator = "In"
	// NotIn tests if the key's value is not one of the specified set.
	NotIn Operator = "NotIn"
	// Exists evalutes to true if the named key exists.
	Exists Operator = "Exists"
	// NotExist evalutes to true if the named key does not exist.
	NotExist Operator = "NotExist"
	// AlwaysTrue always evaluates to true.
	AlwaysTrue = "AlwaysTrue"
	// UserWeightCutoff is the cutoff we clamp user-provided weights to.
	UserWeightCutoff = 1000
	// DefaultWeight is the default assigned weight if omitted in annotations.
	DefaultWeight int32 = 1
)

// ImplicitAffinity is an affinity that gets implicitly added to all eligible containers.
type ImplicitAffinity struct {
	Eligible func(Container) bool // function to determine if Affinity is added to a Container
	Affinity *Affinity            // the actual implicitly added Affinity
}

// Validate checks the affinity for (obvious) invalidity.
func (a *Affinity) Validate() error {
	if err := a.Scope.Validate(); err != nil {
		return cacheError("invalid affinity scope: %v", err)
	}

	if err := a.Match.Validate(); err != nil {
		return cacheError("invalid affinity match: %v", err)
	}

	switch {
	case a.Weight > UserWeightCutoff:
		a.Weight = UserWeightCutoff
	case a.Weight < -UserWeightCutoff:
		a.Weight = -UserWeightCutoff
	}

	return nil
}

// EvaluateAffinity evaluates the given affinity against all known in-scope containers.
func (cch *cache) EvaluateAffinity(a *Affinity) map[string]int32 {
	results := make(map[string]int32)
	for _, c := range cch.FilterScope(a.Scope) {
		if a.Match.Evaluate(c) {
			id := c.GetCacheID()
			results[id] += a.Weight
		}
	}
	return results
}

// FilterScope returns the containers selected by the scope expression.
func (cch *cache) FilterScope(scope *resmgr.Expression) []Container {
	cch.Debug("calculating scope %s", scope.String())
	result := []Container{}
	for _, c := range cch.GetContainers() {
		if scope.Evaluate(c) {
			cch.Debug("  + container %s: IN scope", c.PrettyName())
			result = append(result, c)
		} else {
			cch.Debug("  - container %s: NOT IN scope", c.PrettyName())
		}
	}
	return result
}

// String returns the affinity as a string.
func (a *Affinity) String() string {
	kind := ""
	if a.Weight < 0 {
		kind = "anti-"
	}
	return fmt.Sprintf("<%saffinity: scope %s %s => %d>",
		kind, a.Scope.String(), a.Match.String(), a.Weight)
}

// Try to parse affinities in simplified notation from the given annotation value.
func (pca *podContainerAffinity) parseSimple(pod *pod, value string, weight int32) bool {
	parsed := simpleAffinity{}
	if err := yaml.Unmarshal([]byte(value), &parsed); err != nil {
		return false
	}

	podScope := pod.ScopeExpression()
	for name, values := range parsed {
		(*pca)[name] = append((*pca)[name],
			&Affinity{
				Scope: podScope,
				Match: &resmgr.Expression{
					Key:    kubernetes.ContainerNameLabel,
					Op:     resmgr.In,
					Values: values,
				},
				Weight: weight,
			})
	}

	return true
}

// Try to parse affinities in full notation from the given annotation value.
func (pca *podContainerAffinity) parseFull(pod *pod, value string, weight int32) error {
	parsed := podContainerAffinity{}
	if err := yaml.Unmarshal([]byte(value), &parsed); err != nil {
		return cacheError("failed to parse affinity annotation '%s': %v", value, err)
	}

	podScope := pod.ScopeExpression()
	for name, pa := range parsed {
		ca, ok := (*pca)[name]
		if !ok {
			ca = make([]*Affinity, 0, len(pa))
		}

		for _, a := range pa {
			if a.Scope == nil {
				a.Scope = podScope
			}
			if a.Weight == 0 {
				a.Weight = weight
			} else {
				if weight < 0 {
					a.Weight *= -1
				}
			}
			if err := a.Validate(); err != nil {
				return err
			}

			ca = append(ca, a)
		}

		(*pca)[name] = ca
	}

	return nil
}

// GlobalAffinity creates an affinity with all containers in scope.
func GlobalAffinity(key string, weight int32) *Affinity {
	return &Affinity{
		Scope: &resmgr.Expression{
			Op: resmgr.AlwaysTrue, // evaluate against all containers
		},
		Match: &resmgr.Expression{
			Key: key,
			Op:  resmgr.Exists,
		},
		Weight: weight,
	}
}

// GlobalAntiAffinity creates an anti-affinity with all containers in scope.
func GlobalAntiAffinity(key string, weight int32) *Affinity {
	return GlobalAffinity(key, -weight)
}

// AddImplicitAffinities registers a set of implicit affinities.
func (cch *cache) AddImplicitAffinities(implicit map[string]*ImplicitAffinity) error {
	for name := range implicit {
		if existing, ok := cch.implicit[name]; ok {
			return cacheError("implicit affinity %s already defined (%s)",
				name, existing.Affinity.String())
		}
	}
	for name, a := range implicit {
		cch.implicit[name] = a
	}
	return nil
}

// DeleteImplicitAffinities removes a previously registered set of implicit affinities.
func (cch *cache) DeleteImplicitAffinities(names []string) {
	for _, name := range names {
		delete(cch.implicit, name)
	}
}
