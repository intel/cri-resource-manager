// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
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

package v1alpha1

import (
	"fmt"
	"strings"

	resmgr "github.com/intel/cri-resource-manager/pkg/apis/resmgr"
	corev1 "k8s.io/api/core/v1"
)

// HasSameVersion checks if the policy has the same version as the other.
func (a *Adjustment) HasSameVersion(o *Adjustment) bool {
	if a.ResourceVersion != o.ResourceVersion {
		return false
	}
	if a.Generation != o.Generation {
		return false
	}
	return true
}

// NodeScope returns the sub-slice of scopes that apply to the given node.
func (spec *AdjustmentSpec) NodeScope(node string) []AdjustmentScope {
	filtered := []AdjustmentScope{}
	for _, scope := range spec.Scope {
		if scope.IsNodeInScope(node) {
			filtered = append(filtered, scope)
		}
	}
	return filtered
}

// GetResourceRequirements returns the k8s resource requirements for this adjustment.
func (spec *AdjustmentSpec) GetResourceRequirements() (corev1.ResourceRequirements, bool) {
	if spec.Resources != nil {
		return *spec.Resources, true
	}
	return corev1.ResourceRequirements{}, false
}

// GetRDTClass returns the RDT class for this adjustment.
func (spec *AdjustmentSpec) GetRDTClass() (string, bool) {
	if spec.Classes == nil || spec.Classes.RDT == nil {
		return "", false
	}
	return *spec.Classes.RDT, true
}

// GetBlockIOClass returns the Block I/O class for this adjustment.
func (spec *AdjustmentSpec) GetBlockIOClass() (string, bool) {
	if spec.Classes == nil || spec.Classes.BlockIO == nil {
		return "", false
	}
	return *spec.Classes.BlockIO, true
}

// IsNodeInScope tests if the node is within the scope of this spec.
func (spec *AdjustmentSpec) IsNodeInScope(node string) bool {
	if len(spec.Scope) == 0 {
		return true
	}
	for _, s := range spec.Scope {
		if s.IsNodeInScope(node) {
			return true
		}
	}
	return false
}

// IsContainerInScope tests if the container is within the scope of this spec.
func (spec *AdjustmentSpec) IsContainerInScope(container resmgr.Evaluable) bool {
	if len(spec.Scope) == 0 {
		return true
	}
	for _, s := range spec.Scope {
		if s.IsContainerInScope(container) {
			return true
		}
	}
	return false
}

// Compare checks if this spec is identical to another.
func (spec *AdjustmentSpec) Compare(other *AdjustmentSpec) bool {
	switch {
	case !CompareScopes(spec.Scope, other.Scope):
		return false
	case !spec.compareResources(other):
		return false
	case !spec.Classes.Compare(other.Classes):
		return false
	case spec.ToptierLimit == nil && other.ToptierLimit != nil:
		return false
	case spec.ToptierLimit != nil && other.ToptierLimit == nil:
		return false
	case spec.ToptierLimit != nil && spec.ToptierLimit.Value() != other.ToptierLimit.Value():
		return false
	}
	return true
}

// Verify checks the given spec for obvious errors.
func (spec *AdjustmentSpec) Verify() error {
	if err := spec.verifyResources(); err != nil {
		return err
	}
	if err := spec.verifyToptierLimit(); err != nil {
		return err
	}

	return nil
}

// Check if the resources in this spec are identical to another one.
func (spec *AdjustmentSpec) compareResources(other *AdjustmentSpec) bool {
	switch {
	case spec == nil && other == nil:
		return true
	case spec != nil && other == nil:
		return true
	case spec == nil && other != nil:
		return true
	case spec.Resources == nil && other.Resources == nil:
		return true
	case spec.Resources != nil && other.Resources == nil:
		return false
	case spec.Resources == nil && other.Resources != nil:
		return false
	}

	r := *spec.Resources
	o := *other.Resources

	if len(r.Requests) != len(o.Requests) {
		return false
	}
	if len(r.Limits) != len(o.Limits) {
		return false
	}
	for name, qty := range r.Requests {
		oqty, ok := o.Requests[name]
		if !ok || qty.Cmp(oqty) != 0 {
			return false
		}
	}
	for name, qty := range r.Limits {
		oqty, ok := o.Limits[name]
		if !ok || qty.Cmp(oqty) != 0 {
			return false
		}
	}

	return true
}

