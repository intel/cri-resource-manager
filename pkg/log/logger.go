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
	"math"
	"os"
)

// Level describes the severity of log messages.
type Level int

const (
	// LevelDebug is the severity for debug messages.
	LevelDebug Level = iota
	// LevelInfo is the severity for informational messages.
	LevelInfo
	// LevelWarn is the severity for warnings.
	LevelWarn
	// LevelError is the severity for errors.
	LevelError
	// LevelFatal is the severity for fatal errors.
	LevelFatal
	// LevelPanic is the severity for panic messages.
	LevelPanic
)

// Logger is the interface for producing log messages for/from a particular source.
type Logger interface {
	// Debug formats and emits a debug message.
	Debug(format string, args ...interface{})
	// Info formats and emits an informational message.
	Info(format string, args ...interface{})
	// Warn formats and emits a warning message.
	Warn(format string, args ...interface{})
	// Error formats and emits an error message.
	Error(format string, args ...interface{})
	// Fatal formats and emits an error message and os.Exit()'s with status 1.
	Fatal(format string, args ...interface{})
	// Panic formats and emits an error messages, and panics with the same.
	Panic(format string, args ...interface{})

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

// logger implements our Logger.
type logger uint

// EnableDebug enables/disables debug logging for this logger.
func (l logger) EnableDebug(state bool) bool {
	log.Lock()
	defer log.Unlock()

	cfg, _ := log.configs[l]
	old := cfg.setTracing(state)
	log.configs[l] = cfg

	return old
}

// DebugEnabled checks debug logging is enabled for this logger.
func (l logger) DebugEnabled() bool {
	log.RLock()
	defer log.RUnlock()

	cfg, _ := log.configs[l]

	return cfg.isTracing()
}

// Source returns the source for the given logger.
func (l logger) Source() string {
	log.RLock()
	defer log.RUnlock()

	source, _ := log.sources[l]

	return source
}

// Debug logs a debug message.
func (l logger) Debug(format string, args ...interface{}) {
	level := LevelDebug
	if cfg, active, emit := l.config(level); emit {
		active.Log(level, cfg.source(), format, args...)
	}
}

// Info logs a informational message.
func (l logger) Info(format string, args ...interface{}) {
	level := LevelInfo
	if cfg, active, emit := l.config(level); emit {
		active.Log(LevelInfo, cfg.source(), format, args...)
	}
}

// Warn logs a warning message.
func (l logger) Warn(format string, args ...interface{}) {
	level := LevelWarn
	if cfg, active, emit := l.config(level); emit {
		active.Log(level, cfg.source(), format, args...)
	}
}

// Error logs an error message.
func (l logger) Error(format string, args ...interface{}) {
	level := LevelError
	if cfg, active, emit := l.config(level); emit {
		active.Log(level, cfg.source(), format, args...)
	}
}

// Fatal logs a fatal error message and os.Exit(1)'s.
func (l logger) Fatal(format string, args ...interface{}) {
	level := LevelFatal
	cfg, active, _ := l.config(level)
	active.Log(level, cfg.source(), format, args...)

	os.Exit(1)
}

// Panic logs a panic message and panic()'s.
func (l logger) Panic(format string, args ...interface{}) {
	level := LevelPanic
	cfg, active, _ := l.config(level)
	active.Log(level, cfg.source(), format, args...)

	panic(fmt.Sprintf(cfg.source()+" "+format, args...))
}

// DebugBlock logs a multi-line debug message.
func (l logger) DebugBlock(prefix string, format string, args ...interface{}) {
	level := LevelDebug
	if cfg, active, emit := l.config(level); emit {
		active.Block(level, cfg.source(), prefix, format, args...)
	}
}

// InfoBlock logs a multi-line informational message.
func (l logger) InfoBlock(prefix string, format string, args ...interface{}) {
	level := LevelInfo
	if cfg, active, emit := l.config(level); emit {
		active.Block(level, cfg.source(), prefix, format, args...)
	}
}

// WarnBlock logs a multi-line warning message.
func (l logger) WarnBlock(prefix string, format string, args ...interface{}) {
	level := LevelWarn
	if cfg, active, emit := l.config(level); emit {
		active.Block(level, cfg.source(), prefix, format, args...)
	}
}

// ErrorBlock logs a multi-line error message.
func (l logger) ErrorBlock(prefix string, format string, args ...interface{}) {
	level := LevelError
	if cfg, active, emit := l.config(level); emit {
		active.Block(level, cfg.source(), prefix, format, args...)
	}
}

// config returns the logger's configuration and if the level is logged.
func (l logger) config(level Level) (config, Backend, bool) {
	if level != LevelDebug && level < log.level {
		return config{}, nil, false
	}

	log.RLock()
	cfg, _ := log.configs[l]
	active := log.active
	forced := log.forced
	log.RUnlock()

	switch level {
	case LevelInfo:
		return cfg, active, cfg.isLogging()
	case LevelDebug:
		return cfg, active, cfg.isTracing() || forced
	default:
		return cfg, active, true
	}
}

//
// Runtime configuration of a single logger instance.
//

const (
	maxLoggers = math.MaxUint16
	loggingBit = (1 << iota)
	tracingBit
)

// config is the configuration parameters for a single logger stuffed into a single uint64
type config struct {
	id     uint16
	enable uint16
}

// mkConfig creates a configuration with the given parameters.
func mkConfig(logger logger, logging, tracing bool) config {
	cfg := config{id: uint16(logger)}
	if logging {
		cfg.enable = loggingBit
	}
	if tracing {
		cfg.enable |= tracingBit
	}
	return cfg
}

// logger extracts the logger id from this config.
func (cfg *config) logger() logger {
	return logger(cfg.id)
}

// setEnabled sets the logging and tracing bits for this config.
func (cfg *config) setEnabled(logging, tracing bool) {
	if logging {
		cfg.enable = loggingBit
	} else {
		cfg.enable = 0
	}
	if tracing {
		cfg.enable |= tracingBit
	}
}

// isEnabled check if this config has the logging or tracing bit set.
func (cfg *config) isEnabled() bool {
	return cfg.enable != 0
}

// setLogging sets/clears the logging bit in this config.
func (cfg *config) setLogging(enable bool) bool {
	old := (cfg.enable & loggingBit) != 0
	if enable {
		cfg.enable |= loggingBit
	} else {
		cfg.enable &^= loggingBit
	}
	return old
}

// isLogging tests if this config has its logging bit enabled.
func (cfg *config) isLogging() bool {
	return (cfg.enable & loggingBit) != 0
}

// setTracing sets/clears the tracing bit in this config.
func (cfg *config) setTracing(enable bool) bool {
	old := (cfg.enable & tracingBit) != 0
	if enable {
		cfg.enable |= tracingBit
	} else {
		cfg.enable &^= tracingBit
	}
	return old
}

// isTracing tests if this config has its tracing bit enabled.
func (cfg *config) isTracing() bool {
	return (cfg.enable & tracingBit) != 0
}

// prefix produces an aligned prefix for the given source.
func (cfg config) source() string {
	return cfg.logger().Source()
}
