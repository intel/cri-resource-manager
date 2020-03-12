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
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// NullPolicy is the reserved name for disabling policy altogether.
	NullPolicy = "null"
	// NullPolicyDescription is the description for the null policy.
	NullPolicyDescription = "A policy to bypass local policy processing."
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
		return policyError("failed unmarshal ConstraintSet: %v", err)
	}
	for name, value := range obj {
		switch {
		case DomainCPU.isEqual(name):
			switch value.(type) {
			case string:
				kind, val := "", ""
				split := strings.SplitN(value.(string), ":", 2)
				switch len(split) {
				case 0:
				case 1:
					kind, val = "", split[0]
				case 2:
					kind, val = split[0], split[1]
				}
				switch kind {
				case "cpuset":
					cset, err := cpuset.Parse(val)
					if err != nil {
						return policyError("failed to unmarshal cpuset constraint: %v", err)
					}
					set[DomainCPU] = cset
				case "":
					qty, err := resource.ParseQuantity(val)
					if err != nil {
						return policyError("failed to unmarshal CPU Quantity constraint: %v", err)
					}
					set[DomainCPU] = qty
				default:
					return policyError("invalid CPU constraint qualifier '%s'", kind)
				}
			case int:
				set[DomainCPU] = resource.NewMilliQuantity(int64(value.(int)), resource.DecimalSI)
			case float64:
				qty := resource.NewMilliQuantity(int64(1000.0*value.(float64)), resource.DecimalSI)
				set[DomainCPU] = *qty
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

func (d Domain) isEqual(domain interface{}) bool {
	switch domain.(type) {
	case string:
		return strings.ToLower(string(d)) == strings.ToLower(domain.(string))
	case Domain:
		return strings.ToLower(string(d)) == strings.ToLower(string(domain.(Domain)))
	default:
		return false
	}
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
	policies = append(policies, &AvailablePolicy{
		Name:        NullPolicy,
		Description: NullPolicyDescription,
	})
	sort.Slice(policies, func(i, j int) bool { return policies[i].Name < policies[j].Name })

	return policies
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{
		Policy:    NullPolicy,
		Available: ConstraintSet{},
		Reserved:  ConstraintSet{},
	}
}

// Register us for configuration handling.
func init() {
	config.Register("policy", "Generic policy layer.", opt, defaultOptions)
}
