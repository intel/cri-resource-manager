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
	Debugf       func(string, ...interface{})
	Infof        func(string, ...interface{})
	Warningf     func(string, ...interface{})
	Errorf       func(string, ...interface{})
	Fatalf       func(string, ...interface{})
	Panicf       func(string, ...interface{})
}

// log is our Logger.
var log = defaultLogger()

// SetLogger sets our logger.
func SetLogger(logger Logger) {
	if logger.DebugEnabled != nil {
		log.DebugEnabled = logger.DebugEnabled
	}
	if logger.Debugf != nil {
		log.Debugf = logger.Debugf
	}
	if logger.Infof != nil {
		log.Infof = logger.Infof
	}
	if logger.Warningf != nil {
		log.Warningf = logger.Warningf
	}
	if logger.Errorf != nil {
		log.Errorf = logger.Errorf
	}
	if logger.Panicf != nil {
		log.Panicf = logger.Panicf
	}
	if logger.Fatalf != nil {
		log.Fatalf = logger.Fatalf
	}
}

func defaultLogger() Logger {
	return Logger{
		DebugEnabled: debugEnabled,
		Debugf:       debugmsg,
		Infof:        infomsg,
		Warningf:     warningmsg,
		Errorf:       errormsg,
		Fatalf:       fatalmsg,
		Panicf:       panicmsg,
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
