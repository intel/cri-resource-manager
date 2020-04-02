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
	maxname int                  // longest enabled/debugging source name
	level   Level                // logging severity level
	disable srcmap               // logging source configuration
	debug   srcmap               // debugging source configuration
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
	debug:   make(srcmap),
	backend: make(map[string]BackendFn),
	active:  createFmtBackend(),
}

// Flush flushes any initial message buffer and turns buffering on.
func Flush() {
	log.RLock()
	defer log.RUnlock()
	log.active.Flush()
}

// Sync waits for all current messages to get processed.
func Sync() {
	log.RLock()
	defer log.RUnlock()
	log.active.Sync()
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

	return log.setDebugging(source, true)
}

// DisableDebug disables debug logging for the given source.
func DisableDebug(source string) bool {
	log.Lock()
	defer log.Unlock()

	return log.setDebugging(source, false)
}

// DebugEnabled checks if debug logging is enabled for the source.
func DebugEnabled(source string) bool {
	log.RLock()
	defer log.RUnlock()

	return log.isDebugging(source)
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
	if log.active.Name() == name {
		return nil
	}

	createFn, ok := log.backend[name]
	if !ok {
		return loggerError("can't activate unknown backend '%s'", name)
	}

	log.active.Stop()
	log.active = createFn()
	log.active.SetSourceAlignment(log.maxname)

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

// get returns the logger for source, creating one if necessary (write-locks log).
func (log *logging) get(source string) Logger {
	log.Lock()
	defer log.Unlock()

	if id, ok := log.loggers[source]; ok {
		return id
	}

	return log.create(source)
}

// create creates a new logger for a source (should be called with log write-locked).
func (log *logging) create(source string) Logger {
	id := logger(len(log.loggers))
	if uint64(id) >= maxLoggers {
		panic(fmt.Sprintf("max. number of loggers (%d) exhausted", maxLoggers))
	}

	cfg := mkConfig(id, log.isLogging(source), log.isDebugging(source))
	log.loggers[source] = id
	log.sources[id] = source
	log.configs[id] = cfg

	if (cfg.isEnabled() || log.forced) && len(source) > log.maxname {
		log.realign(len(source))
	}

	return id
}

// setLogging sets the logging state of the source (should be called with log write-locked).
func (log *logging) setLogging(source string, state bool) bool {
	// logging is opt-out (enabled by default) and administered by negated state
	old := log.isLogging(source)
	log.disable[source] = !state

	return old
}

// isLogging gets the logging state of the source (should be called with log read-locked).
func (log *logging) isLogging(source string) bool {
	if state, ok := log.disable[source]; ok {
		return !state
	}
	if state, ok := log.disable["*"]; ok {
		return !state
	}

	return true
}

// setDebugging sets the debugging state of the source (should be called with log write-locked).
func (log *logging) setDebugging(source string, state bool) bool {
	// debugging is opt-in (disabled by default)
	old := log.isDebugging(source)
	log.debug[source] = state

	return old
}

// isDebugging gets the debugging state of the source (should be called with log read-locked).
func (log *logging) isDebugging(source string) bool {
	if state, ok := log.debug[source]; ok {
		return state
	}
	if state, ok := log.debug["*"]; ok {
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
			if !cfg.isEnabled() && !log.forced {
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
func (log *logging) update(enabled srcmap, debug srcmap) {
	if enabled != nil {
		log.disable = make(srcmap)
		for src, state := range enabled {
			log.disable[src] = !state
		}
	}
	if debug != nil {
		log.debug = make(srcmap)
		for src, state := range debug {
			log.debug[src] = state
		}
	}
	maxname := 0
	for id, cfg := range log.configs {
		source := log.sources[id]
		logging := log.isLogging(source)
		debugging := log.isDebugging(source)
		cfg.setEnabled(log.isLogging(source), log.isDebugging(source))
		log.configs[id] = cfg
		if logging || debugging || log.forced {
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
