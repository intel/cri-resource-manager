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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Level is the log message severity level below which we suppress messages.
type Level int32

const (
	// LevelDebug corresponds to debug messages.
	LevelDebug Level = iota
	// LevelInfo corresponds to informational messages.
	LevelInfo
	// LevelWarn corresponds to warning messages.
	LevelWarn
	// LevelError corresponds to error messages.
	LevelError
)

// Logger is the interface for configuring and producing log messages.
type Logger interface {
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Fatal(format string, args ...interface{})
	Panic(format string, args ...interface{})

	EnableDebug(bool) bool
	DebugEnabled() bool
	Debug(format string, args ...interface{})
	Block(fn func(string, ...interface{}), prefix string, format string, args ...interface{})
	DebugBlock(prefix string, format string, args ...interface{})
	InfoBlock(prefix string, format string, args ...interface{})
	WarnBlock(prefix string, format string, args ...interface{})
	ErrorBlock(prefix string, format string, args ...interface{})

	Stop()
}

// Backend is an entity that can emit log messages.
type Backend interface {
	Name() string
	PrefixPreference() bool
	Enabled(Level) bool
	Info(message string)
	Warn(message string)
	Error(message string)

	Debug(message string)
}

// Our logger instance.
type logger struct {
	source  string // logger source/module name
	enabled bool   // logger source module
	level   Level  // first non-suppressed severity level
	debug   bool   // debugging for this instance
	prefix  string // message prefix
}

// log is our runtime state.
type log struct {
	level    Level              // lowest unsuppressed severity
	active   Backend            // active backend
	loggers  map[string]*logger // running loggers (log sources)
	backends map[string]Backend // registered backends (real loggers)
	srcalign int                // longest name of active source seen
}

var logging = &log{
	active: &fmtBackend{},
}

// Get an existing logger or create a new one.
func Get(source string) Logger {
	l, ok := logging.loggers[source]
	if !ok {
		return newLogger(source)
	}
	return l
}

// NewLogger creates a new logger, getting the existing one if possible.
func NewLogger(source string) Logger {
	return Get(source)
}

// newLogger creates a new logger instance.
func newLogger(source string) Logger {
	source = strings.Trim(source, "[] ")

	if logging.loggers == nil {
		logging.loggers = make(map[string]*logger)
	}

	if l := logging.loggers[source]; l != nil {
		return l
	}

	l := &logger{
		source:  source,
		enabled: opt.sourceEnabled(source),
		debug:   opt.debugEnabled(source),
		level:   opt.Level,
	}
	logging.loggers[source] = l

	return l
}

// Optional call to stop a logger once it is not needed any more.
func (l *logger) Stop() {
	l.enabled = false
	delete(logging.loggers, l.source)
}

func (l *logger) shouldPrefix() bool {
	return logging.active.PrefixPreference()
}

func (l *logger) passthrough(level Level) bool {
	return (l.enabled && l.level <= level) || (level == LevelDebug && l.debug)
}

func (l *logger) formatMessage(format string, args ...interface{}) string {
	prefix := ""
	if l.shouldPrefix() {
		prefix = l.prefix
	}

	return prefix + fmt.Sprintf(format, args...)
}

// Emit an info message (lowest priority).
func (l *logger) Info(format string, args ...interface{}) {
	if !l.passthrough(LevelInfo) {
		return
	}
	logging.active.Info(l.formatMessage(format, args...))
}

// Emit a warning message.
func (l *logger) Warn(format string, args ...interface{}) {
	if !l.passthrough(LevelWarn) {
		return
	}
	logging.active.Warn(l.formatMessage(format, args...))
}

// Emit an error message.
func (l *logger) Error(format string, args ...interface{}) {
	if !l.passthrough(LevelError) {
		return
	}
	logging.active.Error(l.formatMessage(format, args...))
}

// Emit a fatal error message and exit.
func (l *logger) Fatal(format string, args ...interface{}) {
	logging.active.Error(l.formatMessage(format, args...))
	os.Exit(1)
}

// Emit a fatal error message and panic.
func (l *logger) Panic(format string, args ...interface{}) {
	message := l.formatMessage(format, args...)
	logging.active.Error(message)
	panic(message)
}

// Default logger/source.
var defLogger = NewLogger("default")

// Default gets the default logger.
func Default() Logger {
	return defLogger
}

// Info emit an info message with the default soruce.
func Info(format string, args ...interface{}) {
	defLogger.Info(format, args...)
}

// Warn emit a warning message with the default source.
func Warn(format string, args ...interface{}) {
	defLogger.Warn(format, args...)
}

// Error emit an error message with the default source.
func Error(format string, args ...interface{}) {
	defLogger.Error(format, args...)
}

// Fatal emit a fatal error message with the default source.
func Fatal(format string, args ...interface{}) {
	defLogger.Fatal(format, args...)
}

// Panic emit a fatal error message with the default source and panic.
func Panic(format string, args ...interface{}) {
	defLogger.Panic(format, args...)
}

// Debug emit a debug message with the default source.
func Debug(format string, args ...interface{}) {
	defLogger.Debug(format, args...)
}

