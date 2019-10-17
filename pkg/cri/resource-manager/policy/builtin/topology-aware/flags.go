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
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
)

const (
	// Control whether containers are CPU-pinned using the cpuset cgroup controller.
	optPinCPU = "pin-cpu"
	// Control whether containers are memory-pinned using the cpuset cgroup controller.
	optPinMem = "pin-memory"
	// Control whether isolated CPUs are preferred for exclusive allocation.
	optPreferIsolated = "prefer-isolated-cpus"
	// Control whether shared CPU allocation is preferred over exclusive by default
	optPreferShared = "prefer-shared-cpus"
	// Provide pod-/container-based fake topology hint.
	optFakeHints = "fake-hints"

	optTestInt     = "int"
	optTestFloat64 = "float64"
)

// Options captures our configurable policy parameters.
type options struct {
	pinCPU         bool      // pin containers to CPU
	pinMemory      bool      // pin containers to memory
	preferIsolated bool      // prefer isolated CPUs for exclusive allocations
	preferShared   bool      // prefer shared CPU allocations by default
	hints          fakehints // fake TopologyHints (for testing)
}

// Our configuration module and configurable options.
var cfg *config.Module
var opt = options{hints: make(fakehints)}

// fakeHints is our flag.Value for per-pod or per-container faked system.TopologyHints.
type fakehints map[string]system.TopologyHints

// newFakeHints creates a new set of fake hints.
func newFakeHints() fakehints {
	return make(fakehints)
}

// merge merges the given hints to the existing set.
func (fh *fakehints) merge(hints fakehints) {
	if fh == nil {
		*fh = newFakeHints()
	}
}

// Set parses and accumulates the given hints to the existing ones.
func (h *fakehints) Set(value string) error {
	if value == "" {
		return nil
	}

	return h.parse(value)
}

// Register us for configuration handling.
func init() {
	cfg = config.Register(PolicyName, "A topology-aware container placement policy.")

	cfg.BoolVar(&opt.pinCPU, optPinCPU, true,
		"Pin containers to CPUs using the cpuset cgroup controller.")
	cfg.BoolVar(&opt.pinMemory, optPinMem, true,
		"Pin containers to memory using the cpuset cgroup controller.")
	cfg.BoolVar(&opt.preferIsolated, optPreferIsolated, true,
		"Prefer isolated CPUs for exclusive CPU allocations.")
	cfg.BoolVar(&opt.preferShared, optPreferShared, false,
		"Prefer shared CPU allocation for containers not annotated otherwise.")
	cfg.Var(&opt.hints, optFakeHints,
		"Assign fake topology hints for testing to the given pods/containers.")
}
