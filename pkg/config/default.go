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

package config

import (
	"flag"
	"os"
	"path/filepath"
)

// GetModule returns the named module for the default runtime configuration collection.
func GetModule(name string) *Module {
	return DefaultConfig().GetModule(name)
}

// Notify notifies all notifiers of the default runtime configuration collection.
func Notify(event Event, source Source) error {
	return DefaultConfig().Notify(event, source)
}

// Reset resets all the default runtime configuration collection to its defaults.
func Reset() error {
	return DefaultConfig().Reset()
}

// Backup returns a snapshot of the default runtime configuration collection.
func Backup() *Snapshot {
	return DefaultConfig().Backup()
}

// Restore restores a previous snapshot to the default runtime configuration collection.
func Restore(s *Snapshot, operation string) error {
	return DefaultConfig().Restore(s, operation)
}

// ParseArgList updates the default runtime configuration collection parsing the given arguments.
func ParseArgList(args []string, source Source, extra *flag.FlagSet) error {
	return DefaultConfig().ParseArgList(args, source, extra)
}

// ParseCmdline updates the default runtime configuration collection parsing the command line.
func ParseCmdline() error {
	return DefaultConfig().ParseCmdline()
}

// ParseYAMLFile updates the default runtime configuration collection parsing the given file.
func ParseYAMLFile(path string) error {
	return DefaultConfig().ParseYAMLFile(path)
}

// ParseYAMLData updates the default runtime configuration collection parsing the given data.
func ParseYAMLData(raw []byte, source Source) error {
	return DefaultConfig().ParseYAMLData(raw, source)
}

// Usage prints help on usage of the default runtime configuration collection.
func Usage() {
	DefaultConfig().Usage()
}

// Help prints help on usage of the default runtime configuration collection.
func Help(args ...string) {
	DefaultConfig().Help(args...)
}

// Args returns the non-flag arguments passed to the default runtime configuration collection.
func Args() []string {
	return DefaultConfig().Args()
}

// DefaultConfig returns the default runtime configuration collection.
func DefaultConfig() *Config {
	if c, ok := configs[DefaultRuntimeConfig]; ok {
		return c
	}

	binary := filepath.Clean(os.Args[0])
	return NewConfig(DefaultRuntimeConfig, "runtime configuration for "+binary)
}
