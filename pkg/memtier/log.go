// Copyright 2021 Intel Corporation. All Rights Reserved.
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

package memtier

import (
	stdlog "log"
)

type Logger interface {
	Debugf(format string, v ...interface{})
	Infof(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
	Panicf(format string, v ...interface{})
	Fatalf(format string, v ...interface{})
}

type logger struct {
	*stdlog.Logger
}

const logPrefix = "memtier "

var log Logger = &logger{Logger: nil}
var logDebugMessages bool = false

func SetLogger(l *stdlog.Logger) {
	log = NewLoggerWrapper(l)
}

func SetLogDebug(debug bool) {
	logDebugMessages = debug
}

func NewLoggerWrapper(l *stdlog.Logger) Logger {
	return &logger{Logger: l}
}

func (l *logger) Debugf(format string, v ...interface{}) {
	if l.Logger != nil && logDebugMessages {
		l.Logger.Printf("DEBUG: "+logPrefix+format, v...)
	}
}

func (l *logger) Infof(format string, v ...interface{}) {
	if l.Logger != nil {
		l.Logger.Printf("INFO: "+logPrefix+format, v...)
	}
}

func (l *logger) Warnf(format string, v ...interface{}) {
	if l.Logger != nil {
		l.Logger.Printf("WARN: "+logPrefix+format, v...)
	}
}

func (l *logger) Errorf(format string, v ...interface{}) {
	if l.Logger != nil {
		l.Logger.Printf("ERROR: "+logPrefix+format, v...)
	}
}

func (l *logger) Panicf(format string, v ...interface{}) {
	if l.Logger != nil {
		l.Logger.Panicf(format, v...)
	}
}

func (l *logger) Fatalf(format string, v ...interface{}) {
	if l.Logger != nil {
		l.Logger.Fatalf(format, v...)
	}
}
