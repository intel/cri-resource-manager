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

package blockio

import (
	"github.com/intel/cri-resource-manager/pkg/config"
)

// options captures our configurable parameters.
type options struct {
	// Classes assigned to actual blockio classes, for example Guaranteed -> HighPrioNoThrottling.
	Classes map[string]string `json:",omitempty"`
}

// Our runtime configuration.
var opt = defaultOptions().(*options)

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{
		Classes: make(map[string]string),
	}
}

// init registers blockio class mapping configuration.
func init() {
	config.Register(ConfigModuleName, configHelp, opt, defaultOptions)
}
