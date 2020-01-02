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

package dump

//
// This package implements the dumping of (gRPC) methods calls where
// each method is called with a single request struct and returns a
// single reply struct or an error. Configuring what to dump happens
// by specifying a comma-separated dump request on the command line.
//
// A dump request is a comma-separated list of dump specs:
//     <spec>[,<spec>,...,<spec>], where each spec is of the form
//     <[target:]request>
// A request is either a requests name (gRPC method name without
// the leading path), or a regexp for matching requests.
// The dump targets are: 'off', 'name', 'full', 'count' by default.
//

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	re "regexp"
	"strings"
	"sync"

	"github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// DefaultConfig is the default dump configuration.
	DefaultConfig = "full:.*,off:Version,.*List.*,.*Status.*,.*Info.*,.*Log.*,.*Reopen.*"
	// optDump is the command line option to specify what to dump.
	optDump = "dump"
	// optDumpFile is the command line option to specify an additional file to dump to.
	optDumpFile = "dump-file"
)

// verbosity defines the level of detail for a message dump.
type verbosity int

const (
	// Off suppresses dumping matching messages.
	Off verbosity = iota
	// NameOnly dumps the names of matching messages.
	NameOnly
	// Full dumps full details of matching requests and replies.
	Full
)

// Additional file to dump messages to.
var file *os.File
var fileName string

// Dumping can be configured both from the command line and through pkg/config.
// Options given on the command line change both the defaults and the runtime
// configuration. Configuration received via pkg/config only changes the runtime
// configuration. An empty runtime configuration causes fallback to defaults.

// Dumping options configurable via the command line or pkg/config.
type options struct {
	sync.Mutex
	Config   string               // last value Set()
	File     dumpFile             // file to also dump to, if set
	Debug    bool                 // log messages as debug messages
	Disabled bool                 // whether dumping is globally disabled
	methods  map[string]verbosity // method to verbosity map
	matches  map[string]verbosity // regexp-matched method verbosity map
	rules    []*rule              // regexp-based verbosity rules
}

type rule struct {
	source string
	regexp *re.Regexp
	v      verbosity
}
type dumpFile string

// Our default and runtime configuration.
var defaults = &options{}
var opt = &options{}

func (v verbosity) String() string {
	switch v {
	case Off:
		return "off"
	case NameOnly:
		return "name"
	case Full:
		return "full"
	}
	return fmt.Sprintf("<invalid verbosity %d>", v)
}

func (o *options) reset() {
	o.methods = make(map[string]verbosity)
	o.rules = []*rule{}
	o.matches = make(map[string]verbosity)
	o.Disabled = false
	o.Debug = false
}

func (o *options) Set(value string) error {
	o.Lock()
	defer o.Unlock()
	return o.set(value)
}

func (o *options) set(value string) error {
	if o != defaults { // from the command line we cumulate --dump options
		o.reset()
	} else {
		if o.methods == nil {
			o.methods = make(map[string]verbosity)
		}
		if o.matches == nil {
			o.matches = make(map[string]verbosity)
		}
	}
	prev := ""
	for _, req := range strings.Split(value, ",") {
		switch strings.ToLower(req) {
		case "enable":
			o.Disabled = false
			continue
		case "disable":
			o.Disabled = true
			continue
		case "debug":
			o.Debug = true
			continue
		case "reset":
			o.reset()
			continue
		}

		var v verbosity
		var level, method string

		split := strings.SplitN(req, ":", 2)
		switch len(split) {
		case 1:
			level = prev
			method = split[0]
		case 2:
			level = split[0]
			method = split[1]
			prev = level
		default:
			continue
		}

		switch level {
		case "off", "suppress":
			v = Off
		case "name", "short":
			v = NameOnly
		case "full", "long":
			v = Full
		default:
			return dumpError("invalid dump verbosity: '%s'", split[0])
		}

		switch {
		case method == "*":
			o.methods["*"] = v
		case strings.ContainsAny(method, ".*?+()[]|"):
			regexp, err := re.Compile(method)
			if err != nil {
				return dumpError("invalid method regexp '%s': %v", method, err)
			}
			o.rules = append(o.rules, &rule{source: method, regexp: regexp, v: v})
		default:
			o.methods[method] = v
		}
	}

	// propagate chanegs to defaults by command line options to runtime config as well
	if o == defaults {
		opt.Set(value)
	}

	o.Config = value

	return nil
}

func (o *options) String() string {
	if o == nil {
		return DefaultConfig
	}
	return o.Config
}

func (f *dumpFile) Set(value string) error {
	*f = dumpFile(value)

	// propagate chanegs to defaults by command line options to runtime config as well
	if defaults != nil && f == &defaults.File {
		opt.File = defaults.File
	}

	return nil
}

func (f *dumpFile) String() string {
	if f == nil {
		return ""
	}
	return string(*f)
}

func (o *options) verbosityOf(method string) verbosity {
	log.Debug("%s: match checking verbosity...", method)
	if v, ok := o.methods[method]; ok {
		log.Debug("  => exact match: %v", v)
		return v
	}
	if v, ok := o.matches[method]; ok {
		log.Debug("  => regexp match: %v", v)
		return v
	}
	if v, ok := o.methods["*"]; ok {
		log.Debug("  => wildcard match: %v", v)
		o.matches[method] = v
		return v
	}
	v := Off
	for _, rule := range o.rules {
		log.Debug("  - checking match rule %s...", rule.source)
		if rule.regexp.MatchString(method) {
			log.Debug("    + regexp match (%s): %v", method, rule.source)
			v = rule.v
		}
	}
	log.Debug("  => final regexp match: %v", v)
	o.matches[method] = v

	return v
}

func (o *options) MarshalJSON() ([]byte, error) {
	cfg := map[string]string{
		"Config": o.Config,
		"File":   string(o.File),
	}
	return json.Marshal(cfg)
}

func (o *options) UnmarshalJSON(raw []byte) error {
	cfg := map[string]string{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return dumpError("failed to unmarshal dump configuration: %v", err)
	}

	for key, value := range cfg {
		switch key {
		case "Config":
			if err := o.Set(value); err != nil {
				return err
			}
		case "File":
			if err := o.File.Set(value); err != nil {
				return err
			}
		}
	}

	return nil
}

// defaultOptions returns a new options instance, initialized to defaults.
func defaultOptions() interface{} {
	o := &options{}
	o.Set(defaults.String())
	o.File.Set(defaults.File.String())
	return o
}

// configNotify updates our runtime configuration.
func (o *options) configNotify(event config.Event, source config.Source) error {
	log.Info("message dumper configuration %v", event)

	checkAndScheduleDumpFileSwitch()

	log.Info(" * dumping: %s", opt.String())
	log.Info(" * dump file: %v", opt.File)
	log.Info(" * log with debug: %v", opt.Debug)

	return nil
}

// Register us for command line parsing and configuration handling.
func init() {
	defaults.Set(DefaultConfig)

	flag.Var(defaults, optDump,
		"value is a dump specification of the format [target:]message[,...].\n"+
			"The possible targets are:\n    off, short, and full")
	flag.Var(&defaults.File, optDumpFile,
		"additional file to dump messages to")

	config.Register("dump", configHelp, opt, defaultOptions,
		config.WithNotify(opt.configNotify))
}
