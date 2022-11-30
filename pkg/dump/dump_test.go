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

package dump

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/intel/cri-resource-manager/pkg/config"
)

// TestConfigParsing test parsing of dump configuration strings.
func TestConfigParsing(t *testing.T) {
	tcases := []string{
		DefaultConfig,
		"off:.*",
		"full:.*",
		"name:.*",
		"off:.*,full:CreateContainer,StartContainer,StopContainer,RemoveContainer",
		"off:.*,full:.*((PodSandbox)|(Container)),off:.*((Status)|(List)).*",
	}

	for _, cfg := range tcases {
		t.Run("parse config "+cfg, func(t *testing.T) {
			r := ruleset{}
			if err := r.parse(cfg); err != nil {
				t.Errorf("failed to parse dump config string '%s': %v", cfg, err)
			}
			if chk := r.String(); chk != cfg {
				switch {
				case strings.Replace(cfg, "short:", "name:", 1) == chk:
				case strings.Replace(cfg, "suppress:", "off:", 1) == chk:
				case strings.Replace(cfg, "verbose:", "full:", 1) == chk:
				default:
					t.Errorf("expected %s, got %s", cfg, chk)
				}
			}
		})
	}
}

// TestFiltering test message filtering, and a bit of formatting.
func fooTestFiltering(t *testing.T) {
	messages := []interface{}{
		mkmsg(&Type1Message1{}),
		mkmsg(&Type1Message2{}),
		mkmsg(&Type1Message3{}),
		mkmsg(&Type1Whatever{}),
		mkmsg(&Type2Message1{}),
		mkmsg(&Type2Message2{}),
		mkmsg(&Type2Message3{}),
		mkmsg(&Type2Whatever{}),
		mkmsg(&Type3Message1{}),
		mkmsg(&Type3Message2{}),
		mkmsg(&Type3Message3{}),
		mkmsg(&Type3Whatever{}),
	}

	tcases := []filterTest{
		{
			messages: messages,
			config:   "off:.*",
		},
		{
			messages: messages,
			config:   "name:Type1.*",
			details: map[string]level{
				msgmethod(&Type1Message1{}): Name,
				msgmethod(&Type1Message2{}): Name,
				msgmethod(&Type1Message3{}): Name,
				msgmethod(&Type1Whatever{}): Name,
			},
		},
		{
			messages: messages,
			config:   "full:.*Whatever.*",
			details: map[string]level{
				msgmethod(&Type1Whatever{}): Full,
				msgmethod(&Type2Whatever{}): Full,
				msgmethod(&Type3Whatever{}): Full,
			},
		},
		{
			messages: messages,
			config:   "full:.*Whatever.*,off:Type1.*",
			details: map[string]level{
				msgmethod(&Type2Whatever{}): Full,
				msgmethod(&Type3Whatever{}): Full,
			},
		},
		{
			messages: messages,
			config:   "full:.*Message.*,off:Type2.*,name:Type2Whatever",
			details: map[string]level{
				msgmethod(&Type1Message1{}): Full,
				msgmethod(&Type1Message2{}): Full,
				msgmethod(&Type1Message3{}): Full,
				msgmethod(&Type2Whatever{}): Name,
				msgmethod(&Type3Message1{}): Full,
				msgmethod(&Type3Message2{}): Full,
				msgmethod(&Type3Message3{}): Full,
			},
		},
	}

	for _, tc := range tcases {
		t.Run("filter with config "+tc.config, func(t *testing.T) {
			tc.run(t)
		})
	}
}

type filterTest struct {
	messages []interface{}
	config   string
	details  map[string]level
}

const (
	// test log marker to identify logged messages
	marker = "<testmsg>"
)

func (ft *filterTest) setup(train bool) *testlog {
	// override message logger
	logger := &testlog{}
	message = logger

	// create training set/reset messages
	methods := []string{}
	if train {
		for _, msg := range ft.messages {
			methods = append(methods, msgname(msg))
		}
	}
	Train(methods)

	// trigger reconfiguration
	opt.Config = ft.config
	opt.configNotify(config.UpdateEvent, config.ConfigFile)

	return logger
}

func (ft *filterTest) dumpMessages(logger *testlog) []string {
	// dump all test messages and a fake reply for each
	for _, msg := range ft.messages {
		RequestMessage(marker, msgname(msg), "", msg, false)
		ReplyMessage(marker, msgname(msg), "", Reply, time.Duration(0), false)
	}
	dump.sync()

	return logger.info
}

func (ft *filterTest) parseLogs(t *testing.T, logged []string) (map[string]int, map[string]int) {
	// count logged entries and lines per message
	lines := map[string]int{}
	entries := map[string]int{}
	for _, entry := range logged {
		entry = strings.Trim(entry, " ")
		split := strings.Split(entry, " ")
		method := ""
		switch {
		// log line: (marker) {REQUEST|REPLY} method
		case len(split) > 1 && split[0] == "("+marker+")":
			method = split[2]
			entries[method] = entries[method] + 1
		case len(split) > 1:
			// log line continuation: method {=>|<=} content...
			method = split[0]
		}
		if method == "" {
			t.Errorf("failed to parse log entry '%s' for config '%s'", entry, ft.config)
		}

		detail, ok := ft.details[method]
		if !ok || detail == Off {
			t.Errorf("message '%s' should have been filtered for config '%s'",
				method, ft.config)
		}
	}

	return lines, entries
}

func (ft *filterTest) checkResult(t *testing.T, entries map[string]int, lines map[string]int) {
	// check correctness of logged entries and lines per method
	for method, lineCnt := range lines {
		logcnt := entries[method]
		expected := 0
		switch ft.details[method] {
		case Full:
			expected = logcnt/2*(1+LinesPerRequest) + logcnt/2*(1+LinesPerReply)
		case Name:
			expected = logcnt
		}
		if lineCnt != expected {
			t.Errorf("message '%s' expected %d logged lines, got %d for config '%s'",
				method, expected, lineCnt, ft.config)
		}
	}
}

