/*
Copyright 2019 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rdt

import (
	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// Set RDT confuration.
	optConfig = "config"
)

// Options captures our configurable parameters.
type options struct {
	value string
	conf  config
}

// Our configuration module and configurable options.
var cfg *pkgcfg.Module
var opt = options{}

// Set RDT configuration to the given value.
func (o *options) Set(value string) error {
	if o != nil {
		conf, err := parseConfData([]byte(value))
		if err != nil {
			return rdtError("failed to parse configuration YAML: %v", err)
		}
		o.value = value
		o.conf = conf
	}
	return nil
}

// String returns the current RDT confguration as a string.
func (o *options) String() string {
	if o != nil {
		return o.value
	}
	return ""
}

// Register us for configuratio handling.
func init() {
	cfg = pkgcfg.Register("rdt", "Generic RDT implementation module.")
	cfg.YamlVar(&opt, optConfig,
		"Read RDT configuration from the given file or set it from the given value.")
}
