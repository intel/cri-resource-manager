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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
)

type BalloonsOptions balloonsOptionsWrapped

// BalloonsOptions contains configuration options specific to this policy.
type balloonsOptionsWrapped struct {
	// PinCPU controls pinning containers to CPUs.
	PinCPU *bool `json:"PinCPU,omitempty"`
	// PinMemory controls pinning containers to memory nodes.
	PinMemory *bool `json:"PinMemory,omitempty"`
	// IdleCpuClass controls how unusded CPUs outside any a
	// balloons are (re)configured.
	IdleCpuClass string `json:"IdleCPUClass",omitempty"`
	// ReservedPoolNamespaces is a list of namespace globs that
	// will be allocated to reserved CPUs.
	ReservedPoolNamespaces []string `json:"ReservedPoolNamespaces,omitempty"`
	// BallonDefs contains balloon type definitions.
	BalloonDefs []*BalloonDef `json:"BalloonTypes,omitempty"`
}

// BalloonDef contains a balloon definition.
type BalloonDef struct {
	// Name of the balloon definition.
	Name string `json:"Name"`
	// Namespaces control which namespaces are assigned into
	// balloon instances from this definition. This is used by
	// namespace assign methods.
	Namespaces []string `json:"Namespaces",omitempty`
	// MaxCpus specifies the maximum number of CPUs exclusively
	// usable by containers in a balloon. Balloon size will not be
	// inflated larger than MaxCpus.
	MaxCpus Limit `json:"MaxCPUs"`
	// MinCpus specifies the minimum number of CPUs exclusively
	// usable by containers in a balloon. When new balloon is created,
	// this will be the number of CPUs reserved for it even if a container
	// would request less.
	MinCpus int `json:"MinCPUs"`
	// AllocatorPriority (0: High, 1: Normal, 2: Low, 3: None)
	// This parameter is passed to CPU allocator when creating or
	// resizing a balloon. At init, balloons with highest priority
	// CPUs are allocated first.
	AllocatorPriority cpuallocator.CPUPriority `json:"AllocatorPriority"`
	// CpuClass controls how CPUs of a balloon are (re)configured
	// whenever a balloon is created, inflated or deflated.
	CpuClass string `json:"CpuClass"`
	// MinBalloons is the number of balloon instances that always
	// exist even if they would become empty. At init this number
	// of instances will be created before assigning any
	// containers.
	MinBalloons int `json:"MinBalloons"`
	// MaxBalloons is the maximum number of balloon instances that
	// is allowed to co-exist. If reached, new balloons cannot be
	// created anymore.
	MaxBalloons Limit `json:"MaxBalloons"`
	// PreferSpreadingPods: containers of the same pod may be
	// placed on separate balloons. The default is false: prefer
	// placing containers of a pod to the same balloon(s).
	PreferSpreadingPods bool
	// PreferPerNamespaceBalloon: if true, containers in different
	// namespaces are preferrably placed in separate balloons,
	// even if the balloon type is the same for all of them. On
	// the other hand, containers in the same namespace will be
	// placed in the same balloon instances. The default is false:
	// namespaces have no effect on placement.
	PreferPerNamespaceBalloon bool
	// PreferNewBalloons: prefer creating new balloons over adding
	// containers to existing balloons. The default is false:
	// prefer using filling free capacity and possibly inflating
	// existing balloons before creating new ones.
	PreferNewBalloons bool
}

var defaultPinCPU bool = true
var defaultPinMemory bool = true

// DeepCopy creates a deep copy of a BalloonsOptions
func (bo *BalloonsOptions) DeepCopy() *BalloonsOptions {
	outBo := *bo
	outBo.ReservedPoolNamespaces = make([]string, len(bo.ReservedPoolNamespaces))
	copy(outBo.ReservedPoolNamespaces, bo.ReservedPoolNamespaces)
	outBo.BalloonDefs = make([]*BalloonDef, len(bo.BalloonDefs))
	for i := range bo.BalloonDefs {
		outBo.BalloonDefs[i] = bo.BalloonDefs[i].DeepCopy()
	}
	return &outBo
}

// String stringifies a BalloonDef
func (bdef BalloonDef) String() string {
	return bdef.Name
}

// DeepCopy creates a deep copy of a BalloonDef
func (bdef *BalloonDef) DeepCopy() *BalloonDef {
	outBdef := *bdef
	outBdef.Namespaces = make([]string, len(bdef.Namespaces))
	copy(outBdef.Namespaces, bdef.Namespaces)
	return &outBdef
}

// defaultBalloonsOptions returns a new BalloonsOptions instance, all initialized to defaults.
func defaultBalloonsOptions() interface{} {
	return &BalloonsOptions{
		ReservedPoolNamespaces: []string{metav1.NamespaceSystem},
		PinCPU:                 &defaultPinCPU,
		PinMemory:              &defaultPinMemory,
	}
}

// Our runtime configuration.
var balloonsOptions = defaultBalloonsOptions().(*BalloonsOptions)

// UnmarshalJSON makes sure all options from previous unmarshals get
// cleared before unmarshaling new data to the same address.
func (bo *BalloonsOptions) UnmarshalJSON(data []byte) error {
	bow := balloonsOptionsWrapped{}
	if err := json.Unmarshal(data, &bow); err != nil {
		return err
	}
	*bo = BalloonsOptions(bow)
	return nil
}

// Register us for configuration handling.
func init() {
	pkgcfg.Register(PolicyPath, PolicyDescription, balloonsOptions, defaultBalloonsOptions)
}
