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

package config

import (
	"fmt"
	"os"
)

//
// Notes:
//   Unless we split the Logger interface (pkg/log.Logger) from its actual implementation
//   we cannot import it import here. pkg/log itself implements its runtime configurability
//   using this module so we would end up with an import cycle. As a workaround for now we
//   let out logger be set externally and we set it from pkg/log.
//

// Logger is our set of logging functions.
type Logger struct {
	DebugEnabled func() bool
	Debug        func(string, ...interface{})
	Info         func(string, ...interface{})
	Warning      func(string, ...interface{})
	Error        func(string, ...interface{})
	Fatal        func(string, ...interface{})
	Panic        func(string, ...interface{})
}

// log is our Logger.
var log = defaultLogger()

// SetLogger sets our logger.
func SetLogger(logger Logger) {
	if logger.DebugEnabled != nil {
		log.DebugEnabled = logger.DebugEnabled
	}
	if logger.Debug != nil {
		log.Debug = logger.Debug
	}
	if logger.Info != nil {
		log.Info = logger.Info
	}
	if logger.Warning != nil {
		log.Warning = logger.Warning
	}
	if logger.Error != nil {
		log.Error = logger.Error
	}
	if logger.Panic != nil {
		log.Panic = logger.Panic
	}
	if logger.Fatal != nil {
		log.Fatal = logger.Fatal
	}
}

func defaultLogger() Logger {
	return Logger{
		DebugEnabled: debugEnabled,
		Debug:        debugmsg,
		Info:         infomsg,
		Warning:      warningmsg,
		Error:        errormsg,
		Fatal:        fatalmsg,
		Panic:        panicmsg,
	}
}

func debugEnabled() bool {
	return true
}

func debugmsg(format string, args ...interface{}) {
	fmt.Printf("D: [config] "+format+"\n", args...)
}

func infomsg(format string, args ...interface{}) {
	fmt.Printf("I: [config] "+format+"\n", args...)
}

func warningmsg(format string, args ...interface{}) {
	fmt.Printf("W: [config] "+format+"\n", args...)
}

func errormsg(format string, args ...interface{}) {
	fmt.Printf("E: [config] "+format+"\n", args...)
}

func fatalmsg(format string, args ...interface{}) {
	fmt.Printf("E: [config] fatal error: "+format+"\n", args...)
	os.Exit(1)
}

func panicmsg(format string, args ...interface{}) {
	errormsg(format, args...)
	panic(fmt.Sprintf("fatal error: "+format+"\n", args...))
}
