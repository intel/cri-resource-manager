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
	"encoding/json"
	"flag"
	"fmt"
	"github.com/intel/cri-resource-manager/pkg/config"
	"strconv"
	"strings"
)

const (
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
	"debug":   LevelDebug,
	"info":    LevelInfo,
	"warn":    LevelWarn,
	"warning": LevelWarn,
	"error":   LevelError,
}

// Logging can be configured both from the command line and through pkg/config.
// Options given on the command line change both the defaults and the runtime
// configuration. Configuration received via pkg/config only changes the runtime
// configuration. An empty runtime configuration is initialized to the defaults.

// Logger options configurable via the command line or pkg/config.
type options struct {
	Level  Level       // lowest unsuppressed severity
	Logger backendName // name of active backend
	Enable stateMap    // enabled logger sources, all if nil
	Debug  stateMap    // enable debug flags, all if nil
}

type stateMap map[string]bool
type backendName string

// Our default and runtime configuration.
var defaults = &options{
	Level:  DefaultLevel,
	Logger: backendName(fmtBackendName),
	Enable: stateMap{"*": true},
	Debug:  stateMap{"*": false},
}
var opt = defaultOptions().(*options)

// Set is the flag.Value setter for Level.
func (l *Level) Set(value string) error {
	level, ok := NamedLevels[value]
	if !ok {
		return loggerError("unknown log level '%s'", value)
	}
	*l = level

	if l == &defaults.Level {
		opt.updateLoggers()
		opt.Level = level
	}

	return nil
}

// String is the flag.Value stringification for Level.
func (l Level) String() string {
	if name, ok := LevelNames[l]; ok {
		return name
	}

	return LevelNames[LevelInfo]
}

func (n *backendName) Set(value string) error {
	*n = backendName(value)
	activateBackend(value)

	if n == &defaults.Logger {
		opt.Logger = *n
	}

	return nil
}

func (n backendName) String() string {
	return string(n)
}

func (m *stateMap) Set(value string) error {
	var err error

	if m != &defaults.Enable && m != &defaults.Debug { // from the command line we cumulate these
		*m = make(stateMap)
	}

	prev := "on"
	for _, req := range strings.Split(strings.TrimSpace(value), ",") {
		var state bool
		status := prev
		names := ""
		split := strings.SplitN(req, ":", 2)

		switch len(split) {
		case 1:
			names = split[0]
		case 2:
			status = split[0]
			names = split[1]
			prev = status
		default:
			continue
		}

		switch status {
		case "on", "enable", "enabled":
			state = true
		case "off", "disable", "disabled":
			state = false
		default:
			if state, err = strconv.ParseBool(status); err != nil {
				return loggerError("invalid state '%s' in spec '%s': %v", status, value, err)
			}
		}

		for _, f := range strings.Split(names, ",") {
			switch f {
			case "all", "*":
				(*m)["*"] = state
			case "none":
				(*m)["*"] = !state
			default:
				(*m)[f] = state
			}
		}
	}

	var optMap *stateMap

	switch {
	case m == &defaults.Enable:
		optMap = &opt.Enable
	case m == &defaults.Debug:
		optMap = &opt.Debug
	}

	if optMap != nil {
		opt.updateLoggers()
		*optMap = make(stateMap)
		for key, value := range *m {
			(*optMap)[key] = value
		}
	}

	return nil
}

func (m *stateMap) String() string {
	if *m == nil {
		return "all"
	}
	if len(*m) == 0 {
		return "none"
	}

	tVal, tSep := "", ""
	fVal, fSep := "", ""

	for name, state := range *m {
		if name == "*" {
			name = "all"
		}
		if state {
			tVal += tSep + name
			tSep = ","
		} else {
			fVal += fSep + name
			fSep = ","
		}
	}

	if tVal != "" {
		tVal = "on:" + tVal
	}
	if fVal != "" {
		fVal = "off:" + fVal
	}

	switch {
	case fVal == "":
		return tVal
	case tVal == "":
		return fVal
	case tVal != "" && fVal != "":
		return tVal + "," + fVal
	}
	return ""
}

