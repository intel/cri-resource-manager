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
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

// Level describes the severity of a log message.
type Level int

const (
	// levelUnset denotes an unset level.
	levelUnset Level = iota
	// LevelDebug is the severity for debug messages.
	LevelDebug
	// LevelInfo is the severity for informational messages.
	LevelInfo
	// LevelWarn is the severity for warnings.
	LevelWarn
	// LevelError is the severity for errors.
	LevelError
	// LevelPanic is the severity for panic messages.
	LevelPanic
	// LevelFatal is the severity for fatal errors.
	LevelFatal
)

// Per-level prefix tags.
var levelTag = map[Level]string{
	levelUnset: "?: ",
	LevelDebug: "D: ",
	LevelInfo:  "I: ",
	LevelWarn:  "W: ",
	LevelError: "E: ",
	LevelFatal: "F: ",
	LevelPanic: "P: ",
}

// Logger is the interface for producing log messages for/from a particular source.
type Logger interface {
	// Standardized Logger interface functions so that this interface can be
	// used from goresctrl library.
	Debugf(format string, v ...interface{})
	Infof(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
	Panicf(format string, v ...interface{})
	Fatalf(format string, v ...interface{})

	// Debug formats and emits a debug message.
	Debug(format string, args ...interface{})
	// Info formats and emits an informational message.
	Info(format string, args ...interface{})
	// Warn formats and emits a warning message.
	Warn(format string, args ...interface{})
	// Error formats and emits an error message.
	Error(format string, args ...interface{})
	// Panic formats and emits an error message then panics with the same.
	Panic(format string, args ...interface{})
	// Fatal formats and emits an error message and os.Exit()'s with status 1.
	Fatal(format string, args ...interface{})

	// DebugBlock formats and emits a multiline debug message.
	DebugBlock(prefix string, format string, args ...interface{})
	// InfoBlock formats and emits a multiline information message.
	InfoBlock(prefix string, format string, args ...interface{})
	// WarnBlock formats and emits a multiline warning message.
	WarnBlock(prefix string, format string, args ...interface{})
	// ErrorBlock formats and emits a multiline error message.
	ErrorBlock(prefix string, format string, args ...interface{})

	// EnableDebug enables debug messages for this Logger.
	EnableDebug(bool) bool
	// DebugEnabled checks if debug messages are enabled for this Logger.
	DebugEnabled() bool

	// Source returns the source name of this Logger.
	Source() string
}

// logger implements Logger.
type logger uint

// logging encapsulates the full runtime state of logging.
type logging struct {
	sync.RWMutex
	level   Level               // logging threshold for stderr
	dbgmap  srcmap              // debug configuration
	loggers map[string]logger   // source to logger mapping
	sources map[logger]string   // logger to source mapping
	debug   map[logger]struct{} // loggers with debugging enabled
	maxlen  int                 // max source length.
	forced  bool                // forced global debugging
	prefix  bool                // prefix messages with logger source
	aligned map[logger]string   // logger sources aligned to maxlen
}

// log tracks our runtime state.
var log = &logging{
	level:   DefaultLevel,
	loggers: make(map[string]logger),
	sources: make(map[logger]string),
	aligned: make(map[logger]string),
	debug:   make(map[logger]struct{}),
}

// Get returns the named Logger.
func Get(source string) Logger {
	log.Lock()
	defer log.Unlock()
	return log.get(source)
}

// NewLogger creates the named logger.
func NewLogger(source string) Logger {
	return Get(source)
}

// EnableDebug enables debug logging for the source.
func EnableDebug(source string) bool {
	log.Lock()
	defer log.Unlock()
	return log.setDebug(source, true)
}

// DisableDebug disables debug logging for the source.
func DisableDebug(source string) bool {
	log.Lock()
	defer log.Unlock()
	return log.setDebug(source, false)
}

// DebugEnabled checks if debug logging is enabled for the source.
func DebugEnabled(source string) bool {
	log.Lock()
	defer log.Unlock()
	return log.getDebug(source)
}

// SetLevel sets the logging severity level.
func SetLevel(level Level) {
	log.Lock()
	defer log.Unlock()
	log.setLevel(level)
}

// Flush flushes any pending log messages.
func Flush() {
	log.RLock()
	defer log.RUnlock()
	klog.Flush()
}

//
// logging
//

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warning"
	case LevelError:
		return "error"
	case LevelPanic:
		return "panic"
	case LevelFatal:
		return "fatal"
	}
	return "unknown"
}

