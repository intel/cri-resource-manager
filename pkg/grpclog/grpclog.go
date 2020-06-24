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

package grpclog

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	logger "github.com/intel/cri-resource-manager/pkg/log"
	"google.golang.org/grpc/grpclog"
)

const (
	// grpcLog is the name of the logger we use for grpc logging.
	grpcLog = "grpc-lib"
)

var (
	// Default rate limit interval.
	defaultInterval = 5 * time.Minute
	// Default bursts per rate-limit interval.
	defaultBurst = 1
	// Minimum level to pass through.
	level logger.Level = logger.LevelWarn
	// V()-logging verbosity.
	verbosity = -1
)

type l struct {
	log logger.Logger
}

// SetLogger sets up a rate-limited logger for gRPC log messages.
func SetLogger() {
	RateLimit(defaultInterval, defaultBurst)
}

// RateLimit sets up a rate-limited logger for gRPC log messages.
func RateLimit(interval time.Duration, burst int) {
	if interval == 0 {
		interval = defaultInterval
	}
	if burst <= 0 {
		burst = defaultBurst
	}

	rate := logger.Rate{Limit: logger.Every(interval), Burst: burst}
	grpcl := &l{
		log: logger.RateLimit(logger.NewLogger(grpcLog), rate),
	}
	grpclog.SetLoggerV2(grpcl)

	logger.Info("grpc logging rate-limited to %d time%s every %s...",
		burst, map[bool]string{false: "", true: "s"}[burst != 1], interval)
}

// Info is the LoggerV2 Info() implementation.
func (l *l) Info(args ...interface{}) {
	if level <= logger.LevelInfo {
		l.log.Info("%s", fmt.Sprint(args...))
	}
}

// Infoln is the LoggerV2 Infoln() implementation.
func (l *l) Infoln(args ...interface{}) {
	if level <= logger.LevelInfo {
		l.log.Info("%s", fmt.Sprint(args...))
	}
}

// Infof is the LoggerV2 Infof() implementation.
func (l *l) Infof(format string, args ...interface{}) {
	if level <= logger.LevelInfo {
		l.log.Info(format, args)
	}
}

// Warning is the LoggerV2 Warning() implementation.
func (l *l) Warning(args ...interface{}) {
	if level <= logger.LevelWarn {
		l.log.Warn("%s", fmt.Sprint(args...))
	}
}

// Warningln is the LoggerV2 Warningln() implementation.
func (l *l) Warningln(args ...interface{}) {
	if level <= logger.LevelWarn {
		l.log.Warn("%s", fmt.Sprint(args...))
	}
}

// Warningf is the LoggerV2 Warningf() implementation.
func (l *l) Warningf(format string, args ...interface{}) {
	if level <= logger.LevelWarn {
		l.log.Warn(format, args...)
	}
}

// Error is the LoggerV2 Error() implementation.
func (l *l) Error(args ...interface{}) {
	l.log.Error("%s", fmt.Sprint(args...))
}

// Errorln is the LoggerV2 Errorln() implementation.
func (l *l) Errorln(args ...interface{}) {
	l.log.Error("%s", fmt.Sprint(args...))
}

// Errorf is the LoggerV2 Errorf() implementation.
func (l *l) Errorf(format string, args ...interface{}) {
	l.log.Error(format, args...)
}

// Fatal is the LoggerV2 Fatal() implementation.
func (l *l) Fatal(args ...interface{}) {
	l.log.Fatal("%s", fmt.Sprint(args...))
}

// Fatalln is the LoggerV2 Fatalln() implementation.
func (l *l) Fatalln(args ...interface{}) {
	l.log.Fatal("%s", fmt.Sprint(args...))
}

// Fatalf is the LoggerV2 Fatalf() implementation.
func (l *l) Fatalf(format string, args ...interface{}) {
	l.log.Fatal(format, args...)
}

// V is the LoggerV2 V() implementation.
func (l *l) V(lvl int) bool {
	return verbosity > 0 && lvl >= verbosity
}

// Read and set up defaults from environment variables.
func init() {
	levels := map[string]logger.Level{
		logger.LevelDebug.String(): logger.LevelDebug,
		logger.LevelInfo.String():  logger.LevelInfo,
		logger.LevelWarn.String():  logger.LevelWarn,
		logger.LevelError.String(): logger.LevelError,
	}

	name := "GRPCLOG_INTERVAL"
	if str, ok := os.LookupEnv(name); ok {
		if interval, err := time.ParseDuration(str); err != nil {
			logger.Error("grpclog: failed to parse %s (%q): %v", name, str, err)
		} else {
			if defaultInterval != time.Duration(0) {
				defaultInterval = interval
			}
		}
	}
	name = "GRPCLOG_BURST"
	if str, ok := os.LookupEnv(name); ok {
		if burst, err := strconv.ParseInt(str, 10, 0); err != nil {
			logger.Error("grpclog: failed to parse %s (%q): %v", name, str, err)
		} else {
			if burst > 0 {
				defaultBurst = int(burst)
			}
		}
	}

	name = "GRPCLOG_LEVEL"
	if str, ok := os.LookupEnv(name); ok {
		if lvl, ok := levels[strings.ToLower(str)]; !ok {
			logger.Error("grpclog: ignoring filtering level %s = %s",
				name, str)
		} else {
			level = lvl
		}
	}

	name = "GRPCLOG_VERBOSE"
	if str, ok := os.LookupEnv(name); ok {
		if v, err := strconv.ParseInt(str, 10, 0); err != nil {
			logger.Error("grpclog: failed to parse verosity %s (%q): %v", name, str, err)
		} else {
			verbosity = int(v)
		}
	}

	l := logger.NewLogger(grpcLog)
	l.Info("default rate-limit: %d time%s every %s",
		defaultBurst, map[bool]string{false: "", true: "s"}[defaultBurst != 1], defaultInterval)
	l.Info("filtering level: %s, verbosity: %d", level.String(), verbosity)
}
