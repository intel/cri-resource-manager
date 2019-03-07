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
)

// CRI resource manager command line options related to STP policy
type options struct {
	confFile        string
	confDir         string
	createNodeLabel bool
	createNodeTaint bool
}

var opt = options{}

// Register our command-line flags.
func init() {
	flag.StringVar(&opt.confFile, "static-pools-conf-file", "", "STP pool configuration file")
	flag.StringVar(&opt.confDir, "static-pools-conf-dir", "/etc/cmk", "STP pool configuration directory")
	flag.BoolVar(&opt.createNodeLabel, "static-pools-create-cmk-node-label", false, "Create CMK-related node label for backwards compatibility")
	flag.BoolVar(&opt.createNodeTaint, "static-pools-create-cmk-node-taint", false, "Create CMK-related node taint for backwards compatibility")
}
