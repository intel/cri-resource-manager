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

package static

import (
	"github.com/intel/cri-resource-manager/pkg/config"
	"sigs.k8s.io/yaml"
)

// Options captures our configurable policy parameters.
type options struct {
	// Relax exclusive isolated CPU allocation criteria
	RelaxedIsolation bool `json:"RelaxedIsolation"`
	// Control whether containers are assigned to RDT classes by this policy.
	Rdt Tristate `json:"Rdt"`
}

// Tristate is boolean-like value with 3 states: on, off, automatically-determined.
type Tristate int

const (
	// TristateOff is unconditional boolean false
	TristateOff = iota
	// TristateOn is unconditional boolean true
	TristateOn
	// TristateAuto indicates boolean value should be inferred using other data.
	TristateAuto
)

// Our runtime configuration.
var opt = defaultOptions().(*options)

// UnmarshalJSON implements the unmarshaller function for "encoding/json"
func (t *Tristate) UnmarshalJSON(data []byte) error {
	var value interface{}
	if err := yaml.Unmarshal(data, &value); err != nil {
		return policyError("invalid Tristate value '%s': %v", string(data), err)
	}

	switch value.(type) {
	case bool:
		*t = map[bool]Tristate{false: TristateOff, true: TristateOn}[value.(bool)]
		return nil
	case string:
		if value.(string) == "auto" {
			*t = TristateAuto
			return nil
		}
	}

	return policyError("invalid Tristate value %v of type %T", value, value)
}

// MarshalJSON implements the marshaller function for "encoding/json"
func (t Tristate) MarshalJSON() ([]byte, error) {
	switch t {
	case TristateOff:
		return []byte("false"), nil
	case TristateOn:
		return []byte("true"), nil
	case TristateAuto:
		return []byte("\"auto\""), nil
	}
	return nil, policyError("invalid tristate value %v", t)
}

// String returns the value of Tristate as a string
func (t *Tristate) String() string {
	switch *t {
	case TristateOff:
		return "false"
	case TristateOn:
		return "true"
	}
	return "auto"
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{Rdt: TristateAuto}
}

const (
	// ConfigDescription describes our configuration fragment.
	ConfigDescription = PolicyDescription // XXX TODO
)

func (o *options) Describe() string {
	return PolicyDescription
}

func (o *options) Reset() {
	*o = options{
		Rdt: TristateAuto,
	}
}

func (o *options) Validate() error {
	// XXX TODO
	log.Warn("*** Implement semantic validation for %q, or remove this.", ConfigDescription)
	return nil
}

// Register us for configuration handling.
func init() {
	config.Register(PolicyPath, opt)
}
