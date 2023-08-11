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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cgroups"
	"github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// NonePolicy is the name of our no-op policy.
	NonePolicy = "none"
	// DefaultPolicy is the name of our default policy.
	DefaultPolicy = NonePolicy
	// ConfigPath is the configuration module path for the generic policy layer.
	ConfigPath = "policy"
)

// Options captures our configurable parameters.
type options struct {
	// Policy is the name of the policy backend to activate.
	Policy string `json:"Active"`
	// Available hardware resources to use.
	Available ConstraintSet `json:"AvailableResources,omitempty"`
	// Reserved hardware resources, for system and kube tasks.
	Reserved ConstraintSet `json:"ReservedResources,omitempty"`
}

// Our runtime configuration.
var opt = defaultOptions().(*options)

// MarshalJSON implements JSON marshalling for ConstraintSets.
func (cs ConstraintSet) MarshalJSON() ([]byte, error) {
	obj := map[string]interface{}{}
	for domain, constraint := range cs {
		name := string(domain)
		switch constraint.(type) {
		case cpuset.CPUSet:
			obj[name] = "cpuset:" + constraint.(cpuset.CPUSet).String()
		case resource.Quantity:
			qty := constraint.(resource.Quantity)
			obj[name] = qty.String()
		case int:
			obj[name] = strconv.Itoa(constraint.(int))
		default:
			return nil, policyError("invalid %v constraint of type %T", domain, constraint)
		}
	}
	return json.Marshal(obj)
}

// UnmarshalJSON implements JSON unmarshalling for ConstraintSets.
func (cs *ConstraintSet) UnmarshalJSON(raw []byte) error {
	set := make(ConstraintSet)
	obj := map[string]interface{}{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return policyError("failed to unmarshal ConstraintSet: %v", err)
	}

	for name, value := range obj {
		switch strings.ToUpper(name) {
		case string(DomainCPU):
			switch v := value.(type) {
			case string:
				if err := set.parseCPU(v); err != nil {
					return err
				}
			case int:
				set.setCPUMilliQuantity(v)
			case float64:
				set.setCPUMilliQuantity(int(1000.0 * v))
			default:
				return policyError("invalid CPU constraint of type %T", value)
			}
		default:
			return policyError("internal error: unhandled ConstraintSet domain %s", name)
		}
	}

	*cs = set
	return nil
}

func (cs *ConstraintSet) String() string {
	ret := ""
	sep := ""
	for domain, value := range *cs {
		ret += sep + string(domain) + "=" + ConstraintToString(value)
		sep = ","
	}
	return ret
}

func (cs *ConstraintSet) parseCPU(value string) error {
	kind, spec := "", ""
	if sep := strings.IndexByte(value, ':'); sep != -1 {
		kind = value[:sep]
		spec = value[sep+1:]
	} else {
		spec = value
	}
	if len(spec) == 0 {
		return policyError("missing CPU constraint value")
	}

	switch {
	case kind == "cgroup" || spec[0] == '/':
		if err := cs.parseCPUFromCgroup(spec); err != nil {
			return err
		}
	case kind == "cpuset" || strings.IndexAny(spec, "-,") != -1:
		if err := cs.parseCPUSet(spec); err != nil {
			return err
		}
	case kind == "":
		if err := cs.parseCPUQuantity(spec); err != nil {
			return err
		}
	default:
		return policyError("invalid CPU constraint qualifier %q", kind)
	}

	return nil
}

func (cs *ConstraintSet) parseCPUSet(value string) error {
	cset, err := cpuset.Parse(value)
	if err != nil {
		return policyError("failed to parse CPU cpuset constraint %q: %v",
			value, err)
	}
	(*cs)[DomainCPU] = cset
	return nil
}

func (cs *ConstraintSet) parseCPUQuantity(value string) error {
	qty, err := resource.ParseQuantity(value)
	if err != nil {
		return policyError("failed to parse CPU Quantity constraint %q: %v",
			value, err)
	}
	(*cs)[DomainCPU] = qty
	return nil
}

func (cs *ConstraintSet) parseCPUFromCgroup(dir string) error {
	pathToCpuset := func(outPath *string, fragments ...string) bool {
		*outPath = filepath.Join(filepath.Join(fragments...), "cpuset.cpus")
		_, err := os.Stat(*outPath)
		return !errors.Is(err, os.ErrNotExist)
	}
	path := ""
	switch {
	case len(dir) == 0:
		return policyError("empty CPU cgroup constraint")
	case dir[0] == '/' && pathToCpuset(&path, dir):
		// dir is a direct, absolute path to an existing cgroup
	case pathToCpuset(&path, cgroups.GetMountDir(), dir):
		// dir is a relative path starting from the cgroup mount point
	case pathToCpuset(&path, cgroups.Cpuset.Path(), dir):
		// dir is a relative path starting from the cpuset controller (cgroup v1)
	default:
		// dir is none of the previous
		return policyError("failed to find cpuset.cpus for CPU cgroup constraint %q", dir)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return policyError("failed read CPU cpuset cgroup constraint %q: %v",
			path, err)
	}
	cpus := strings.TrimSuffix(string(bytes), "\n")
	cset, err := cpuset.Parse(cpus)
	if err != nil {
		return policyError("failed to parse cpuset cgroup constraint %q: %v",
			cpus, err)
	}
	(*cs)[DomainCPU] = cset
	return nil
}

func (cs *ConstraintSet) setCPUMilliQuantity(value int) {
	qty := resource.NewMilliQuantity(int64(value), resource.DecimalSI)
	(*cs)[DomainCPU] = *qty
}

// AvailablePolicy describes an available policy.
type AvailablePolicy struct {
	// Name is the name of the policy.
	Name string
	// Description is a short description of the policy.
	Description string
}

// AvailablePolicies returns the available policies and their descriptions.
func AvailablePolicies() []*AvailablePolicy {
	policies := make([]*AvailablePolicy, 0, len(backends)+1)
	for name, be := range backends {
		policies = append(policies, &AvailablePolicy{
			Name:        name,
			Description: be.description,
		})
	}
	sort.Slice(policies, func(i, j int) bool { return policies[i].Name < policies[j].Name })

	return policies
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{
		Policy:    DefaultPolicy,
		Available: ConstraintSet{},
		Reserved:  ConstraintSet{},
	}
}

// Register us for configuration handling.
func init() {
	config.Register(ConfigPath, "Generic policy layer.", opt, defaultOptions,
		config.WithNotify(configNotify))
}
