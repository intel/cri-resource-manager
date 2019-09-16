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
	"flag"
	"fmt"
	"github.com/ghodss/yaml"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"strconv"
)

const (
	// optPrefix is our common option prefix.
	optPrefix = "topology-aware-"
	// Control whether containers are CPU-pinned using the cpuset cgroup controller.
	optPinCPU = optPrefix + "pin-cpu"
	// Control whether containers are memory-pinned using the cpuset cgroup controller.
	optPinMem = optPrefix + "pin-memory"
	// Control whether isolated CPUs are preferred for exclusive allocation.
	optPreferIsolated = optPrefix + "prefer-isolated-cpus"
	// Control whether shared CPU allocation is preferred over exclusive by default
	optPreferShared = optPrefix + "prefer-shared-cpus"
	// Provide pod-/container-based fake topology hint.
	optFakeHint = optPrefix + "fake-hint"
)

// Options captures our configurable policy parameters.
type options struct {
	PinCPU         bool `json:"PinCPU"`             // pin workloads to CPU
	PinMem         bool `json:"PinMemory"`          // pin workloads to memory
	PreferIsolated bool `json:"PreferIsolatedCPUs"` // prefer isolated CPUs for exclusive usage
	PreferShared   bool `json:"PreferSharedCPUs"`   // prefer shared CPU allocation
	Hints          map[string]system.TopologyHints
	explicit       map[string]struct{}
}

// Our configurable options with their defaults.
var opt = options{
	PinCPU:         true,
	PinMem:         true,
	PreferIsolated: true,
	PreferShared:   false,
	Hints:          make(map[string]system.TopologyHints),
	explicit:       make(map[string]struct{}),
}

func parseConfig(raw []byte) (*options, error) {
	conf := &options{}

	if len(raw) != 0 {
		if err := yaml.Unmarshal(raw, conf); err != nil {
			return nil, policyError("failed to parse configuration: %v", err)
		}
	}

	return conf, nil
}

func (o *options) Set(name, value string) error {
	var err error

	switch name {
	case optPinCPU:
		o.PinCPU, err = strconv.ParseBool(value)
	case optPinMem:
		o.PinMem, err = strconv.ParseBool(value)
	case optPreferIsolated:
		o.PreferIsolated, err = strconv.ParseBool(value)
	case optPreferShared:
		o.PreferShared, err = strconv.ParseBool(value)
	case optFakeHint:
		err = o.parseFakeHint(value)
	default:
		return policyError("unknown %s policy option '%s' with value '%s'",
			PolicyName, name, value)
	}

	if err != nil {
		return policyError("invalid value '%s' for option '%s': %v", value, name, err)
	}

	o.explicit[name] = struct{}{}

	return nil
}

func (o *options) Get(name string) string {
	switch name {
	case optPinCPU:
		return fmt.Sprintf("%v", o.PinCPU)
	case optPinMem:
		return fmt.Sprintf("%v", o.PinMem)
	case optPreferIsolated:
		return fmt.Sprintf("%v", o.PreferIsolated)
	case optPreferShared:
		return fmt.Sprintf("%v", o.PreferShared)
	case optFakeHint:
		return ""
	default:
		return fmt.Sprintf("<no value, unknown instrumentation option '%s'>", name)
	}
}

func (o *options) IsExplicit(option string) bool {
	_, explicit := o.explicit[option]
	return explicit
}

type wrappedOption struct {
	name     string
	opt      *options
	explicit bool
}

func wrapOption(name, usage string) (*wrappedOption, string, string) {
	return &wrappedOption{name: name, opt: &opt}, name, usage
}

func (wo *wrappedOption) Name() string {
	return wo.name
}

func (wo *wrappedOption) Set(value string) error {
	return wo.opt.Set(wo.Name(), value)
}

func (wo *wrappedOption) String() string {
	return wo.opt.Get(wo.Name())
}

// Register our command-line flags.
func init() {
	flag.Var(wrapOption(optPinCPU,
		"Whether container should be CPU-pinned using the cpuset cgroup controller."))
	flag.Var(wrapOption(optPinMem,
		"Whether container should be memory-pinned using the cpuset cgroup controller."))
	flag.Var(wrapOption(optPreferIsolated,
		"Try to allocate kernel-isolated CPUs for exclusive usage unless the Pod or "+
			"Container is explicitly annotated otherwise."))
	flag.Var(wrapOption(optPreferShared,
		"Allocate shared CPUs unless the Pod or Container is explicitly annotated otherwise."))
	flag.Var(wrapOption(optFakeHint,
		"A fake hint to pass to specified the pod or container."))
}
