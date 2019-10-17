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
	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// Control configuration file or data to use.
	optConfig = "config"
	// Control configuration directory to use.
	optConfDir = "conf-dir"
	// Control whether legacy CMK node labels are set.
	optNodeLabel = "create-cmk-node-label"
	// Control whether legacy CMK node taints are set.
	optNodeTaint = "create-cmk-node-taint"
)

// CRI resource manager command line options related to STP policy
type options struct {
	confDir         string
	createNodeLabel bool
	createNodeTaint bool
	conf            yamlConf
}

var opt = options{}
var cfg *pkgcfg.Module

// yamlConf is our flag.Value for configuration.
type yamlConf struct {
	value string // configuration as a string
	conf  config // parsed configuration
}

// Set STP configuration data.
func (yc *yamlConf) Set(value string) error {
	if yc == nil || value == "" {
		return nil
	}

	conf, err := parseConfData([]byte(value))
	if err != nil {
		return stpError("failed to parse configuration: %v", err)
	}
	yc.value = value
	yc.conf = *conf

	return nil
}

// String returns STP configuration data as a string.
func (yc *yamlConf) String() string {
	if yc == nil {
		return ""
	}
	return yc.value
}

// Register our command-line flags.
func init() {
	cfg = pkgcfg.Register(PolicyName, PolicyDescription)

	cfg.YamlVar(&opt.conf, optConfig, "STP pool configuration file")
	cfg.StringVar(&opt.confDir, optConfDir, "/etc/cmk", "STP pool configuration directory")
	cfg.BoolVar(&opt.createNodeLabel, optNodeLabel, false,
		"Create CMK-related node label for backwards compatibility")
	cfg.BoolVar(&opt.createNodeTaint, optNodeTaint, false,
		"Create CMK-related node taint for backwards compatibility")
}
