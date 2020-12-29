// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package podpools

import (
	"bytes"
	"encoding/json"
	"fmt"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
)

// PodpoolsOptions contains configuration options specific to this policy.
type PodpoolsOptions struct {
	// PinCPU controls pinning containers to CPUs.
	PinCPU bool `json:"pinCPU",omitempty`
	// PinMemory controls pinning containers to memory nodes.
	PinMemory bool `json:"pinMemory",omitempty`
	// PoolTypes contains types of pools.
	PoolTypes []*PoolType `json:"poolTypes,omitempty"`
}

// PoolType contains configuration of a pool type.
type PoolType struct {
	Name string `json:"name"`
	// TypeResource defines host resources to be consumed by this pool type.
	TypeResources PoolResources `json:"typeResources,omitempty"`
	// Resources defines resources in each pool of this type. The
	// number of pools is floor(TypeResources/Resources).
	Resources PoolResources `json:"resources"`
	// Capacity defines the (pod) capacity of each pool.
	Capacity PoolCapacity `json:"capacity"` // Capacity per pool
	// FillOrder defines how the capacity of pod pools is filled.
	FillOrder FillOrder `json:"fillOrder"`
}

// PoolResources contains resources of a pool type or a pool instance.
type PoolResources struct {
	CPU string `json:"cpu"`
}

// PoolCapacity contains the capacity of a pool instance.
type PoolCapacity struct {
	Pod int `json:"pod"`
}

// FillOrder specifies the order in which pool instances should be filled.
type FillOrder int

const (
	FillBalanced FillOrder = iota
	FillPacked
	FillFirstFree
)

var fillOrderNames = map[FillOrder]string{
	FillBalanced:  "Balanced",
	FillPacked:    "Packed",
	FillFirstFree: "FirstFree",
}

// String stringifies a FillOrder
func (fo FillOrder) String() string {
	if fon, ok := fillOrderNames[fo]; ok {
		return fon
	}
	return fmt.Sprintf("#UNNAMED-FILLORDER(%d)", int(fo))
}

// MarshalJSON marshals the enum as a quoted json string
func (fo FillOrder) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(fmt.Sprintf("%q", fo))
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmashals a quoted json string to the enum value
func (fo *FillOrder) UnmarshalJSON(b []byte) error {
	var fillOrderName string
	err := json.Unmarshal(b, &fillOrderName)
	if err != nil {
		return err
	}
	for foID, foName := range fillOrderNames {
		if foName == fillOrderName {
			*fo = foID
			return nil
		}
	}
	return podpoolsError("invalid fill order %q", fillOrderName)
}

// defaultPodpoolsOptions returns a new PodpoolsOptions instance, all initialized to defaults.
func defaultPodpoolsOptions() interface{} {
	return &PodpoolsOptions{
		PinCPU:    true,
		PinMemory: true,
	}
}

// Our runtime configuration.
var podpoolsOptions = defaultPodpoolsOptions().(*PodpoolsOptions)

// Register us for configuration handling.
func init() {
	pkgcfg.Register(PolicyPath, PolicyDescription, podpoolsOptions, defaultPodpoolsOptions)
}
