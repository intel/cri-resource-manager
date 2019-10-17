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

package policy

import (
	config "github.com/intel/cri-resource-manager/pkg/config"

	"io/ioutil"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	// Control which policy backend gets activated.
	optPolicy = "activate"
	// Control the amount of available resources passed to the active policy.
	optAvailableResources = "available-resources"
	// Control the amount of resources reserved for system- and kube-tasks.
	optReservedResources = "reserved-resources"
	// NullPolicy is the name of the "null" policy which prevents any policy initialization.
	NullPolicy = "null"
)

// Options captures the configurable parameters of the policy layer.
type options struct {
	policy    string                    // active policy
	policies  map[string]Implementation // registered policies
	available ConstraintSet             // resource availability
	reserved  ConstraintSet             // resource reservations
}

// Our configuration module and configurable options.
var cfg *config.Module
var opt = options{}

//
// XXX TODO: write missing constraint parsers as needed...
//

func parseCPUConstraint(cpus string) (interface{}, error) {
	switch {
	case strings.Contains(cpus, ":"):
		split := strings.Split(cpus, ":")
		qualifier, value := split[0], split[1]

		switch qualifier {
		case "cpuset":
			return parseCpuset(cpus, value)

		case "quantity":
			return parseCPUQuantity(cpus, value)

		case "cgroup":
			return parseCgroupCpuset(cpus, value)
		}

		return nil, policyError("invalid qualifier '%s' cpu constraint '%s'",
			qualifier, cpus)

	case strings.ContainsAny(cpus, "-,"):
		return parseCpuset(cpus, cpus)

	case cpus[0] == '#':
		return parseCpuset(cpus, cpus[1:])

	case cpus[0] == '/':
		return parseCgroupCpuset(cpus, cpus)

	default:
		return parseCPUQuantity(cpus, cpus)
	}
}

func parseCPUQuantity(constraint, value string) (interface{}, error) {
	qty, err := resource.ParseQuantity(value)
	if err != nil {
		return nil, policyError("invalid cpu quantity '%s' in constraint '%s': %v",
			value, constraint, err)
	}
	return qty, nil
}

func parseCpuset(constraint, value string) (interface{}, error) {
	cset, err := cpuset.Parse(value)
	if err != nil {
		return nil, policyError("invalid cpuset '%s' in constraint '%s': %v",
			value, constraint, err)
	}
	return cset, nil
}

func parseCgroupCpuset(constraint, value string) (interface{}, error) {
	if !strings.HasSuffix(value, "cpuset.cpus") {
		value = filepath.Join(value, "cpuset.cpus")
	}

	data, err := ioutil.ReadFile(value)
	if err != nil {
		return nil, policyError("can't read constraint ('%s' )cpuset file '%s': %v",
			constraint, value, err)
	}
	return parseCpuset(constraint, string(data))
}

func parseMemConstraint(mems string) (interface{}, error) {
	return mems, nil
}

func parseHugePagesConstraint(hp string) (interface{}, error) {
	return hp, nil
}

func parseCacheConstraint(c string) (interface{}, error) {
	return c, nil
}

func parseMemBWConstraint(mbw string) (interface{}, error) {
	return mbw, nil
}

// Set implements the Set() function of flags.Value interface
func (c *ConstraintSet) Set(value string) error {
	if value == "" {
		return nil
	}

	for _, constraint := range strings.Split(value, ",") {
		if err := (*c).setConstraint(constraint); err != nil {
			return policyError("invalid constraint: %v", err)
		}
	}
	return nil
}

// setConstraint sets a single constraint
func (c ConstraintSet) setConstraint(constraint string) error {
	var err error
	var domain, value string

	domval := strings.Split(constraint, "=")
	if len(domval) != 2 {
		return policyError("invalid constraint '%s' (not of form: domain=value)", constraint)
	}

	domain, value = domval[0], domval[1]

	switch domain {
	case string(DomainCPU):
		c[DomainCPU], err = parseCPUConstraint(value)
	case string(DomainMemory):
		c[DomainMemory], err = parseMemConstraint(value)
	case string(DomainHugePage):
		c[DomainHugePage], err = parseHugePagesConstraint(value)
	case string(DomainCache):
		c[DomainCache], err = parseCacheConstraint(value)
	case string(DomainMemoryBW):
		c[DomainMemoryBW], err = parseMemBWConstraint(value)
	default:
		err = policyError("unknown resource domain in constraint %s=%s", domain, value)
	}

	return err
}

func (c *ConstraintSet) String() string {
	ret := ""
	sep := ""
	for domain, value := range *c {
		ret += sep + string(domain) + "=" + ConstraintToString(value)
		sep = ","
	}
	return ret
}

// Register us for configuration handling.
func init() {
	opt = options{
		available: ConstraintSet{},
		reserved:  ConstraintSet{},
		policies:  make(map[string]Implementation),
	}

	cfg = config.Register("policy", "Implementation-agnostic policy layer.")

	cfg.StringVar(&opt.policy, optPolicy, NullPolicy,
		"Selects the policy backend to use for decision making.")
	cfg.Var(&opt.available, optAvailableResources,
		"Specify the amount of resources available for the active policy to allocate.")
	cfg.Var(&opt.reserved, optReservedResources,
		"Specify the amount of resources reserved for system- and kube-tasks.")
}
