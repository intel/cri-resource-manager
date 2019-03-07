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
	"fmt"
	"strconv"

	"github.com/ghodss/yaml"
)

const (
	// Flag for relaxing exclusive isolated CPU allocation criteria.
	optionRelaxedIsolation = PolicyName + "-policy-relaxed-isolation"
)

// Policy options configurable via the command line.
type options struct {
	// relax exclusive isolated CPU allocation criteria
	RelaxedIsolation bool `json:"RelaxedIsolation"`
}

// Policy options with their defaults.
var opt = options{}

// parseConfData parses options from a YAML data.
func parseConfData(raw []byte) (*options, error) {
	conf := &options{}

	if len(raw) != 0 {
		if err := yaml.Unmarshal(raw, conf); err != nil {
			return nil, policyError("failed to parse configuration data: %v", err)
		}
	}

	return conf, nil
}

// Set the named configuration option to the given value.
func (o *options) Set(name, value string) error {
	var err error

	switch name {
	case optionRelaxedIsolation:
		o.RelaxedIsolation, err = strconv.ParseBool(value)
		if err != nil {
			return policyError("invalid boolean '%s' for option '%s': %v", name, value, err)
		}
	default:
		return policyError("unknown static policy option '%s' with value '%s'", name, value)
	}

	return nil
}

// Return the current value for the named configuration option.
func (o *options) Get(name string) string {
	switch name {
	case optionRelaxedIsolation:
		return strconv.FormatBool(o.RelaxedIsolation)
	default:
		return fmt.Sprintf("<no value for unknown static policy option '%s'>", name)
	}
}

type wrappedOption struct {
	name string
	opt  *options
}

func wrapOption(name, usage string) (wrappedOption, string, string) {
	return wrappedOption{name: name, opt: &opt}, name, usage
}

func (wo wrappedOption) Name() string {
	return wo.name
}

func (wo wrappedOption) Set(value string) error {
	return wo.opt.Set(wo.Name(), value)
}

func (wo wrappedOption) String() string {
	return wo.opt.Get(wo.Name())
}

// Register our command-line flags.
func init() {
	flag.Var(wrapOption(optionRelaxedIsolation,
		"Allow allocating multiple available isolated CPUs exclusively to any single container."))
}
