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

package dump

//
// This package implements the dumping of (gRPC) methods calls where
// each method is called with a single request struct and returns a
// single reply struct or an error. Configuring what to dump happens
// by specifying a comma-separated dump request on the command line.
//
// A dump request is a comma-separated list of dump specs:
//     <spec>[,<spec>,...,<spec>], where each spec is of the form
//     <[target:]request>
// A request is either a requests name (gRPC method name without
// the leading path), or a regexp for matching requests.
// The dump targets are: 'off', 'name', 'full', 'count' by default.
//

import (
	"fmt"
	"github.com/ghodss/yaml"
	"os"
	"strings"
	"time"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// stampLayout is the timestamp format used in dump files.
	stampLayout = "2006-Jan-02 15:04:05.000"
	// stampLen is the length we adjust our printed latencies to.
	stampLen = len(stampLayout)
)

// Our logger instances, one for generic logging and another for message dumps.
var log = logger.NewLogger("dump")
var message = logger.NewLogger("message")

func checkAndScheduleDumpFileSwitch() {
	opt.Lock()
	defer opt.Unlock()
	if fileName != string(opt.File) {
		if file != nil {
			file.Close()
			file = nil
			log.Info("old message dump file '%s' closed", fileName)
		}
	}
}

func checkDumpFile() bool {
	// this must be called with opt.Lock()'ed
	switch {
	case file != nil:
		return true
	case opt.File == "":
		return false
	}

	f, err := os.Create(string(opt.File))
	if err != nil {
		log.Error("failed to open message dump file '%v': %v", opt.File, err)
		opt.File = dumpFile("")
		return false
	}

	log.Info("opened new message dump file '%v'", opt.File)
	file = f
	fileName = string(opt.File)
	return true
}

// RequestMessage dumps a CRI request.
func RequestMessage(kind, name string, request interface{}) {
	method := name[strings.LastIndex(name, "/")+1:]
	opt.Lock()
	defer opt.Unlock()
	switch opt.verbosityOf(method) {
	case NameOnly:
		dumpName("request", kind, method, request, 0)
	case Full:
		dumpFull("request", kind, method, request, 0)
	}
}

// ReplyMessage dumps a CRI reply.
func ReplyMessage(kind, name string, reply interface{}, latency time.Duration) {
	method := name[strings.LastIndex(name, "/")+1:]
	opt.Lock()
	defer opt.Unlock()
	switch opt.verbosityOf(method) {
	case NameOnly:
		dumpName("reply", kind, method, reply, latency)
	case Full:
		dumpFull("reply", kind, method, reply, latency)
	}
}

func dumpName(dir, kind, method string, msg interface{}, latency time.Duration) {
	go func() {
		switch dir {
		case "request":
			return
		case "reply":
			if _, ok := msg.(error); ok {
				dumpWarn(dir, latency, "(%s) FAILED %s: %vs", kind, method, msg.(error))
			} else {
				dumpLine(dir, latency, "(%s) REQUEST %s", kind, method)
			}
		}
	}()
}

func dumpFull(dir, kind, method string, msg interface{}, latency time.Duration) {
	go func() {
		switch dir {
		case "request":
			raw, _ := yaml.Marshal(msg)
			str := strings.TrimRight(string(raw), "\n")
			if strings.LastIndexByte(str, '\n') > 0 {
				dumpLine(dir, latency, "(%s) REQUEST %s", kind, method)
				dumpBlock(dir, latency, "    "+method+" => ", str)
			} else {
				dumpLine(dir, latency, "(%s) REQUEST %s => %s", kind, method, str)
			}

		case "reply":
			switch msg.(type) {
			case error:
				dumpWarn(dir, latency, "(%s) FAILED %s", kind, method)
				dumpWarn(dir, latency, "  %s <= %s", method, msg.(error))
			default:
				raw, _ := yaml.Marshal(msg)
				str := strings.TrimRight(string(raw), "\n")
				if strings.LastIndexByte(str, '\n') > 0 {
					dumpLine(dir, latency, "(%s) REPLY %s", kind, method)
					dumpBlock(dir, latency, "    "+method+" <= ", str)
				} else {
					dumpLine(dir, latency, "(%s) REPLY %s <= %s", kind, method, str)
				}
			}
		}
	}()
}

func dumpLine(dir string, latency time.Duration, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if !opt.Debug {
		message.Info("%s", msg)
	} else {
		message.Debug("%s", msg)
	}
	if opt.File != "" {
		dumpToFile(dir, latency, "%s", msg)
	}
}

func dumpBlock(dir string, latency time.Duration, prefix, msg string) {
	if !opt.Debug {
		message.InfoBlock(prefix, msg)
	} else {
		message.DebugBlock(prefix, msg)
	}
	if opt.File != "" {
		log.Block(
			func(format string, args ...interface{}) {
				dumpToFile(dir, latency, format, args...)
			},
			prefix, "%s", msg)
	}
}

func dumpWarn(dir string, latency time.Duration, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Warn("%s", msg)
	if opt.File != "" {
		dumpToFile(dir, latency, "%s", msg)
	}
}

func dumpToFile(dir string, latency time.Duration, format string, args ...interface{}) {
	if !checkDumpFile() {
		return
	}

	fmt.Fprintf(file, "["+stamp(dir, latency)+"] "+format+"\n", args...)
}

func stamp(dir string, latency time.Duration) string {
	switch dir {
	case "request":
		return time.Now().Format(stampLayout)
	case "reply":
		return fmt.Sprintf("%*s", stampLen, fmt.Sprintf("+%f", latency.Seconds()))
	}
	return ""
}

func dumpError(format string, args ...interface{}) error {
	return fmt.Errorf("dump: "+format, args...)
}
