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

package main

import (
	"github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// Option to specify a file to read configuration from.
	optConfigFile = "config"
)

// Options captures our main configuration options.
type options struct {
	configFile string // file to parse for configuration.
}

// Our configuration module and configurable options.
var cfg *config.Module
var opt = options{}

// Register us for configuration handling.
func init() {
	cfg = config.Register(config.MainModule, "main configuration")
	cfg.StringVar(&opt.configFile, optConfigFile, "", "file to read configuration from.")
}
