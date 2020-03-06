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
	"fmt"
	"sync"
)

// logging encapsulates the full runtime state of logging.
type logging struct {
	sync.RWMutex
	loggers map[string]logger    // source to logger mapping
	sources map[logger]string    // logger to source mapping
	configs map[logger]config    // logger configuration
	maxname int                  // longest enabled/tracing source name
	level   Level                // logging severity level
	disable srcmap               // logging source configuration
	tracing srcmap               // tracing source configuration
	backend map[string]BackendFn // registered backends
	active  Backend              // logging backend
	forced  bool                 // whether forced debugging is on
}

// our logging runtime state
var log = &logging{
	level:   LevelInfo,
	loggers: make(map[string]logger),
	sources: make(map[logger]string),
	configs: make(map[logger]config),
	disable: make(srcmap),
	tracing: make(srcmap),
	backend: make(map[string]BackendFn),
	active:  createFmtBackend(),
}

// Flush flushes any optional startup message buffer and turns buffering on.
func Flush() {
	log.RLock()
	defer log.RUnlock()

	log.active.Flush()
}

// Get returns the Logger for source, creating one if necessary.
func Get(source string) Logger {
	return log.get(source)
}

// NewLogger is an alias for Get().
func NewLogger(source string) Logger {
	return log.get(source)
}

// EnableLogging enables non-debug logging for the source.
func EnableLogging(source string) bool {
	log.Lock()
	defer log.Unlock()

	return log.setLogging(source, true)
}

// DisableLogging disables non-debug logging for the given source.
func DisableLogging(source string) bool {
	log.Lock()
	defer log.Unlock()

	return log.setLogging(source, false)
}

// LoggingEnabled checks if non-debug logging is enabled for the source.
func LoggingEnabled(source string) bool {
	log.RLock()
	defer log.RUnlock()

	return log.isLogging(source)
}

// EnableDebug enables debug logging for the source.
func EnableDebug(source string) bool {
	log.Lock()
	defer log.Unlock()

	return log.setTracing(source, true)
}

// DisableDebug disables debug logging for the given source.
func DisableDebug(source string) bool {
	log.Lock()
	defer log.Unlock()

	return log.setTracing(source, false)
}

// DebugEnabled checks if debug logging is enabled for the source.
func DebugEnabled(source string) bool {
	log.RLock()
	defer log.RUnlock()

	return log.isTracing(source)
}

// SetLevel sets the logging severity level.
func SetLevel(level Level) {
	log.setLevel(level)
}

// SetBackend activates the named Backend for logging.
func SetBackend(name string) error {
	log.Lock()
	defer log.Unlock()

	return log.setBackend(name)
}

// setLevel sets the logging severity level.
func (log *logging) setLevel(level Level) {
	log.level = level
}

// setBackend activates the named Backend for logging.
func (log *logging) setBackend(name string) error {
	createFn, ok := log.backend[name]
	if !ok {
		return loggerError("can't activate unknown backend '%s'", name)
	}

	log.active.Stop()
	log.active = createFn()

	return nil
}

// forceDebug enables/disables forced full debugging.
func (log *logging) forceDebug(state bool) bool {
	log.Lock()
	defer log.Unlock()

	old := log.forced
	log.forced = state
	log.update(nil, nil)

	return old
}

// debugForced checks if full debugging is forced.
func (log *logging) debugForced() bool {
	return log.forced
}

// get returns the logger for source, creating one if necessary (write-locks).
func (log *logging) get(source string) Logger {
	log.Lock()
	defer log.Unlock()

	if id, ok := log.loggers[source]; ok {
		return id
	}

	return log.create(source)
}

// create creates a new logger for a source (call write-locked).
func (log *logging) create(source string) Logger {
	id := logger(len(log.loggers))
	if uint64(id) >= maxLoggers {
		panic(fmt.Sprintf("max. number of loggers (%d) exhausted", maxLoggers))
	}

	cfg := mkConfig(id, log.isLogging(source), log.isTracing(source))
	log.loggers[source] = id
	log.sources[id] = source
	log.configs[id] = cfg

	if (cfg.isEnabled() || log.forced) && len(source) > log.maxname {
		log.realign(len(source))
	}

	return id
}

// setLogging sets the logging state of the given source (call write-locked).
func (log *logging) setLogging(source string, state bool) bool {
	// logging is opt-out (enabled by default) and administered by negated state
	old := log.isLogging(source)
	log.disable[source] = !state

	return old
}

// isLogging gets the logging state of the given source (call read-locked).
func (log *logging) isLogging(source string) bool {
	if state, ok := log.disable[source]; ok {
		return !state
	}
	if state, ok := log.disable["*"]; ok {
		return !state
	}

	return true
}

// setTracing sets the tracing state of the given source (call write-locked).
func (log *logging) setTracing(source string, state bool) bool {
	// tracing is opt-in (disabled by default)
	old := log.isTracing(source)
	log.tracing[source] = state

	return old
}

// isTracing gets the tracing state of the given source (call read-locked).
func (log *logging) isTracing(source string) bool {
	if state, ok := log.tracing[source]; ok {
		return state
	}
	if state, ok := log.tracing["*"]; ok {
		return state
	}

	return false
}

// realign updates prefix alignment.
func (log *logging) realign(maxname int) {
	if log.maxname != 0 && log.maxname == maxname {
		return
	}
	if maxname == 0 {
		for id, cfg := range log.configs {
			if !cfg.isEnabled() {
				continue
			}
			if length := len(log.sources[id]); length > maxname {
				maxname = length
			}
		}
	}
	log.maxname = maxname
	log.active.SetSourceAlignment(maxname)
}

// update updates the state of all loggers.
func (log *logging) update(enabled srcmap, tracing srcmap) {
	if enabled != nil {
		log.disable = make(srcmap)
		for src, state := range enabled {
			log.disable[src] = !state
		}
	}
	if tracing != nil {
		log.tracing = make(srcmap)
		for src, state := range tracing {
			log.tracing[src] = state
		}
	}
	maxname := 0
	for id, cfg := range log.configs {
		source := log.sources[id]
		logging := log.isLogging(source)
		tracing := log.isTracing(source)
		cfg.setEnabled(log.isLogging(source), log.isTracing(source))
		log.configs[id] = cfg
		if logging || tracing || log.forced {
			if length := len(source); length > maxname {
				maxname = length
			}
		}
	}
	log.realign(maxname)
}

// loggerError produces a formatted logger-specific error.
func loggerError(format string, args ...interface{}) error {
	return fmt.Errorf("logger: "+format, args...)
}
