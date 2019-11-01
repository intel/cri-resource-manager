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

package stp

import (
	"flag"
	"github.com/intel/cri-resource-manager/pkg/config"
)

// CRI resource manager command line options related to STP policy
type options struct {
	confFile        string
	confDir         string
	createNodeLabel bool
	createNodeTaint bool
}

// conf captures our runtime configurable parameters.
type conf struct {
	// Pools defines our set of pools in use.
	Pools map[string]poolConfig `json:"pools,omitempty"`
	// LabelNode controls whether backwards-compatible CMK node label is created.
	LabelNode bool
	// TaintNode controls whether backwards-compatible CMK node taint is created.
	TaintNode bool
}

// STP policy command line options and runtime configuration with their defaults.
var opt = defaultOptions().(*options)
var cfg = defaultConfig().(*conf)

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{
		confFile:        "",
		confDir:         "/etc/cmk",
		createNodeLabel: defaultConfig().(*conf).LabelNode,
		createNodeTaint: defaultConfig().(*conf).TaintNode,
	}
}

// defaultConfig returns a new conf instance, all initialized to defaults.
func defaultConfig() interface{} {
	return &conf{
		Pools:     make(map[string]poolConfig),
		LabelNode: false,
		TaintNode: false,
	}
}

// Register us for command line option processing and configuration management.
func init() {
	flag.StringVar(&opt.confFile, "static-pools-conf-file", "", "STP pool configuration file")
	flag.StringVar(&opt.confDir, "static-pools-conf-dir", "/etc/cmk", "STP pool configuration directory")
	flag.BoolVar(&opt.createNodeLabel, "static-pools-create-cmk-node-label", false, "Create CMK-related node label for backwards compatibility")
	flag.BoolVar(&opt.createNodeTaint, "static-pools-create-cmk-node-taint", false, "Create CMK-related node taint for backwards compatibility")

	config.Register(PolicyPath, PolicyDescription, cfg, defaultConfig)
}
