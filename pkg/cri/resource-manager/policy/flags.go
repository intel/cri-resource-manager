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
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	// NullPolicy is the reserved name for disabling policy altogether.
	NullPolicy = "null"

	// Flag for listing the available policies.
	optionListPolicies = "list-policies"
)

// Policy options configurable via the command line.
type options struct {
	policy    string                    // active policy
	policies  map[string]Implementation // registered policies
	available ConstraintSet             // resource availability
	reserved  ConstraintSet             // resource reservations
}

// Policy options with their defaults.
var opt = options{
	available: ConstraintSet{},
	reserved:  ConstraintSet{},
	policies:  make(map[string]Implementation),
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

// Set implements the Set() function of flags.Value interface
func (c *ConstraintSet) Set(value string) error {
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

// listPolicies is a helper type used for implementing the "list-policies"
// option which acts like a sub-command
type listPolicies bool

var listCmd listPolicies

func (l *listPolicies) Set(value string) error {
	fmt.Printf("The available policies are:\n")
	for name, impl := range opt.policies {
		fmt.Printf("  %s: %s\n", name, impl.Description())
	}
	os.Exit(0)
	return nil
}

func (l *listPolicies) IsBoolFlag() bool { return true }

func (l *listPolicies) String() string { return strconv.FormatBool(bool(*l)) }

func init() {
	flag.StringVar(&opt.policy, "policy", NullPolicy,
		"select the policy to use for hardware resource decision making.\n"+
			"You can list the available policies with the --"+optionListPolicies+" option.")
	flag.Var(&opt.available, "available-resources",
		"specify the amount of resources available for allocation by the active policy.")
	flag.Var(&opt.reserved, "reserved-resources",
		"specify the amount of resources reserved for system- and kube-tasks.")
	flag.Var(&listCmd, optionListPolicies,
		"list the available resource management policies.")
}
