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

//
// This module lets one tag built binaries with version metadata.
//
// Currently two pieces of metadata tracked/provided:
//   - Version: version number, by convention one provided by 'git describe'
//   - Build:   build id, by convention the git SHA1 the binary has been built from.
//
// To enable automatic versioning metadata for your binary, you need to
//
//   1) import this package
//   2) add the linker flags to override the dummy package variables, for instance:
//        LDFLAGS=-ldflags \
//          "-X=github.com/intel/cri-resource-manager/pkg/version.Version=<version> \
//           -X=github.com/intel/cri-resource-manager/pkg/version.Build=<build-id>"
//
// Note that further metadata can be trivially added in a similar fashion:
//
//   1) add the corresponding variables to this modules
//   2) arrange the default values to be correctly overridden during linking
//   3) add printing of the new metadata to PrintVersionInfo()
//

package version

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Default values of variables we'll override with the linker.
var (
	// Version is our version as given by 'git describe'.
	Version = "<If you see this, you ain't doin' it right, Jimbo...>"
	// Build is the SHA1 of the repository we've been built from.
	Build = "<If you see this, you ain't doin' it right, Jimbo...>"
)

// PrintVersionInfo prints version information about this binary.
func PrintVersionInfo() {
	fmt.Printf("%s version information:\n", filepath.Base(os.Args[0]))
	fmt.Printf("  - version: %s\n", Version)
	fmt.Printf("  - build:   %s\n", Build)
}

// Dummy struct used to hook into flag.Value.Set of -version during commandline parsing.
type version struct{}

// IsBoolFlag tell flag that we only have optional arguments.
func (version) IsBoolFlag() bool {
	return true
}

// Set is our dummy flag.Value setter.
func (version) Set(value string) error {
	print, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if print {
		PrintVersionInfo()
		os.Exit(0)
	}

	return nil
}

// String is our dummy flag.Value stringification function.
func (*version) String() string {
	return "false"
}

// Put in place a '--version' command line option for us.
func init() {
	flag.Var(&version{}, "version", "Print version information about "+filepath.Base(os.Args[0]))
}
