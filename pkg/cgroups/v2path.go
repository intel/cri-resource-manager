// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package cgroups

import (
	"flag"
)

var (
	// V2path is the mount point for the cgroup V2 pseudofilesystem.
	V2path string
)

func init() {
	flag.StringVar(&V2path, "cgroupv2-path", "/sys/fs/cgroup/unified",
		"Path to cgroup-v2 mountpoint")
}
