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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	// DefaultPolicy is the name of the default policy to use.
	DefaultPolicy = NullPolicy
	// NullPolicy is the reserved name for disabling policy altogether.
	NullPolicy = "null"

	// Flag for selecting the active policy.
	optionPolicy = "policy"
	// Flag for specifying per hardware domain resource availability constraints.
	optionAvailable = "available-resources"
	// Flag for specifying per hardware domain resource reservations for kube/system.
	optionReserved = "reserved-resources"
	// Flag for listing the available policies.
	optionListPolicies = "list-policies"
)

// Policy options configurable via the command line.
type options struct {
	policy    string                    // active policy
	policies  map[string]Implementation // registered policies
	available Constraint                // resource availability
	reserved  Constraint                // resource reservations
}

// Policy options with their defaults.
var opt = options{
	policy:    DefaultPolicy,
	policies:  make(map[string]Implementation),
	available: Constraint{},
	reserved:  Constraint{},
}

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

func (o *options) setConstraint(c Constraint, constraint string) error {
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

func (o *options) getConstraint(c Constraint) string {
	constraint := ""
	sep := ""
	for domain := range c {
		constraint += sep + c.String(domain)
		sep = ","
	}
	return constraint
}

func (o *options) Set(name, value string) error {
	switch name {
	case optionPolicy:
		o.policy = value
	case optionAvailable:
		for _, constraint := range strings.Split(value, ",") {
			if err := o.setConstraint(o.available, constraint); err != nil {
				return policyError("invalid availability constraint: %v", err)
			}
		}
	case optionReserved:
		for _, constraint := range strings.Split(value, ",") {
			if err := o.setConstraint(o.reserved, constraint); err != nil {
				return policyError("invalid reservation constraint: %v", err)
			}
		}
	case optionListPolicies:
		fmt.Printf("The available policies are:\n")
		for name, impl := range opt.policies {
			fmt.Printf("  %s: %s\n", name, impl.Description())
		}
		os.Exit(0)

	default:
		return fmt.Errorf("can't set unknown policy option '%s'", name)
	}

	return nil
}

func (o *options) Get(name string) string {
	switch name {
	case optionPolicy:
		return o.policy
	case optionAvailable:
		return o.getConstraint(o.available)
	case optionReserved:
		return o.getConstraint(o.reserved)
	default:
		return fmt.Sprintf("<no default for unknown policy option '%s'>", name)
	}
}

type wrappedOption struct {
	name   string
	opt    *options
	isBool bool
}

func (wo *wrappedOption) IsBoolFlag() bool {
	return wo.isBool
}

func wrapOption(name, usage string) (flag.Value, string, string) {
	return wrappedOption{name: name, opt: &opt}, name, usage
}

func (wo wrappedOption) Name() string {
	return wo.name
}

func (wo wrappedOption) Set(value string) error {
	return wo.opt.Set(wo.Name(), value)
}

func (wo wrappedOption) String() string {
	if wo.isBool {
		return ""
	}
	return wo.opt.Get(wo.Name())
}

func wrapBoolean(name, usage string) (flag.Value, string, string) {
	return &wrappedOption{name: name, opt: &opt, isBool: true}, name, usage
}

func init() {
	flag.Var(wrapOption(optionPolicy,
		"select the policy to use for hardware resource decision making.\n"+
			"You can list the available policies with the --"+optionListPolicies+" option."))
	flag.Var(wrapOption(optionAvailable,
		"specify the amount of resources available for allocation by the active policy."))
	flag.Var(wrapOption(optionReserved,
		"specify the amount of resources reserved for system- and kube-tasks."))
	flag.Var(wrapBoolean(optionListPolicies,
		"list the available resource management policies."))
}
