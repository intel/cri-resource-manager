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

package log

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	// DefaultLogger is the name of the default logging backend.
	DefaultLogger = "fmt"
	// DefaultLevel is the default lowest unsuppressed severity.
	DefaultLevel = LevelInfo

	// Flag for selecting logger backend.
	optionLogger = "logger"
	// Flag for selecting logging level.
	optionLevel = "logger-level"
	// Flag for enabling logging sources.
	optionSource = "logger-source"
	// Flag for enabling/disabling logging sources.
	optionDebug = "logger-debug"
	// Flag for controlling message prefixing with log source name.
	optionPrefix = "logger-prefix"
	// Flag for listing registered logging backends.
	optionList = "list-loggers"
	// Use prefixing preference of backend.
	dontCare = -1
)

// LevelNames maps severity levels to names.
var LevelNames = map[Level]string{
	LevelDebug: "debug",
	LevelInfo:  "info",
	LevelWarn:  "warn",
	LevelError: "error",
}

// NamedLevels maps severity names to levels.
var NamedLevels = map[string]Level{
	"debug": LevelDebug,
	"info":  LevelInfo,
	"warn":  LevelWarn,
	"error": LevelError,
}

// Backend names in default order of preference.
var defaultBackends = []string{DefaultLogger}

// Logger options configurable via the command line.
type options struct {
	level    Level              // lowest unsuppressed severity
	sources  map[string]bool    // enabled logger sources, all if nil
	debugs   map[string]bool    // enabled debug flags, all if nil
	loggers  map[string]*logger // running loggers (log sources)
	logger   string             // name of active/selected backend
	active   Backend            // active backend
	prefix   int                // prefix messages with source ?
	backends map[string]Backend // registered backends (real loggers)
	srcalign int
}

// Logger options with their defaults.
var opt = options{
	level:  DefaultLevel,
	debugs: map[string]bool{},
	logger: DefaultLogger,
	prefix: dontCare,
}

func (o *options) sourceEnabled(source string) bool {
	if o.sources == nil {
		return true
	}

	_, enabled := o.sources[source]
	return enabled
}

func (o *options) debugEnabled(source string) bool {
	if o.debugs == nil {
		return true
	}

	_, enabled := o.debugs[source]
	return enabled
}

func (o *options) splitFlagsAndState(value string) (string, bool, error) {
	var err error

	state := true
	split := strings.Split(value, ":")
	flags := split[0]
	switch {
	case len(split) > 2:
		return "", false, fmt.Errorf("log: invalid source spec '%s'", value)
	case len(split) == 2:
		if state, err = strconv.ParseBool(split[1]); err != nil {
			return "", false, err
		}
	}

	return flags, state, nil
}

func (o *options) parseFlags(optName, values string, state bool) {
	var flags map[string]bool

	if optName == optionSource {
		flags = o.sources
	} else {
		flags = o.debugs
	}

	if flags == nil {
		flags = make(map[string]bool)
	}

	for _, f := range strings.Split(values, ",") {
		switch f {
		case "all", "*":
			if state {
				flags = nil
			} else {
				flags = make(map[string]bool)
			}
		case "none":
			if state {
				flags = make(map[string]bool)
			} else {
				flags = nil
			}
		default:
			flags[f] = state
		}
	}

	if optName == optionSource {
		o.sources = flags
		for name, state := range flags {
			if l, ok := opt.loggers[name]; ok {
				l.enabled = state
			}
		}
	} else {
		o.debugs = flags
		if o.debugs != nil {
			for _, l := range opt.loggers {
				l.debug = false
			}
			for name, state := range flags {
				if l, ok := opt.loggers[name]; ok {
					l.debug = state
					if l.debug {
						l.enabled = true
					}
				}
			}
		} else {
			for _, l := range opt.loggers {
				l.debug = true
			}
		}
	}
}

func (o *options) Set(name, value string) error {
	switch name {
	case optionLogger:
		o.logger = value
		SelectBackend(o.logger)

	case optionLevel:
		level, ok := NamedLevels[value]
		if !ok {
			return fmt.Errorf("log: unknown log level '%s'", value)
		}
		o.level = level
		o.updateLoggers()

	case optionSource:
		sources, state, err := o.splitFlagsAndState(value)
		if err != nil {
			return err
		}
		o.parseFlags(optionSource, sources, state)

	case optionDebug:
		sources, state, err := o.splitFlagsAndState(value)
		if err != nil {
			return err
		}
		o.parseFlags(optionDebug, sources, state)

	case optionPrefix:
		switch value {
		case "off", "false":
			o.prefix = 0
		default:
			o.prefix = 1
		}

	case optionList:
		fmt.Printf("The available logger backends are: %s\n", registeredBackendNames())
		os.Exit(0)

	default:
		return fmt.Errorf("can't set unknown logger option '%s' to '%s'", name, value)
	}

	return nil
}

func (o *options) Get(name string) string {
	switch name {
	case optionLogger:
		return o.logger

	case optionLevel:
		return o.level.String()

	case optionSource:
		switch {
		case o.sources == nil:
			return "all"
		case len(o.sources) == 0:
			return "none"
		default:
			value := ""
			sep := ""
			for source, state := range o.sources {
				value += sep + source + ":" + strconv.FormatBool(state)
			}
			return value
		}

	case optionDebug:
		switch {
		case o.debugs == nil:
			return "all"
		case len(o.debugs) == 0:
			return "none"
		default:
			value := ""
			sep := ""
			for source, state := range o.debugs {
				value += sep + source + ":" + strconv.FormatBool(state)
			}
			return value
		}

	case optionPrefix:
		switch o.prefix {
		case 0:
			return "off"
		case 1:
			return "on"
		default:
			return "<by backend preference>"
		}

	default:
		return fmt.Sprintf("<no value for unknown logger option '%s'>", name)
	}
}

type wrappedOption struct {
	name   string
	opt    *options
	isBool bool
}

func wrapOption(name, usage string) (flag.Value, string, string) {
	return wrappedOption{name: name, opt: &opt}, name, usage
}

func (wo wrappedOption) Name() string {
	return wo.name
}

func (wo wrappedOption) Set(value string) error {
	err := wo.opt.Set(wo.Name(), value)

	return err
}

func (wo wrappedOption) String() string {
	if wo.isBool {
		return ""
	}
	return wo.opt.Get(wo.Name())
}

func (wo *wrappedOption) IsBoolFlag() bool {
	return wo.isBool
}

func wrapBoolean(name, usage string) (flag.Value, string, string) {
	return &wrappedOption{name: name, opt: &opt, isBool: true}, name, usage
}

func init() {
	flag.Var(wrapOption(optionLevel,
		"least severity of log messages to start passsing through.\n"))
	flag.Var(wrapOption(optionSource,
		"value is a comma-separated logger source names to enable.\n"+
			"Specify '*' or all for enabling logging for all sources."))
	flag.Var(wrapOption(optionDebug,
		"value is a comma-separated logger source names to enable debug for.\n"+
			"Specify '*' or all for enabling debugging for all sources."))
	flag.Var(wrapOption(optionLogger,
		"select logging backends to use.\n"+
			"The available backends are "+registeredBackendNames()+"."))
	flag.Var(wrapOption(optionPrefix,
		"whether to prefix logged messages with log source"))
	flag.Var(wrapBoolean(optionList,
		"list available logger backends"))

}
