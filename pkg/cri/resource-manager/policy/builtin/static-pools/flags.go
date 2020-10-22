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
	"github.com/intel/cri-resource-manager/pkg/config"
)

// conf captures our runtime configurable parameters.
type conf struct {
	// Pools defines our set of pools in use.
	Pools map[string]poolConfig `json:"pools,omitempty"`
	// ConfDirPath is the filesystem path to the legacy configuration directry structure.
	ConfDirPath string
	// ConfFilePath is the filesystem path to the legacy configuration file.
	ConfFilePath string
	// LabelNode controls whether backwards-compatible CMK node label is created.
	LabelNode bool
	// TaintNode controls whether backwards-compatible CMK node taint is created.
	TaintNode bool
}

// STP policy runtime configuration with their defaults.
var cfg = defaultConfig().(*conf)

// defaultConfig returns a new conf instance, all initialized to defaults.
func defaultConfig() interface{} {
	return &conf{
		Pools:       make(map[string]poolConfig),
		ConfDirPath: "/etc/cmk",
	}
}

// Register us for command line option processing and configuration management.
func init() {
	config.Register(PolicyPath, PolicyDescription, cfg, defaultConfig)
}
