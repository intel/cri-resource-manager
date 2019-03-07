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
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// Flag for specifying a configuration file.
	optionConfigFile = "config"
)

// Options disallowed in a configuration file.
var forbiddenOptions = map[string]struct{}{
	// To trivially avoid config-include cycles, disallow nested configs altogether.
	optionConfigFile: {},
}

// Our logger instance.
var log = logger.NewLogger("config")

type options struct {
	file string
}

// Split a configuration file entry into a (fully specified) option and value.
func (o *options) splitEntry(line string) (string, string) {
	optval := strings.SplitN(line, "=", 2)

	if len(optval) > 1 {
		return strings.Trim(optval[0], " \t"), strings.Trim(optval[1], " \t")
	}

	return strings.Trim(optval[1], " \t"), ""
}

// Parse the given configuration file.
func (o *options) parseConfigFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return configError("failed to parse configuration file '%s': %v", path, err)
	}
	defer file.Close()

	log.Debug("parsing configuration file '%s'...", path)

	scan := bufio.NewScanner(file)
	cmdline := []string{}
	prefix := ""

	//
	// Parse the configuration file line by line, filtering comments (^[ \t]*#.*$),
	// and empty lines (^[ \t]*$). All the remaining unfiltered lines are expected
	// to be
	//
	//   - self-contained configuration entries (^[^ \t].*$), or
	//   - configuration sub-entries
	//
	// These are used to construct a pseudo-commandline which is then parsed using
	// the stock flag package. The pseudo commandline is constructed by prefixing
	// self-contained (IOW unindented) entries with '--', and sub-entries with
	// '--<prefix>-', where <prefix> is the previous self-contained entry.
	//

	for scan.Scan() {
		line := scan.Text()

		if line == "" {
			continue
		}

		subopt := line[0] == ' ' || line[0] == '\t'
		line = strings.Trim(line, " \t")

		if line == "" || line[0] == '#' {
			continue
		}

		if subopt && prefix == "" {
			return configError("invalid configuration entry '%s' in file '%s',"+
				"indented entry without previous option", line, path)

		}

		optval := strings.SplitN(line, "=", 2)
		opt := strings.Trim(optval[0], " \t")

		if _, forbidden := forbiddenOptions[opt]; forbidden {
			return configError("option '%s' is not allowed in a configuration file", opt)
		}

		if subopt {
			cmdline = append(cmdline, prefix+"-"+opt)
		} else {
			prefix = "--" + opt
			cmdline = append(cmdline, prefix)
		}

		if len(optval) > 1 {
			cmdline = append(cmdline, strings.Trim(optval[1], " \t"))
		}
	}

	// dig out the remaining commandline options, we'll need to resume parsing
	remaining := os.Args[1:]
	for idx, arg := range remaining {
		if arg == "--"+optionConfigFile || arg == "-"+optionConfigFile {
			if idx < len(remaining)-1 && remaining[idx+1] == path {
				remaining = remaining[idx+2:]
			}
		}
	}

	log.Debug("parsing constructed pseudo-commandline '%s'", strings.Join(cmdline, " "))
	log.Debug("commandline to resume after file: '%s'", strings.Join(remaining, " "))

	// parse the contents of the config file
	flag.CommandLine.Init(fmt.Sprintf("configuration file '%s'", path), flag.ExitOnError)
	flag.CommandLine.Parse(cmdline)
	if len(flag.CommandLine.Args()) > 0 {
		log.Fatal("unknown arguments in configuration file '%s': %s", path,
			strings.Join(flag.CommandLine.Args(), ","))
	}

	// resume parsing the remaining of the real command line
	flag.CommandLine.Init("", flag.ExitOnError)
	flag.CommandLine.Parse(remaining)

	return nil
}

// configError produces a formatted error related to parsing configuration files.
func configError(format string, args ...interface{}) error {
	return fmt.Errorf("config: "+format, args...)
}

func (o *options) Set(name, value string) error {
	switch name {
	case optionConfigFile:
		if abs, err := filepath.Abs(value); err != nil {
			value = abs
		}
		o.file = filepath.Clean(value)
		if err := o.parseConfigFile(value); err != nil {
			log.Fatal("failed to parse configuration file '%s': %v", value, err)
		}
	default:
		return fmt.Errorf("can't set unknown policy option '%s'", name)
	}

	return nil
}

func (o *options) Get(name string) string {
	switch name {
	case optionConfigFile:
		return o.file
	default:
		return fmt.Sprintf("<no default for unknown policy option '%s'>", name)
	}
}

type wrappedOption struct {
	name   string
	opt    *options
	isBool bool
}

func (wo *wrappedOption) IsBoolFlag() bool {
	return wo.isBool
}

func wrapOption(name, usage string) (flag.Value, string, string) {
	return wrappedOption{name: name, opt: &opt}, name, usage
}

func (wo wrappedOption) Name() string {
	return wo.name
}

func (wo wrappedOption) Set(value string) error {
	return wo.opt.Set(wo.Name(), value)
}

func (wo wrappedOption) String() string {
	if wo.isBool {
		return ""
	}
	return wo.opt.Get(wo.Name())
}

func wrapBoolean(name, usage string) (flag.Value, string, string) {
	return &wrappedOption{name: name, opt: &opt, isBool: true}, name, usage
}

var opt = options{}

func init() {
	flag.Var(wrapOption(optionConfigFile, "Configuration file to use."))
}