// setLevel sets the logging severity level.
func (log *logging) setLevel(level Level) error {
	log.level = level
	kThreshold := ""
	switch level {
	case LevelDebug, LevelInfo:
		kThreshold = "INFO"
	case LevelWarn:
		kThreshold = "WARNING"
	case LevelError, LevelPanic, LevelFatal:
		kThreshold = "ERROR"
	}
	if err := klogctl.Set("stderrthreshold", kThreshold); err != nil {
		return loggerError("failed to set log level/threshold to %s: %v", kThreshold, err)
	}
	return nil
}

// setDebug sets the debug state for the given source and returns the previous one.
func (log *logging) setDebug(source string, enabled bool) bool {
	l := log.get(source)
	_, old := log.debug[l]
	if enabled {
		log.debug[l] = struct{}{}
	} else {
		delete(log.debug, l)
	}
	return old
}

// getDebug sets the debug state for the given source and returns the previous one.
func (log *logging) getDebug(source string) bool {
	if log.forced {
		return true
	}
	l := log.get(source)
	_, enabled := log.debug[l]
	return enabled
}

// setDbgMap updates the debug configuration of logging.
func (log *logging) setDbgMap(dbgmap srcmap) {
	log.dbgmap = dbgmap
	log.debug = make(map[logger]struct{})
	for source := range log.loggers {
		state, ok := log.dbgmap[source]
		if !ok {
			state = log.dbgmap["*"]
		}
		log.setDebug(source, state)
	}
}

// setPrefix sets the prefix (source) logging preference.
func (log *logging) setPrefix(prefix bool) {
	log.prefix = prefix
}

// align calculates and stores an aligned prefix for the given logger.
func (log *logging) align(l logger) {
	source := log.sources[l]
	srclen := len(source)

	if srclen > log.maxlen {
		log.realign(srclen)
		return
	}

	pad := log.maxlen - srclen
	pre := (pad + 1) / 2
	suf := pad - pre
	log.aligned[l] = "[" + fmt.Sprintf("%*s", pre, "") + source + fmt.Sprintf("%*s", suf, "") + "] "
}

// realign recalculates aligned prefixes for all loggers.
func (log *logging) realign(maxlen int) {
	if maxlen <= 0 {
		for _, source := range log.sources {
			if srclen := len(source); srclen > maxlen {
				maxlen = srclen
			}
		}
	}
	log.maxlen = maxlen
	log.aligned = make(map[logger]string)
	for l := range log.sources {
		log.align(l)
	}
}

//
// Logger
//

// get returns the logger for source, creating one if necessary.
func (log *logging) get(source string) logger {
	if l, ok := log.loggers[source]; ok {
		return l
	}

	l := logger(len(log.loggers))
	log.loggers[source] = l
	log.sources[l] = source
	log.align(l)

	state, ok := log.dbgmap[source]
	if !ok {
		state = log.dbgmap["*"]
	}
	log.setDebug(source, state)

	return l
}

func (l logger) EnableDebug(state bool) bool {
	log.Lock()
	defer log.Unlock()
	if _, ok := log.sources[l]; !ok {
		return false
	}
	_, old := log.debug[l]
	log.debug[l] = struct{}{}
	return old
}

func (l logger) DebugEnabled() bool {
	log.RLock()
	defer log.RUnlock()
	_, enabled := log.debug[l]
	return enabled || log.forced
}

func (l logger) Source() string {
	log.RLock()
	defer log.RUnlock()
	return log.sources[l]
}

