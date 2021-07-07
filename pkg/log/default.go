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
	"os"
	"path/filepath"
)

// our default logger
var deflog = log.get(filepath.Base(filepath.Clean(os.Args[0])))

// Default returns the default Logger.
func Default() Logger {
	return deflog
}

// Infof formats and emits an informational message.
func Infof(format string, args ...interface{}) {
	deflog.Infof(format, args...)
}

// Warnf formats and emits a warning message.
func Warnf(format string, args ...interface{}) {
	deflog.Warnf(format, args...)
}

// Errorf formats and emits an error message.
func Errorf(format string, args ...interface{}) {
	deflog.Errorf(format, args...)
}

// Fatalf formats and emits an error message and os.Exit()'s with status 1.
func Fatalf(format string, args ...interface{}) {
	deflog.Fatalf(format, args...)
}

// Panicf formats and emits an error messages, and panics with the same.
func Panicf(format string, args ...interface{}) {
	deflog.Panicf(format, args...)
}

// Debugf formats and emits a debug message.
func Debugf(format string, args ...interface{}) {
	deflog.Debugf(format, args...)
}

// InfoBlock formats and emits a multiline information message.
func InfoBlock(prefix string, format string, args ...interface{}) {
	deflog.InfoBlock(prefix, format, args...)
}

// WarnBlock formats and emits a multiline warning message.
func WarnBlock(prefix string, format string, args ...interface{}) {
	deflog.WarnBlock(prefix, format, args...)
}

// ErrorBlock formats and emits a multiline error message.
func ErrorBlock(prefix string, format string, args ...interface{}) {
	deflog.ErrorBlock(prefix, format, args...)
}

// DebugBlock formats and emits a multiline debug message.
func DebugBlock(prefix string, format string, args ...interface{}) {
	deflog.DebugBlock(prefix, format, args...)
}

func init() {
	binary := filepath.Clean(os.Args[0])
	source := filepath.Base(binary)
	deflog = log.get(source)
}
