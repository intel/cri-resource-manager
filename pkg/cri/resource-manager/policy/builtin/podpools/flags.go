// Copyright 2020-2021 Intel Corporation. All Rights Reserved.
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
	PinCPU bool `json:"PinCPU,omitempty"`
	// PinMemory controls pinning containers to memory nodes.
	PinMemory bool `json:"PinMemory,omitempty"`
	// PoolDefs contains pool definitions
	PoolDefs []*PoolDef `json:"Pools,omitempty"`
}

// PoolDef contains a pool definition.
type PoolDef struct {
	// Name is the name of the pool, or name prefix of
	// multi-instance pools.
	Name string `json:"Name"`
	// CPU specifies the number of CPUs exclusively usable by
	// pods in the pool.
	CPU string `json:"CPU"`
	// MaxPods specifies the maximum number of pods assigned to
	// the pool. 0 (the default) means unlimited. -1 means no
	// pods.
	MaxPods int `json:"MaxPods"`
	// Instances specifies the number of multi-instance pools,
	// either directly or as CPU (count/percentage) reserved for
	// instances. The default is 1.
	Instances string `json:"Instances,omitempty"`
	// FillOrder specifies how multi-instance pools are filled.
	FillOrder FillOrder `json:"FillOrder"`
	// For the future: when enabling dynamic (on-demand) pool
	// instantiation, consider different ways of handling the case
	// of MaxPods>1, FillOrder==Balanced. Creating underloaded
	// pool instances will consume CPUs from other pool instances,
	// in a bad case causing workload migrations between memory
	// controllers when rearranging pool load is needed for
	// creation of new pools.
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

// MarshalJSON marshals a FillOrder as a quoted json string
func (fo FillOrder) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(fmt.Sprintf("%q", fo))
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmarshals a FillOrder quoted json string to the enum value
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

const (
	// ConfigDescription describes our configuration fragment.
	ConfigDescription = PolicyDescription // XXX TODO
)

func (o *PodpoolsOptions) Describe() string {
	return PolicyDescription
}

func (o *PodpoolsOptions) Reset() {
	*o = PodpoolsOptions{
		PinCPU:    true,
		PinMemory: true,
	}
}

func (o *PodpoolsOptions) Validate() error {
	// XXX TODO
	log.Warn("*** Implement semantic validation for %q, or remove this.", ConfigDescription)
	return nil
}

// Our runtime configuration.
var podpoolsOptions = defaultPodpoolsOptions().(*PodpoolsOptions)

// Register us for configuration handling.
func init() {
	pkgcfg.Register(PolicyPath, podpoolsOptions)
}