func (m *stateMap) isEnabled(name string) bool {
	if m == nil || *m == nil {
		return false
	}

	if state, ok := (*m)[name]; ok {
		return state
	}
	if state, ok := (*m)["*"]; ok {
		return state
	}

	return false
}

func (o *options) MarshalJSON() ([]byte, error) {
	cfg := map[string]string{
		"Level":  o.Level.String(),
		"Logger": string(o.Logger),
	}
	if o.Enable != nil {
		cfg["Enable"] = o.Enable.String()
	}
	if o.Debug != nil {
		cfg["Debug"] = o.Debug.String()
	}

	return json.Marshal(cfg)
}

func (o *options) UnmarshalJSON(raw []byte) error {
	cfg := map[string]string{}

	if err := json.Unmarshal(raw, &cfg); err != nil {
		return loggerError("failed to unmarshal logger configuration: %v", err)
	}

	*o = *defaults

	for key, value := range cfg {
		switch key {
		case "Level":
			if err := o.Level.Set(value); err != nil {
				return err
			}
		case "Logger":
			o.Logger = backendName(value)
			activateBackend(value)
		case "Enable":
			if err := o.Enable.Set(value); err != nil {
				return err
			}
		case "Debug":
			if err := o.Debug.Set(value); err != nil {
				return err
			}
		default:
			return loggerError("unknown configuration entry '%s' (%s)", key, value)
		}
	}

	return nil
}

func (o *options) sourceEnabled(source string) bool {
	return o.Enable.isEnabled(source)
}

func (o *options) debugEnabled(source string) bool {
	return o.Debug.isEnabled(source)
}

func loggerError(format string, args ...interface{}) error {
	return fmt.Errorf("log: "+format, args...)
}

// defaultOptions returns a new options instance initialized to defaults.
func defaultOptions() interface{} {
	o := &options{
		Level:  defaults.Level,
		Logger: defaults.Logger,
		Enable: make(stateMap),
		Debug:  make(stateMap),
	}
	for key, value := range defaults.Enable {
		o.Enable[key] = value
	}
	for key, value := range defaults.Debug {
		o.Debug[key] = value
	}
	return o
}

// configNotify updates our runtime configuration.
func (o *options) configNotify(event config.Event, source config.Source) error {
	defLogger.Info("logger configuration %v", event)

	opt.updateLoggers()

	defLogger.Info(" * log level: %v", opt.Level)
	defLogger.Info(" * enabled sources: %v", opt.Enable.String())
	defLogger.Info(" * enabled debugging: %v", opt.Debug.String())

	return nil
}

// Register us for command line parsing and configuration handling.
func init() {
	cfglog := NewLogger("config")
	config.SetLogger(config.Logger{
		DebugEnabled: cfglog.DebugEnabled,
		Debug:        cfglog.Debug,
		Info:         cfglog.Info,
		Warning:      cfglog.Warn,
		Error:        cfglog.Error,
		Fatal:        cfglog.Fatal,
		Panic:        cfglog.Panic,
		Block:        cfglog.Block,
	})

	flag.Var(&defaults.Level, optionLevel,
		"least severity of log messages to start passing through.")
	flag.Var(&defaults.Enable, optionSource,
		"value is a comma-separated logger source names to enable.\n"+
			"Specify '*' or all for enabling logging for all sources.")
	flag.Var(&defaults.Debug, optionDebug,
		"value is a comma-separated logger source names to enable debug for.\n"+
			"Specify '*' or all for enabling debugging for all sources.")
	flag.Var(&defaults.Logger, optionLogger,
		"select logging backend to use")

	config.Register("logger", configHelp, opt, defaultOptions,
		config.WithNotify(opt.configNotify))
}
