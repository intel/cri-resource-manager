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

package visualizer

import (
	"flag"
	"strings"
)

// dirList is a comma-separated list of directory names to search for visualizers.
type dirList string

// Set registers external visualizer implementations found in the directory list.
func (d *dirList) Set(value string) error {
	for _, dir := range strings.Split(value, ",") {
		log.Info("searching for external visualizers in %s...", dir)
		visualizers.discoverExternal(dir)
	}
	return nil
}

// String returns the given directory list as a string.
func (d dirList) String() string {
	return string(d)
}

// Register our command line options.
func init() {
	var dir dirList
	flag.Var(&dir, "external-visualizers",
		"comma-separated list of directories to search for external visualizers.")
}
