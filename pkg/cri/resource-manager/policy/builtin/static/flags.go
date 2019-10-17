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
	config "github.com/intel/cri-resource-manager/pkg/config"
	"strconv"
)

const (
	// Control whether isolated CPUs are used for multi-CPU exclusive allocations.
	optRelaxedIsolation = "relaxed-isolation"
	// Control whether containers are assigned RDT classes.
	optRdt = "rdt"
)

// Options captures our configurable policy parameters.
type options struct {
	// relax exclusive isolated CPU allocation criteria
	RelaxedIsolation bool `json:"RelaxedIsolation"`
	// RDT class assignments: off, on, auto (use if available, disable otherwise)
	Rdt Tristate `json:"Rdt"`
}

// Our configuration module and configurable options.
var cfg *config.Module
var opt = options{
	Rdt: TristateAuto,
}

// A tristate boolean: on, off, automatically decide based on other conditions.
type Tristate int

const (
	TristateOff = iota
	TristateOn
	TristateAuto
)

// UnmarshalJSON implements the unmarshaller function for "encoding/json"
func (t *Tristate) Set(value string) error {
	val, err := strconv.ParseBool(value)
	switch {
	case err != nil:
		*t = TristateAuto
	case val == true:
		*t = TristateOn
	default:
		*t = TristateOff
	}
	return nil
}

// String returns the value of Tristate as a string
func (t *Tristate) String() string {
	switch *t {
	case TristateOff:
		return "off"
	case TristateOn:
		return "on"
	}
	return "auto"
}

// Register our command-line flags.
func init() {
	cfg = config.Register(PolicyName, "A proof-of-concept port of the static CPU Manager policy.")
	cfg.BoolVar(&opt.RelaxedIsolation, optRelaxedIsolation, false,
		"Allow allocating multiple available isolated CPUs exclusively to any single container.")
	cfg.Var(&opt.Rdt, optRdt,
		"Control whether RDT CLOS assignments are enabled.")
}
