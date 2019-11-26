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

package rdt

import (
	"flag"
	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/rdt"
)

// options captures our configurable parameters.
type options struct {
	// ResctrlPath is the mount point of the resctrl pseudo-filesystem.
	ResctrlPath string `json:",omitempty"`
	// Class is a assigned to actual RDT class map.
	Classes map[string]string `json:",omitempty"`
}

// Our runtime configuration.
var opt = defaultOptions().(*options)
var defaultPath string

// resctrlPath returns the default path for the resctrl pseudo-filesystem mount point.
func resctrlPath() string {
	if defaultPath != "" {
		return defaultPath
	}
	path, _ := rdt.ResctrlMountPath()
	return path
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{
		ResctrlPath: resctrlPath(),
		Classes:     make(map[string]string),
	}
}

// Register command line options and for configuration handling.
func init() {
	flag.StringVar(&defaultPath, "resctrl-path", resctrlPath(),
		"Path of the resctrl filesystem mountpoint")

	config.Register("resource-manager.rdt", configHelp, opt, defaultOptions,
		config.WithNotify(getRDTController().(*rdtctl).configNotify))
}