func (ft *filterTest) run(t *testing.T) {
	for _, train := range []bool{false, true} {
		logger := ft.setup(train)
		logged := ft.dumpMessages(logger)
		lines, entries := ft.parseLogs(t, logged)

		ft.checkResult(t, entries, lines)
	}
}

//
// a few message types for testing
//

type Message struct {
	Body []string
}

type Type1Message1 Message
type Type1Message2 Message
type Type1Message3 Message
type Type1Whatever Message
type Type2Message1 Message
type Type2Message2 Message
type Type2Message3 Message
type Type2Whatever Message
type Type3Message1 Message
type Type3Message2 Message
type Type3Message3 Message
type Type3Whatever Message

const (
	LinesPerRequest = 6
	LinesPerReply   = 2
)

var (
	Reply  = []string{"reply", "OK"}
	msgCnt int
)

func mkmsg(o interface{}) interface{} {
	msgCnt++
	body := []string{
		"this",
		"is",
		"message",
		fmt.Sprintf("#%d", msgCnt),
		fmt.Sprintf("of type (%T)", o),
	}

	switch o.(type) {
	case *Type1Message1:
		m := o.(*Type1Message1)
		m.Body = body
	case *Type1Message2:
		m := o.(*Type1Message2)
		m.Body = body
	case *Type1Message3:
		m := o.(*Type1Message3)
		m.Body = body
	case *Type1Whatever:
		m := o.(*Type1Whatever)
		m.Body = body

	case *Type2Message1:
		m := o.(*Type2Message1)
		m.Body = body
	case *Type2Message2:
		m := o.(*Type2Message2)
		m.Body = body
	case *Type2Message3:
		m := o.(*Type2Message3)
		m.Body = body
	case *Type2Whatever:
		m := o.(*Type2Whatever)
		m.Body = body

	case *Type3Message1:
		m := o.(*Type3Message1)
		m.Body = body
	case *Type3Message2:
		m := o.(*Type3Message2)
		m.Body = body
	case *Type3Message3:
		m := o.(*Type3Message3)
		m.Body = body
	case *Type3Whatever:
		m := o.(*Type3Whatever)
		m.Body = body
	}

	return o
}

func msgname(o interface{}) string {
	return strings.ReplaceAll(fmt.Sprintf("%T", o), ".", "/")
}

func msgmethod(o interface{}) string {
	return methodName(msgname(o))
}

//
// test logger to override and check dumping/logging for test.
//

type testlog struct {
	sync.Mutex
	info  []string
	warn  []string
	err   []string
	debug []string
}

func (t *testlog) reset() {
	t.Lock()
	defer t.Unlock()
	t.info = nil
	t.warn = nil
	t.err = nil
	t.debug = nil
}

func (t *testlog) log(save *[]string, prefix, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	*save = append(*save, msg)
	fmt.Println("<dump-test> " + prefix + " " + msg)
}

func (t *testlog) Info(format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	t.log(&t.info, "I:", format, args...)
}

func (t *testlog) Warn(format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	t.log(&t.warn, "W:", format, args...)
}

func (t *testlog) Error(format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	t.log(&t.err, "E:", format, args...)
}

func (t *testlog) Debug(format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	t.log(&t.debug, "D:", format, args...)
}

func (t *testlog) Fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("<dump-test> Fatal error: %s\n", msg)
	os.Exit(1)
}

func (*testlog) Panic(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("<dump-test> Panic: %s\n", msg)
	panic(msg)
}

func (t *testlog) Infof(format string, args ...interface{}) {
	t.Info(format, args...)
}

func (t *testlog) Warnf(format string, args ...interface{}) {
	t.Warn(format, args...)
}

func (t *testlog) Errorf(format string, args ...interface{}) {
	t.Error(format, args...)
}

func (t *testlog) Debugf(format string, args ...interface{}) {
	t.Debug(format, args...)
}

func (t *testlog) Fatalf(format string, args ...interface{}) {
	t.Fatal(format, args...)
}

func (t *testlog) Panicf(format string, args ...interface{}) {
	t.Panic(format, args...)
}

func (*testlog) Block(fn func(string, ...interface{}), prfx string, frmt string, a ...interface{}) {
	for _, line := range strings.Split(fmt.Sprintf(frmt, a...), "\n") {
		fn("%s%s", prfx, line)
	}
}

func (t *testlog) InfoBlock(prefix string, format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	for _, line := range strings.Split(fmt.Sprintf(format, args...), "\n") {
		t.log(&t.info, "I:", "%s%s", prefix, line)
	}
}

func (t *testlog) WarnBlock(prefix string, format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	for _, line := range strings.Split(fmt.Sprintf(format, args...), "\n") {
		t.log(&t.info, "W:", "%s%s", prefix, line)
	}
}

func (t *testlog) ErrorBlock(prefix string, format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	for _, line := range strings.Split(fmt.Sprintf(format, args...), "\n") {
		t.log(&t.err, "E:", "%s%s", prefix, line)
	}
}

func (t *testlog) DebugBlock(prefix string, format string, args ...interface{}) {
	t.Lock()
	defer t.Unlock()
	for _, line := range strings.Split(fmt.Sprintf(format, args...), "\n") {
		t.log(&t.debug, "I:", "%s%s", prefix, line)
	}
}

func (*testlog) EnableDebug(bool) bool { return true }
func (*testlog) DebugEnabled() bool    { return true }
func (*testlog) Stop()                 {}
func (*testlog) Source() string        { return "" }