// EnableDebug controls debugging for the default source.
func EnableDebug(enable bool) bool {
	return defLogger.EnableDebug(enable)
}

// DebugEnabled returns the debugging state of the default source.
func DebugEnabled() bool {
	return defLogger.DebugEnabled()
}

// Block emits a block of messages with the given emitting/printing function.
func Block(fn func(string, ...interface{}), prefix string, format string, args ...interface{}) {
	defLogger.Block(fn, prefix, format, args...)
}

// DebugBlock emits a block of debug messages with the default source.
func DebugBlock(prefix string, format string, args ...interface{}) {
	defLogger.DebugBlock(prefix, format, args...)
}

// InfoBlock emits a block of info messages with the default source.
func InfoBlock(prefix string, format string, args ...interface{}) {
	defLogger.InfoBlock(prefix, format, args...)
}

// WarnBlock emits a block of warning messages with the default source.
func WarnBlock(prefix string, format string, args ...interface{}) {
	defLogger.WarnBlock(prefix, format, args...)
}

// ErrorBlock emits a block of error messages with the default source.
func ErrorBlock(prefix string, format string, args ...interface{}) {
	defLogger.ErrorBlock(prefix, format, args...)
}

// EnableDebug controls debugging for the logger and returns the its previous debugging state.
func (l *logger) EnableDebug(enable bool) bool {
	previous := l.debug
	opt.Debug.Set(l.source + ":" + map[bool]string{false: "false", true: "true"}[enable])
	return previous
}

// Check if debugging is enabled.
func (l *logger) DebugEnabled() bool {
	return l.debug
}

// Emit a debug message.
func (l *logger) Debug(format string, args ...interface{}) {
	if !l.debug {
		return
	}
	logging.active.Debug(l.formatMessage(format, args...))
}

// Block emits a block of messages with using the given emitting function.
func (l *logger) Block(fn func(string, ...interface{}), prefix string, format string, args ...interface{}) {
	for _, line := range strings.Split(fmt.Sprintf(format, args...), "\n") {
		fn("%s%s", prefix, line)
	}
}

// Emit a block of debug messages.
func (l *logger) DebugBlock(prefix string, format string, args ...interface{}) {
	if !l.debug {
		return
	}

	l.Block(l.Debug, prefix, format, args...)
}

// Emit a block of info messages.
func (l *logger) InfoBlock(prefix string, format string, args ...interface{}) {
	l.Block(l.Info, prefix, format, args...)
}

// Emit a block of warning messages.
func (l *logger) WarnBlock(prefix string, format string, args ...interface{}) {
	l.Block(l.Warn, prefix, format, args...)
}

// Emit a block of error messages.
func (l *logger) ErrorBlock(prefix string, format string, args ...interface{}) {
	l.Block(l.Error, prefix, format, args...)
}

// RegisterBackend registers a logger backend.
func RegisterBackend(b Backend) {
	name := b.Name()

	if logging.backends == nil {
		logging.backends = make(map[string]Backend)
	}

	logging.backends[name] = b

	if string(opt.Logger) == name {
		logging.active = b
	}
}

// activateBackend selects the logger backend to activate.
func activateBackend(name string) {
	if b, ok := logging.backends[string(opt.Logger)]; ok {
		logging.active = b
	} else {
		logging.active = &fmtBackend{}
	}

	logging.active.Info(fmt.Sprintf("activated logger backend '%s'", logging.active.Name()))
}

// Update loggers when debug flags or sources change.
func (o *options) updateLoggers() {
	logging.srcalign = 0
	for s, l := range logging.loggers {
		l.enabled = opt.sourceEnabled(s)
		l.debug = opt.debugEnabled(s)
		l.level = opt.Level
		l.prefix = ""
		if (l.enabled || l.debug) && len(l.source) > logging.srcalign {
			logging.srcalign = len(l.source)
		}
	}
	for _, l := range logging.loggers {
		suf := (logging.srcalign - len(l.source)) / 2
		pre := logging.srcalign - (len(l.source) + suf)
		l.prefix = "[" + fmt.Sprintf("%-*s", pre, "") + l.source + fmt.Sprintf("%*s", suf, "") + "] "
	}
}

//
// fallback fmt backend, using fmt.*Printf
//

const (
	fmtBackendName = "fmt"
)

type fmtBackend struct{}

var _ Backend = &fmtBackend{}

func (f *fmtBackend) Name() string {
	return "fmt"
}

func (f *fmtBackend) PrefixPreference() bool {
	return true
}

func (f *fmtBackend) Info(message string) {
	fmt.Println("I: " + message)
}

func (f *fmtBackend) Warn(message string) {
	fmt.Println("W: " + message)
}

func (f *fmtBackend) Error(message string) {
	fmt.Println("E: " + message)
}

func (f *fmtBackend) Debug(message string) {
	fmt.Println("D: " + message)
}

func (f *fmtBackend) Enabled(l Level) bool {
	return l >= opt.Level
}

func init() {
	RegisterBackend(&fmtBackend{})

	binary := filepath.Clean(os.Args[0])
	source := filepath.Base(binary)
	defLogger = newLogger(source)
}