func (l logger) Debug(format string, args ...interface{}) {
	log.RLock()
	defer log.RUnlock()

	if !log.forced {
		if _, ok := log.debug[l]; !ok {
			return
		}
	}

	msg := fmt.Sprintf(format, args...)

	if log.prefix {
		klog.InfoDepth(1, levelTag[LevelDebug], log.aligned[l], msg)
	} else {
		klog.InfoDepth(1, msg)
	}
}

func (l logger) Info(format string, args ...interface{}) {
	log.RLock()
	defer log.RUnlock()

	msg := fmt.Sprintf(format, args...)

	if log.prefix {
		klog.InfoDepth(1, levelTag[LevelInfo], log.aligned[l], msg)
	} else {
		klog.InfoDepth(1, msg)
	}
}

func (l logger) Warn(format string, args ...interface{}) {
	log.RLock()
	defer log.RUnlock()

	msg := fmt.Sprintf(format, args...)

	if log.prefix {
		klog.WarningDepth(1, levelTag[LevelWarn], log.aligned[l], msg)
	} else {
		klog.WarningDepth(1, msg)
	}
}

func (l logger) Error(format string, args ...interface{}) {
	log.RLock()
	defer log.RUnlock()

	msg := fmt.Sprintf(format, args...)
	if log.prefix {
		klog.ErrorDepth(1, levelTag[LevelError], log.aligned[l], msg)
	} else {
		klog.ErrorDepth(1, msg)
	}
}

func (l logger) Fatal(format string, args ...interface{}) {
	log.RLock()
	defer log.RUnlock()

	msg := fmt.Sprintf(format, args...)
	if log.prefix {
		klog.ExitDepth(1, levelTag[LevelFatal], log.aligned[l], msg)
	} else {
		klog.ExitDepth(1, msg)
	}
}

func (l logger) Panic(format string, args ...interface{}) {
	log.RLock()
	defer log.RUnlock()

	msg := fmt.Sprintf(format, args...)
	if log.prefix {
		klog.ErrorDepth(1, levelTag[LevelPanic], log.aligned[l], msg)
	} else {
		klog.ErrorDepth(1, msg)
	}
	panic(msg)
}

func (l logger) DebugBlock(prefix string, format string, args ...interface{}) {
	if l.DebugEnabled() {
		l.block(LevelDebug, prefix, format, args...)
	}
}

func (l logger) InfoBlock(prefix string, format string, args ...interface{}) {
	l.block(LevelInfo, prefix, format, args...)
}

func (l logger) WarnBlock(prefix string, format string, args ...interface{}) {
	l.block(LevelWarn, prefix, format, args...)
}

func (l logger) ErrorBlock(prefix string, format string, args ...interface{}) {
	l.block(LevelError, prefix, format, args...)
}

func (l logger) block(level Level, prefix, format string, args ...interface{}) {
	log.Lock()
	defer log.Unlock()

	var logFn func(int, ...interface{})

	switch level {
	case LevelDebug, LevelInfo:
		logFn = klog.InfoDepth
	case LevelWarn:
		logFn = klog.WarningDepth
	case LevelError:
		logFn = klog.ErrorDepth
	default:
		return
	}

	if log.prefix {
		src := log.aligned[l]
		for _, msg := range strings.Split(fmt.Sprintf(format, args...), "\n") {
			logFn(2, levelTag[level], src, prefix, msg)
		}
	} else {
		for _, msg := range strings.Split(fmt.Sprintf(format, args...), "\n") {
			logFn(2, prefix, msg)
		}
	}
}

// loggerError produces a formatted logger-specific error.
func loggerError(format string, args ...interface{}) error {
	return fmt.Errorf("logger: "+format, args...)
}

func (l logger) Debugf(format string, args ...interface{}) {
	l.Debug(format, args...)
}

func (l logger) Infof(format string, args ...interface{}) {
	l.Info(format, args...)
}

func (l logger) Warnf(format string, args ...interface{}) {
	l.Warn(format, args...)
}

func (l logger) Errorf(format string, args ...interface{}) {
	l.Error(format, args...)
}

func (l logger) Panicf(format string, args ...interface{}) {
	l.Panic(format, args...)
}

func (l logger) Fatalf(format string, args ...interface{}) {
	l.Fatal(format, args...)
}
