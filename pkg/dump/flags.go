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
	"fmt"
	re "regexp"
	"strings"

	"github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// DefaultConfig is the default dump configuration.
	DefaultConfig = "off:.*,short:((Create)|(Start)|(Run)|(Update)|(Stop)|(Remove)).*,off:.*Image.*"
)

// Dumping options configurable via the command line or pkg/config.
type options struct {
	Debug    bool    // log messages as debug messages
	Disabled bool    // whether dumping is globally disabled
	File     string  // file to also dump to, if set
	Config   string  // dumping configuration
	rules    ruleset // corresponding dumping rules
}

// ruleset is an ordered set of dumping rules.
type ruleset []*rule

// rule is a single dumping rule, declaring verbosity of a single or a set of methods.
type rule struct {
	method string     // method, '*' wildcard, or regexp matching a set of methods
	regexp *re.Regexp // compiled regexp, if applicable
	detail level      // dumping verbosity
}

// level describes the level of detail to dump.
type level int

const (
	// Off suppresses dumping of matching methods
	Off level = iota
	// Name dumps only success/failure status of matching methods.
	Name
	// Full dumps matching methods with full level of detail.
	Full
)

// Our runtime configuration.
var opt = defaultOptions().(*options)

// parse parses the given string into a ruleset.
func (set *ruleset) parse(value string) error {
	prev := Full
	for _, spec := range strings.Split(value, ",") {
		r := &rule{}
		split := strings.SplitN(spec, ":", 2)
		switch len(split) {
		case 1:
			r.detail = prev
			r.method = split[0]
		case 2:
			switch strings.ToLower(split[0]) {
			case "off", "suppress":
				r.detail = Off
			case "name", "short":
				r.detail = Name
			case "full", "verbose":
				r.detail = Full
			default:
				return dumpError("invalid dump level '%s'", split[0])
			}
			r.method = split[1]
			prev = r.detail
		}

		if strings.ContainsAny(r.method, ".*?+()[]|") && r.method != "*" {
			regexp, err := re.Compile(r.method)
			if err != nil {
				return dumpError("invalid dump method regexp '%s': %v", r.method, err)
			}
			r.regexp = regexp
		}
		*set = append(*set, r)
	}

	return nil
}

// String returns the ruleset as a string.
func (set *ruleset) String() string {
	if set == nil || *set == nil {
		return ""
	}
	prev := Off
	value, sep := "", ""
	for idx, r := range *set {
		detail := ""
		if idx == 0 || r.detail != prev {
			detail = r.detail.String() + ":"
		}
		value += sep + detail + r.method
		sep = ","
		prev = r.detail
	}
	return value
}

// detailOf returns the level of detail for dumping the given method.
func (set *ruleset) detailOf(method string) level {
	log.Debug("%s: checking level of detail...", method)
	if set == nil {
		return Off
	}
	detail := Off
	for _, r := range *set {
		log.Debug("  - checking rule '%s'...", r.method)
		switch {
		case r.method == method:
			log.Debug("    => exact match: %v", r.detail)
			return r.detail
		case r.method == "*":
			log.Debug("    => wildcard match: %v", r.detail)
			detail = r.detail
		case r.regexp != nil && r.regexp.MatchString(method):
			log.Debug("    => regexp match (%s): %v", r.method, r.detail)
			detail = r.detail
		}
	}
	return detail
}

// copy creates a (shallow) copy of the ruleset.
func (set *ruleset) duplicate() ruleset {
	if set == nil || *set == nil {
		return nil
	}
	cp := make([]*rule, len(*set))
	copy(cp, *set)
	return cp
}

// String returns the level of detail as a string.
func (detail level) String() string {
	switch detail {
	case Off:
		return "off"
	case Name:
		return "name"
	case Full:
		return "full"
	}
	return fmt.Sprintf("<invalid dump level of detail %d>", detail)
}

// defaultOptions returns a new options instance, initialized to defaults.
func defaultOptions() interface{} {
	o := &options{Config: DefaultConfig}
	o.rules.parse(DefaultConfig)
	return o
}

// configNotify updates our runtime configuration.
func (o *options) configNotify(event config.Event, source config.Source) error {
	log.Info("message dumper configuration %v", event)
	log.Info(" * config: %s", o.Config)

	rules := ruleset{}
	if err := rules.parse(o.Config); err != nil {
		return err
	}

	o.rules = rules

	log.Info(" * parsed: %s", o.rules.String())
	log.Info(" * dump file: %v", opt.File)
	log.Info(" * log with debug: %v", opt.Debug)

	dump.configure(o)

	return nil
}

// Register us for command line parsing and configuration handling.
func init() {
	opt.rules.parse(opt.Config)
	config.Register("dump", configHelp, opt, defaultOptions,
		config.WithNotify(opt.configNotify))
}
