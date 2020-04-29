/*
Copyright 2020 Intel Corporation

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

package blockio

import (
	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
)

// options captures our configurable parameters.
type options struct {
	// Classes define weights and throttling parameters for sets of devices.
	Classes map[string][]DevicesParameters `json:",omitempty"`
}

// DevicesParameters defines Block IO parameters for a set of devices.
type DevicesParameters struct {
	Devices           []string `json:",omitempty"`
	ThrottleReadBps   string   `json:",omitempty"`
	ThrottleWriteBps  string   `json:",omitempty"`
	ThrottleReadIOPS  string   `json:",omitempty"`
	ThrottleWriteIOPS string   `json:",omitempty"`
	Weight            string   `json:",omitempty"`
}

// Currently active set of "raw" options
var opt = defaultOptions().(*options)

// defaultOptions returns a new instance of "raw" options set to their defaults
func defaultOptions() interface{} {
	return &options{}
}

func init() {
	pkgcfg.Register(ConfigModuleName, "Block I/O class control", opt, defaultOptions)
}
