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

package log

import (
	"encoding/json"
	"flag"
	"strings"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

const (
	// DefaultLevel is the default logging severity level.
	DefaultLevel = LevelInfo
	// command-line argument prefix.
	optPrefix = "logger"
	// Flag for enabling/disabling normal non-debug logging for sources.
	optEnable = optPrefix + "-sources"
	// Flag for enabling/disabling debug logging for sources.
	optDebug = optPrefix + "-debug"
	// Flag for selecting logging level.
	optLevel = optPrefix + "-level"
	// Flag for selecting logging backend.
	optLogger = optPrefix
	// configModule is our module name in the runtime configuration.
	configModule = optPrefix
)

// Logger options configurable via the command line or pkg/config.
type options struct {
	// Level is the logging severity/level.
	Level Level
	// Enable is a map for enabling/disabling normal logging for sources.
	Enable srcmap
	// Debug is a map for enabling/disabling debug logging for sources.
	Debug srcmap
	// Logger is the name of the logger backend to use.
	Logger backendName
}

// srcmap tracks logging or debugging settings for sources.
type srcmap map[string]bool

// backendName is a name for a Backend.
type backendName string

// Default configuration given on the command line (or set via pkg/flag).
var defaults = &options{
	Logger: FmtBackendName,
	Level:  DefaultLevel,
	Enable: make(srcmap),
	Debug:  make(srcmap),
}

// Runtime configuration, from an agent, fallback, or forced configuration file.
var opt = &options{
	Logger: FmtBackendName,
	Level:  DefaultLevel,
	Enable: make(srcmap),
	Debug:  make(srcmap),
}

// Set sets the level from the given name.
func (l *Level) Set(value string) error {
	levels := map[string]Level{
		"debug":   LevelDebug,
		"info":    LevelInfo,
		"warning": LevelWarn,
		"error":   LevelError,
		"fatal":   LevelFatal,
		"panic":   LevelPanic,
	}
	level, ok := levels[strings.ToLower(value)]
	if !ok {
		return loggerError("invalid logging level %s", value)
	}

	*l = level
	opt.Level = level
	SetLevel(level)

	return nil
}

// String returns the name of the level.
func (l Level) String() string {
	names := map[Level]string{
		LevelDebug: "debug",
		LevelInfo:  "info",
		LevelWarn:  "warning",
		LevelError: "error",
		LevelFatal: "fatal",
		LevelPanic: "panic",
	}
	if level, ok := names[l]; ok {
		return level
	}

	return names[LevelInfo]
}

// Set sets the name of the active Backend.
func (n *backendName) Set(value string) error {
	if err := SetBackend(value); err != nil {
		return err
	}

	return nil
}

// String returns the name of the active backend.
func (n backendName) String() string {
	return string(n)
}

// Set sets entries of srcmap by parsing the given value.
func (m *srcmap) Set(value string) error {
	log.Lock()
	defer log.Unlock()

	sm := *m
	prev, state, src := "", "", ""
	for _, entry := range strings.Split(value, ",") {
		statesrc := strings.Split(entry, ":")
		switch len(statesrc) {
		case 2:
			state, src = statesrc[0], statesrc[1]
		case 1:
			state, src = "", statesrc[0]
		default:
			return loggerError("invalid state spec '%s' in source map", entry)
		}

		if state != "" {
			prev = state
		} else {
			state = prev
			if state == "" {
				state = "on"
			}
		}
		if src == "all" {
			src = "*"
		}

		enabled, err := utils.ParseEnabled(state)
		if err != nil {
			return loggerError("invalid state '%s' in source map", state)
		}
		sm[src] = enabled
	}

	// propagate command-line to runtime defaults, reconfigure loggers
	if m == &defaults.Enable {
		opt.Enable.copy(sm)
		log.update(sm, nil)
	}
	if m == &defaults.Debug {
		opt.Debug.copy(sm)
		log.update(nil, sm)
	}

	return nil
}

// String returns a string representation of the srcmap.
func (m *srcmap) String() string {
	log.RLock()
	defer log.RUnlock()

	off := ""
	on := ""
	for src, state := range *m {
		if state {
			if on == "" {
				on = src
			} else {
				on += "," + src
			}
		} else {
			if off == "" {
				off = src
			} else {
				off += "," + src
			}
		}
	}

	if off == "" {
		return "on:" + on
	}
	if on == "" {
		return "off:" + off
	}

	return "on:" + on + "," + "off:" + off
}

// MarshalJSON is the JSON marshaller for srcmap.
func (m srcmap) MarshalJSON() ([]byte, error) {
	raw := map[string][]string{"on": {}, "off": {}}
	which := map[bool][]string{false: raw["off"], true: raw["on"]}

	for src, state := range m {
		which[state] = append(which[state], src)
	}

	return json.Marshal(raw)
}

// UnmarshalJSON is the JSON unmarshaller for srcmap.
func (m *srcmap) UnmarshalJSON(raw []byte) error {
	var err error

	*m = make(map[string]bool)

	boolmap := map[bool][]string{}
	if err := json.Unmarshal(raw, &boolmap); err == nil {
		for state, sources := range boolmap {
			for _, src := range sources {
				if src == "all" {
					src = "*"
				}
				(*m)[src] = state
			}
		}
		return nil
	}

	rawmap := map[string][]string{}
	if err = json.Unmarshal(raw, &rawmap); err == nil {
		for state, sources := range rawmap {
			for _, src := range sources {
				if src == "all" {
					src = "*"
				}
				(*m)[src], err = utils.ParseEnabled(state)
				if err != nil {
					return loggerError("source '%s' has invalid state '%s' in logger source map",
						src, state)
				}
			}
		}
		return nil
	}

	cfgstr := ""
	if err = json.Unmarshal(raw, &cfgstr); err == nil {
		if err := m.Set(cfgstr); err != nil {
			return loggerError("failed to unmarshal logger source map/configuration '%s': %v",
				string(raw), err)
		}
		return nil
	}

	return loggerError("failed to unmarshal logger source map '%s': %v",
		string(raw), err)
}

// copy state from another srcmap.
func (m srcmap) copy(o srcmap) {
	for src, state := range o {
		m[src] = state
	}
}

// configNotify is the configuration change notification callback for options.
func (o *options) configNotify(event pkgcfg.Event, src pkgcfg.Source) error {
	deflog.Info("logger configuration event %v", event)

	deflog.Info("*  log level: %v", opt.Level)
	deflog.Info("*    logging: %v", opt.Enable.String())
	deflog.Info("*  debugging: %v", opt.Debug.String())

	log.Lock()
	defer log.Unlock()

	log.setLevel(opt.Level)
	log.setBackend(opt.Logger.String())

	if len(opt.Enable) == 0 {
		opt.Enable.copy(defaults.Enable)
	}
	if len(opt.Debug) == 0 {
		opt.Debug.copy(defaults.Debug)
	}
	log.update(opt.Enable, opt.Debug)

	return nil
}

func defaultOptions() interface{} {
	o := &options{
		Logger: defaults.Logger,
		Level:  defaults.Level,
		Enable: make(srcmap),
		Debug:  make(srcmap),
	}
	for key, value := range defaults.Enable {
		o.Enable[key] = value
	}
	for key, value := range defaults.Debug {
		o.Debug[key] = value
	}

	return o
}

// Register us for command line parsing and configuration handling.
func init() {
	cfglog := log.get("config")
	pkgcfg.SetLogger(pkgcfg.Logger{
		DebugEnabled: cfglog.DebugEnabled,
		Debug:        cfglog.Debug,
		Info:         cfglog.Info,
		Warning:      cfglog.Warn,
		Error:        cfglog.Error,
		Fatal:        cfglog.Fatal,
		Panic:        cfglog.Panic,
	})

	flag.Var(&defaults.Logger, optLogger,
		"override logger backend to use.")
	flag.Var(&defaults.Level, optLevel,
		"lowest severity level to pass through (info, warning, error)")
	flag.Var(&defaults.Enable, optEnable,
		"comma-separated list of source names to enable/disable.\n"+
			"Specify '*' or 'all' to enable all sources, which is also the default.\n"+
			"Prefix a source or list with 'off:' to disable.")
	flag.Var(&defaults.Debug, optDebug,
		"comma-separated list of source names to enable debug messages for.\n"+
			"Specify '*' or 'all' to enable all sources.\n"+
			"Prefix a source or list with 'off:' to disable, which is also the default state.")

	pkgcfg.Register(configModule, configHelp, opt, defaultOptions,
		pkgcfg.WithNotify(opt.configNotify))
}
