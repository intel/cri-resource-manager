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

package dyp

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
)

type DynamicPoolsOptions dynamicPoolsOptionsWrapped

// dynamicPoolsOptions contains configuration options specific to this policy.
type dynamicPoolsOptionsWrapped struct {
	// PinCPU controls pinning containers to CPUs.
	PinCPU *bool `json:"PinCPU,omitempty"`
	// PinMemory controls pinning containers to memory nodes.
	PinMemory *bool `json:"PinMemory,omitempty"`
	// IdleCpuClass controls how unusded CPUs outside any a
	// dynamicPool are (re)configured.
	IdleCpuClass string `json:"IdleCPUClass",omitempty"`
	// ReservedPoolNamespaces is a list of namespace globs that
	// will be allocated to reserved CPUs.
	ReservedPoolNamespaces []string `json:"ReservedPoolNamespaces,omitempty"`
	// DynamicPoolDefs contains dynamicPool type definitions.
	DynamicPoolDefs []*DynamicPoolDef `json:"DynamicPoolTypes,omitempty"`
}

// DynamicPoolDef contains a dynamicPool definition.
type DynamicPoolDef struct {
	// Name of the dynamicPool definition.
	Name string `json:"Name"`
	Namespaces []string `json:"Namespaces",omitempty`
	CpuClass   string   `json:"CpuClass"`
	// AllocatorPriority (0: High, 1: Normal, 2: Low, 3: None)
	// This parameter is passed to CPU allocator when creating or
	// resizing a dynamicPool. At init, dynamicPools with highest priority
	// CPUs are allocated first.
	AllocatorPriority cpuallocator.CPUPriority `json:"AllocatorPriority"`
}

var defaultPinCPU bool = true
var defaultPinMemory bool = true

// DeepCopy creates a deep copy of a DynamicPoolsOptions
func (dpo *DynamicPoolsOptions) DeepCopy() *DynamicPoolsOptions {
	outDpo := *dpo
	outDpo.ReservedPoolNamespaces = make([]string, len(dpo.ReservedPoolNamespaces))
	copy(outDpo.ReservedPoolNamespaces, dpo.ReservedPoolNamespaces)
	outDpo.DynamicPoolDefs = make([]*DynamicPoolDef, len(dpo.DynamicPoolDefs))
	for i := range dpo.DynamicPoolDefs {
		outDpo.DynamicPoolDefs[i] = dpo.DynamicPoolDefs[i].DeepCopy()
	}
	return &outDpo
}

// String stringifies a DynamicPoolsDef
func (dpDef DynamicPoolDef) String() string {
	return dpDef.Name
}

// DeepCopy creates a deep copy of a DynamicPoolDef
func (bdef *DynamicPoolDef) DeepCopy() *DynamicPoolDef {
	outBdef := *bdef
	outBdef.Namespaces = make([]string, len(bdef.Namespaces))
	copy(outBdef.Namespaces, bdef.Namespaces)
	return &outBdef
}

// defaultDynamicPoolsOptions returns a new DynamicPoolsOptions instance, all initialized to defaults.
func defaultDynamicPoolsOptions() interface{} {
	return &DynamicPoolsOptions{
		ReservedPoolNamespaces: []string{metav1.NamespaceSystem},
		PinCPU:                 &defaultPinCPU,
		PinMemory:              &defaultPinMemory,
	}
}

// Our runtime configuration.
var dynamicPoolsOptions = defaultDynamicPoolsOptions().(*DynamicPoolsOptions)

// UnmarshalJSON makes sure all options from previous unmarshals get
// cleared before unmarshaling new data to the same address.
func (bo *DynamicPoolsOptions) UnmarshalJSON(data []byte) error {
	bow := dynamicPoolsOptionsWrapped{}
	if err := json.Unmarshal(data, &bow); err != nil {
		return err
	}
	*bo = DynamicPoolsOptions(bow)
	return nil
}

// Register us for configuration handling.
func init() {
	pkgcfg.Register(PolicyPath, PolicyDescription, dynamicPoolsOptions, defaultDynamicPoolsOptions)
}
