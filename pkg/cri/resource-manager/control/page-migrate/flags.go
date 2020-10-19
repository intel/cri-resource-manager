// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
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

package pagemigrate

import (
	"github.com/intel/cri-resource-manager/pkg/config"
)

// options captures our configurable controller parameters.
type options struct {
	// PageScanInterval controls how much time we give containers to touch non-idle pages.
	PageScanInterval config.Duration
	// PageMoveInterval controls how often we trigger moving pages.
	PageMoveInterval config.Duration
	// MaxPageMoveCount controls how many pages we can move in a single go.
	MaxPageMoveCount uint
}

// Our runtime configuration.
var opt = defaultOptions().(*options)

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{}
}

// Register us for configuration handling.
func init() {
	config.Register(PageMigrationConfigPath, PageMigrationDescription, opt, defaultOptions)
}
