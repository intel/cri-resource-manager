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
	"github.com/intel/cri-resource-manager/pkg/config"
	"os"
	"strconv"
	"strings"
)

const (
	// DefaultLogger is the name of the default logging backend.
	DefaultLogger = "fmt"
	// DefaultLevel is the default lowest unsuppressed severity.
	DefaultLevel = LevelInfo
	// Use prefixing preference of backend.
	PrefixDontCare = -1

	// Flag for selecting logger backend.
	optLogger = "backend"
	// Flag for selecting logging level.
	optLevel = "level"
	// Flag for enabling logging sources.
	optSource = "source"
	// Flag for enabling/disabling debugging sources.
	optDebug = "debug"
	// Flag for controlling message prefixing with log source name.
	optPrefix = "prefix"
	// Flag for listing the available logger backends.
	optListBackends = "list"
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

// Options captures our configurable logger options.
type options struct {
	level    Level              // lowest unsuppressed severity
	sources  mapState           // enabled logger sources, all if nil
	debugs   mapState           // enabled debug flags, all if nil
	loggers  map[string]*logger // running loggers (log sources)
	logger   backendName        // name of active/selected backend
	active   Backend            // active backend
	prefix   prefix             // prefix messages with source ?
	backends map[string]Backend // registered backends (real loggers)
	list     bool               // list backends and exit
	srcalign int
}

type mapState map[string]bool
type srcState mapState
type dbgState mapState
type backendName string
type prefix int

// Our configuration module and configurable options.
var cfg *config.Module
var opt = options{
	level:  DefaultLevel,
	prefix: PrefixDontCare,
	debugs: map[string]bool{},
	logger: DefaultLogger,
	active: &fmtBackend{},
}

func (m *mapState) Set(value string) error {
	var err error

	state := true
	split := strings.Split(value, ":")
	names := split[0]
	switch {
	case len(split) > 2:
		return loggerError("invalid spec '%s'", value)
	case len(split) == 2:
		status := strings.ToLower(split[1])
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
	}

	for _, f := range strings.Split(names, ",") {
		switch f {
		case "all", "*":
			if state {
				(*m) = nil
			} else {
				(*m) = make(map[string]bool)
			}
		case "none":
			if state {
				(*m) = make(map[string]bool)
			} else {
				(*m) = nil
			}
		default:
			if *m == nil {
				*m = make(map[string]bool)
			}
			(*m)[f] = state
		}
	}

	return nil
}

func (m *mapState) String() string {
	if *m == nil {
		return "all"
	}
	if len(*m) == 0 {
		return "none"
	}

	tVal, tSep := "", ""
	fVal, fSep := "", ""

	for name, state := range *m {
		if state {
			tVal += tSep + name
			tSep = ","
		} else {
			fVal += fSep + name
			fSep = ","
		}
	}

	if tVal != "" {
		tVal += ":on"
	}
	if fVal != "" {
		fVal += ":off"
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

func (m *mapState) enabled(name string) bool {
	if m == nil || *m == nil {
		return true
	}

	state, _ := (*m)[name]
	return state
}

func (s *srcState) Set(value string) error {
	if err := ((*mapState)(s)).Set(value); err != nil {
		return err
	}

	for name, state := range *s {
		if l, ok := opt.loggers[name]; ok {
			l.enabled = state
		}
	}
	return nil
}

func (s *srcState) String() string {
	return ((*mapState)(s)).String()
}

func (s *dbgState) Set(value string) error {
	if err := ((*mapState)(s)).Set(value); err != nil {
		return err
	}

	if *s != nil {
		for _, l := range opt.loggers {
			l.debug = false
		}
		for name, state := range *s {
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
	return nil
}

func (s *dbgState) String() string {
	return ((*mapState)(s)).String()
}

func (o *options) sourceEnabled(source string) bool {
	return o.sources.enabled(source)
}

func (o *options) debugEnabled(source string) bool {
	return o.debugs.enabled(source)
}

func (l *Level) Set(value string) error {
	level, ok := NamedLevels[value]
	if !ok {
		return loggerError("unknown log level '%s'", value)
	}
	*l = level
	opt.updateLoggers()
	return nil
}

func (l Level) String() string {
	if name, ok := LevelNames[l]; ok {
		return name
	}

	return LevelNames[LevelInfo]
}

func (be *backendName) Set(value string) error {
	*be = backendName(value)
	SelectBackend(value)
	return nil
}

func (be *backendName) String() string {
	return string(*be)
}

func (p *prefix) Set(value string) error {
	switch value {
	case "off", "false":
		*p = 0
	case "on", "true":
		*p = 1
	default:
		*p = PrefixDontCare
	}
	return nil
}

func (p *prefix) String() string {
	switch *p {
	case 0:
		return "off"
	case 1:
		return "on"
	default:
		return "dontcare"
	}
}

func configChanged(event config.Event, source config.Source) error {
	if opt.list {
		ListBackends(os.Stdout)
		os.Exit(0)
	}
	return nil
}

// Register us for configuration handling.
func init() {
	// give config a logger
	logger := NewLogger("config")
	config.SetLogger(config.Logger{
		DebugEnabled: logger.DebugEnabled,
		Debug:        logger.Debug,
		Info:         logger.Info,
		Warning:      logger.Warn,
		Error:        logger.Error,
		Fatal:        logger.Fatal,
		Panic:        logger.Panic,
		Block:        logger.Block,
	})

	cfg = config.Register("logger", "logging and debugging",
		config.WithUnsetVarsKept(), config.WithNotify(configChanged))

	cfg.Var(&opt.logger, optLogger,
		"select logging backend to use. You can list the available backends using the\n"+
			optListBackends+" command line option.")
	cfg.Var(&opt.level, optLevel,
		"least severity of log messages to start passsing through.")
	cfg.Var(&opt.sources, optSource,
		"Enable/disable logging of sources. Value is source[,...,source[:{on|off}]].\n"+
			"By default all sources are logged. Use '*' or 'all' to refer to all sources.")
	cfg.Var(&opt.debugs, optDebug,
		"Enable/disable debugging of sources. Value is source[,...,source[:{on|off}]].\n"+
			"By default no sources are debugged. Use '*' or 'all' to refer to all sources.")
	cfg.Var(&opt.prefix, optPrefix,
		"whether to prefix logged messages with log source")

	cfg.BoolVar(&opt.list, optListBackends, false,
		"List available logger backends.")
}