// verifyResources verifies the resource requirements of this spec.
func (spec *AdjustmentSpec) verifyResources() error {
	if spec.Resources == nil {
		return nil
	}

	r := *spec.Resources
	if r.Requests == nil {
		r.Requests = corev1.ResourceList{}
	}
	if r.Limits == nil {
		r.Limits = corev1.ResourceList{}
	}

	req, rok := r.Requests[corev1.ResourceCPU]
	lim, lok := r.Limits[corev1.ResourceCPU]
	switch {
	case !rok && lok:
		r.Requests[corev1.ResourceCPU] = lim
	case rok && lok:
		if lim.Cmp(req) < 0 {
			return apiError("invalid CPU limit %q < request %q", lim, req)
		}
	}

	req, rok = r.Requests[corev1.ResourceMemory]
	lim, lok = r.Limits[corev1.ResourceMemory]
	switch {
	case !rok && lok:
		r.Requests[corev1.ResourceMemory] = lim
	case rok && lok:
		if lim.Cmp(req) < 0 {
			return apiError("invalid memory limit %q < request %q", lim, req)
		}
	}

	for name := range r.Requests {
		switch name {
		case corev1.ResourceCPU, corev1.ResourceMemory:
		default:
			return apiError("invalid resource requests: unsupported resource %v", name)
		}
	}

	for name := range r.Limits {
		switch name {
		case corev1.ResourceCPU, corev1.ResourceMemory:
		default:
			return apiError("invalid resource limits: unsupported resource %v", name)
		}
	}

	return nil
}

// verifyToptierLimit verifies the top tier memory limit settings of this spec.
func (spec *AdjustmentSpec) verifyToptierLimit() error {
	if spec.ToptierLimit == nil {
		return nil
	}

	l := spec.ToptierLimit.Value()
	if l < 0 {
		return apiError("invalid ToptierLimit %v", l)
	}

	return nil
}

// IsNodeInScope tests if the node is within this scope.
func (scope *AdjustmentScope) IsNodeInScope(node string) bool {
	if len(scope.Nodes) == 0 {
		return true
	}
	for _, n := range scope.Nodes {
		if matches(n, node) {
			return true
		}
	}
	return false
}

// IsContainerInScope tests if the container is within this scope.
func (scope *AdjustmentScope) IsContainerInScope(container resmgr.Evaluable) bool {
	if len(scope.Containers) == 0 {
		return true
	}
	for _, expr := range scope.Containers {
		if expr.Evaluate(container) {
			return true
		}
	}
	return false
}

// match a string against a primitive pattern with a single optional trailing '*'.
func matches(pattern, name string) bool {
	if pattern == "" {
		return true
	}
	if !strings.HasSuffix(pattern, "*") {
		return pattern == name
	}
	return strings.HasPrefix(name, pattern[0:len(pattern)-1])
}

// CompareScopes checks if two slices of scopes are (syntactically) identical.
func CompareScopes(scopes []AdjustmentScope, others []AdjustmentScope) bool {
	if len(scopes) != len(others) {
		return false
	}
	for idx, s := range scopes {
		o := others[idx]
		if !s.Compare(&o) {
			return false
		}
	}
	return true
}

// Compare check if the scope is identical to another one.
func (scope *AdjustmentScope) Compare(other *AdjustmentScope) bool {
	if len(scope.Nodes) != len(other.Nodes) || len(scope.Containers) != len(other.Containers) {
		return false
	}
	for idx, n := range scope.Nodes {
		if other.Nodes[idx] != n {
			return false
		}
	}
	for idx, c := range scope.Containers {
		if other.Containers[idx] != c {
			return false
		}
	}
	return true
}

// Compare checks if the classes are identical to another set.
func (c *Classes) Compare(o *Classes) bool {
	switch {
	case c == nil && o == nil:
		return true
	case c != nil && o == nil, c == nil && o != nil:
		return false
	case c.RDT != nil && o.RDT == nil, c.RDT == nil && o.RDT != nil:
		return false
	case c.BlockIO != nil && o.BlockIO == nil, c.BlockIO == nil && o.BlockIO != nil:
		return false
	case c.RDT == nil && c.BlockIO == nil:
		return true
	}
	return *c.RDT == *o.RDT && *c.BlockIO == *o.BlockIO
}

// apiError returns a format error specific to this API.
func apiError(format string, args ...interface{}) error {
	return fmt.Errorf("adjustment API error: "+format, args...)
}
