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
	config "github.com/intel/cri-resource-manager/pkg/config"
)

// Options captures our configurable policy parameters.
type options struct {
	// PinCPU controls CPU pinning in this policy.
	PinCPU bool
	// PinMemory controls memory pinning in this policy.
	PinMemory bool
	// PreferIsolated controls whether isolated CPUs are preferred for isolated allocations.
	PreferIsolated bool `json:"PreferIsolatedCPUs"`
	// PreferShared controls whether shared CPU allocation is always preferred by default.
	PreferShared bool `json:"PreferSharedCPUs"`
	// ReservedPoolNamespaces is a list of namespace globs that will be allocated to reserved CPUs
	ReservedPoolNamespaces []string `json:"ReservedPoolNamespaces,omitempty"`
	// ColocatePods causes all containers in a pod to have affinity for each other.
	ColocatePods bool `json:"ColocatePods"`
	// ColocateNamespaces causes all containers in a namespace to have affinity for each other.
	ColocateNamespaces bool `json:"ColocateNamespaces"`
}

// Our runtime configuration.
var opt = defaultOptions().(*options)
var aliasOpt = defaultOptions().(*options)

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{
		PinCPU:                 true,
		PinMemory:              true,
		PreferIsolated:         true,
		PreferShared:           false,
		ReservedPoolNamespaces: []string{"kube-system"},
	}
}

// Register us for configuration handling.
func init() {
	config.Register(PolicyPath, PolicyDescription, opt, defaultOptions)
	config.Register(AliasPath, PolicyDescription, aliasOpt, defaultOptions)
}
