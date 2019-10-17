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
	"flag"
	"strconv"

	"github.com/ghodss/yaml"
)

// Policy options configurable via the command line.
type options struct {
	// relax exclusive isolated CPU allocation criteria
	RelaxedIsolation bool     `json:"RelaxedIsolation"`
	Rdt              Tristate `json:"Rdt"`
}

type Tristate int

const (
	TristateOff = iota
	TristateOn
	TristateAuto
)

// UnmarshalJSON implements the unmarshaller function for "encoding/json"
func (t *Tristate) UnmarshalJSON(data []byte) error {
	val, err := strconv.ParseBool(string(data))
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

// Policy options with their defaults.
var opt = options{
	Rdt: TristateAuto,
}

// parseConfData parses options from a YAML data.
func parseConfData(raw []byte) (*options, error) {
	conf := &options{
		Rdt: TristateAuto, // Rdt defaults to 'auto'
	}

	if len(raw) != 0 {
		if err := yaml.Unmarshal(raw, conf); err != nil {
			return nil, policyError("failed to parse configuration data: %v", err)
		}
	}

	return conf, nil
}

// Register our command-line flags.
func init() {
	flag.BoolVar(&opt.RelaxedIsolation, PolicyName+"-policy-relaxed-isolation", false,
		"Allow allocating multiple available isolated CPUs exclusively to any single container.")
}
