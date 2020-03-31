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
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// a test Backend that can log and optionally record messages for verification
type testlogger struct {
	sync.RWMutex                     // debug by signal testcase needs locking
	protect      bool                // true if we should lock during Log()/check()
	recorded     []string            // messages record()ed by Log() for check()
	logfn        func(Level, string) // logger function (log(), save(), or log()+save())
	test         *testing.T          // runtime test case/state
}

var testlog *testlogger

func createTestLogger() Backend {
	tl := &testlogger{}
	tl.logfn = tl.log
	testlog = tl
	return testlog
}

const testLoggerName = "testlogger"

func (l *testlogger) Name() string {
	return testLoggerName
}

func (l *testlogger) Log(level Level, source, format string, args ...interface{}) {
	l.logfn(level, fmt.Sprintf("["+source+"] "+format, args...))
}

func (l *testlogger) Block(level Level, source, prefix, format string, args ...interface{}) {
	l.logfn(level, fmt.Sprintf("["+source+"] "+format, args...))
}

func (l *testlogger) Flush()                 {}
func (l *testlogger) Sync()                  {}
func (l *testlogger) Stop()                  {}
func (l *testlogger) SetSourceAlignment(int) {}

func setup(test *testing.T, quiet bool, record int, parallel bool) *testlogger {
	if err := SetBackend(testLoggerName); err != nil {
		test.Errorf("failed to activate test backend '%s': %v", testLoggerName, err)
		return nil
	}
	l := testlog

	l.test = test
	if record > 0 {
		l.recorded = make([]string, 0, record)
	} else {
		l.recorded = nil
	}
	switch {
	case quiet && record == 0:
		l.logfn = func(Level, string) {}
	case !quiet && record > 0:
		l.logfn = func(level Level, msg string) { l.log(level, msg); l.record(level, msg) }
		l.protect = parallel
	case !quiet:
		l.logfn = l.log
	default:
		l.logfn = l.record
		l.protect = parallel
	}

	return l
}

func (l *testlogger) lock() {
	if l.protect {
		l.RWMutex.Lock()
	}
}
func (l *testlogger) unlock() {
	if l.protect {
		l.RWMutex.Unlock()
	}
}
func (l *testlogger) rlock() {
	if l.protect {
		l.RWMutex.RLock()
	}
}
func (l *testlogger) runlock() {
	if l.protect {
		l.RWMutex.RUnlock()
	}
}

func (l *testlogger) log(level Level, msg string) {
	fmt.Println("<log-test>", fmtTags[level], msg)
}

func (l *testlogger) record(level Level, msg string) {
	l.lock()
	defer l.unlock()
	l.recorded = append(l.recorded, msg)
}

func (l *testlogger) check(expected []string, ordered bool, checkSources map[string]struct{}) {
	var recorded []string

	l.rlock()
	defer l.runlock()

	if !ordered {
		recorded = make([]string, 0, len(l.recorded))
		copy(recorded, l.recorded)
		sort.Strings(recorded)
		sort.Strings(expected)
	} else {
		recorded = l.recorded
	}

	for i, j := 0, 0; i < len(recorded) && j < len(expected); {
		split := strings.SplitN(recorded[i], "] ", 2)
		i++

		source, message := strings.Trim(split[0], "[] "), split[1]
		if _, check := checkSources[source]; checkSources != nil && !check {
			continue
		}

		if message != expected[j] {
			l.test.Errorf("%s failed, #%d message is '%s', expected '%s'",
				l.test.Name(), j, message, expected[j])
			return
		}
		j++
	}
}

// TestBackendOverride tests the effect of overriding the active log backend.
func TestBackendOverride(t *testing.T) {
	tl := setup(t, false, 1024, false)

	SetLevel(LevelInfo)
	test := NewLogger("test")
	messages := []string{
		"this is a test info message",
		"this is a test warning message",
		"this is a test error message",
	}
	test.Info(messages[0])
	test.Warn(messages[1])
	test.Error(messages[2])

	tl.check(messages, true, nil)
}

// TestSeverityFiltering tests the severity-level based filtering.
func TestSeverityFiltering(t *testing.T) {
	tl := setup(t, false, 1024, false)

	test := NewLogger("test")
	// level to logger function mapping
	logfns := map[Level]func(string){
		LevelDebug: func(s string) { test.Debug(s) },
		LevelInfo:  func(s string) { test.Info(s) },
		LevelWarn:  func(s string) { test.Warn(s) },
		LevelError: func(s string) { test.Error(s) },
	}
	// a bunch of debug-toggling functions to loop through
	setDebugFns := []func() bool{
		func() bool { test.EnableDebug(false); return false },
		func() bool { test.EnableDebug(true); return true },
		func() bool { flag.Set(optDebug, "off:*"); return false },
		func() bool { flag.Set(optDebug, "on:*"); return true },
		func() bool { flag.Set(optDebug, "on:*"); test.EnableDebug(false); return false },
		func() bool { flag.Set(optDebug, "off:*"); test.EnableDebug(true); return true },
	}
	// a bunch of logging level settings to loop through
	loggingLevels := []Level{
		LevelDebug, LevelInfo, LevelWarn, LevelError,
		LevelError, LevelWarn, LevelInfo, LevelDebug,
	}
	// function to generate a single message
	mkmsg := func(threshold, level Level, msg string, count int) string {
		return fmt.Sprintf("filtering: %s, message: %s -> "+msg+" #%d", threshold, level, count)
	}

	cnt := 0
	expected := []string{}
	for _, setDebugFn := range setDebugFns {
		debugging := setDebugFn()
		for _, threshold := range loggingLevels {
			SetLevel(threshold)
			for _, msg := range []string{
				"test",
				"message",
				"test message",
				"test message once more",
				"test message a final time",
			} {
				for _, msgLevel := range []Level{LevelDebug, LevelInfo, LevelWarn, LevelError} {
					msg := mkmsg(threshold, msgLevel, msg, cnt)
					logfns[msgLevel](msg)
					cnt++
					switch {
					case msgLevel == LevelDebug && debugging:
						expected = append(expected, msg)
					case msgLevel != LevelDebug && msgLevel >= threshold:
						expected = append(expected, msg)
					}
				}
			}
		}
	}

	tl.check(expected, true, nil)
}

// TestForcedDebugToggling tests toggling debug on/off by a signal.
func TestForcedDebugToggling(t *testing.T) {
	tl := setup(t, false, 1024, true)

	SetLevel(LevelInfo)
	test := NewLogger("test")

	debugSignal := syscall.SIGUSR1
	SetupDebugToggleSignal(debugSignal)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, debugSignal)
	flag.Set(optDebug, "off:*")
	debugging := false

	expected := []string{}
	messages := []string{"debug", "info", "warning", "error"}
	for i := 0; i < 2; i++ {
		for _, msg := range messages {
			var logfn func(string, ...interface{})

			filtered := false
			switch msg {
			case "debug":
				logfn = test.Debug
				filtered = !debugging
			case "info":
				logfn = test.Info
			case "warning":
				logfn = test.Warn
			case "error":
				logfn = test.Error
			default:
				continue
			}
			logfn("%s", msg)
			if !filtered {
				expected = append(expected, msg)
			}
		}
		log.forceDebug(!log.debugForced())
		debugging = !debugging
	}

	sources := map[string]struct{}{
		"test": {},
	}

	tl.check(expected, true, sources)
}

func getenv(key string, fallback interface{}) interface{} {
	strval := os.Getenv(key)
	if strval == "" {
		return fallback
	}
	switch defv := fallback.(type) {
	case int:
		v, err := strconv.ParseInt(strval, 10, 0)
		if err != nil {
			fmt.Printf("error: invalid environment variable %s = %s: %v\n", key, strval, err)
			return defv
		}
		return int(v)
	case time.Duration:
		v, err := time.ParseDuration(strval)
		if err != nil {
			fmt.Printf("error: invalid enviroment variable %s = %s: %v\n", key, strval, err)
			return defv
		}
		return v
	case []bool:
		v := []bool{}
		for _, strb := range strings.Split(strval, ",") {
			b, err := strconv.ParseBool(strb)
			if err != nil {
				fmt.Printf("error: invalid environment variable %s = %s: %v\n", key, strval, err)
				return defv
			}
			v = append(v, b)
		}
		return v

	default:
		panic(fmt.Sprintf("enviroment variable %s=%s with unhandled type %T",
			key, strval, fallback))
	}
}

var (
	// number of concurrent loggers, togglers, test duration, test verbosity
	numLoggers    = getenv("LOGTEST_LOGGERS", 32).(int)
	numTogglers   = getenv("LOGTEST_TOGGLERS", 4).(int)
	testDuration  = getenv("LOGTEST_DURATION", 5*time.Second).(time.Duration)
	testVerbosity = getenv("LOGTEST_VERBOSITY", []bool{true, false}).([]bool)
)

// createLoggers creates a bunch of loggers.
func createLoggers(cnt int) []Logger {
	loggers := make([]Logger, cnt, cnt)
	for idx := range loggers {
		switch idx % 7 {
		case 0:
			loggers[idx] = NewLogger(fmt.Sprintf("%d-<0>-test", idx))
		case 1:
			loggers[idx] = NewLogger(fmt.Sprintf("%d-<1>-logger", idx))
		case 2:
			loggers[idx] = NewLogger(fmt.Sprintf("%d-<2>-logging", idx))
		case 3:
			loggers[idx] = NewLogger(fmt.Sprintf("%d-<3>-log", idx))
		case 4:
			loggers[idx] = NewLogger(fmt.Sprintf("%d-<4>-logger", idx))
		case 5:
			loggers[idx] = NewLogger(fmt.Sprintf("%d-<5>-tester", idx))
		default:
			loggers[idx] = NewLogger(fmt.Sprintf("%d-<->-logger-tester", idx))
		}

		// fuzz a but more by enabling debugging only for a quasi-random subset of loggers
		if (idx % 11) == 0 {
			loggers[idx].EnableDebug(true)
		}

	}
	return loggers
}

// exercise exercises a set of loggers in pseudo-random order.
func exercise(loggers []Logger, levels []Level, start, stop chan struct{}, wg *sync.WaitGroup) {
	rnd := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))
	idx := rnd.Perm(len(loggers))

	for _, i := range idx {
		loggers[i].Info("logger <%s> waiting for start...", loggers[i].Source())
	}
	_ = <-start

	done := false
	cnt := 0
	for !done {
		for _, i := range idx {
			log := loggers[idx[i]]
			for _, level := range levels {
				switch level {
				case LevelDebug:
					log.Debug("logged debug message #%d", cnt)
				case LevelInfo:
					log.Info("logged info message #%d", cnt)
				case LevelWarn:
					log.Warn("logged warning message #%d", cnt)
				case LevelError:
					log.Error("logged error message #%d", cnt)
				}
			}
		}
		cnt++

		select {
		case _ = <-stop:
			done = true
		default:
		}
	}

	for _, i := range idx {
		loggers[i].Info("logger <%s> stopped...", loggers[i].Source())
	}

	if wg != nil {
		wg.Done()
	}
}

// toggle toggles debug-logging on/off for a set of loggers in pseudo-random order.
func toggle(loggers []Logger, start, stop chan struct{}, m *sync.Mutex, wg *sync.WaitGroup) {
	rnd := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))
	idx := rnd.Perm(len(loggers))

	for _, i := range idx {
		loggers[i].Info("toggler <%s> waiting for start...", loggers[i].Source())
	}
	_ = <-start

	done := false
	cnt := 0
	for !done {
		nth := 3 + cnt%7
		cfg := "on:*,"
		sep := "off:"
		for i := 0; i < len(idx)/4; i++ {
			log := loggers[idx[i]]
			if i != 0 && i%nth == 0 {
				cfg += sep + log.Source()
				sep = ","
			}
		}
		Info("toggling %s...", cfg)
		m.Lock()
		flag.Set("logging-debug", cfg)
		m.Unlock()
		cnt++

		select {
		case _ = <-stop:
			done = true
		default:
		}
	}

	for _, i := range idx {
		loggers[i].Info("toggler <%s> stopped...", loggers[i].Source())
	}

	if wg != nil {
		wg.Done()
	}
}

// prepareLoggerGoroutines prepares a bunch of goroutines for logging.
func prepareLoggerGoroutines(loggers []Logger, start, stop chan struct{}, wg *sync.WaitGroup) {
	levelCombos := [][]Level{
		{LevelDebug},
		{LevelInfo},
		{LevelWarn},
		{LevelError},
		{LevelDebug, LevelInfo},
		{LevelDebug, LevelInfo, LevelWarn, LevelError},
		{LevelDebug, LevelInfo, LevelDebug, LevelWarn, LevelDebug, LevelError},
	}

	min, max := 0, 0
	for i := 0; i < len(loggers); i += 5 {
		if min = i - 5; min < 0 {
			min = 0
		}
		if max = i + 5; max > len(loggers) {
			max = len(loggers)
		}
		subset := loggers[min:max]
		levels := levelCombos[i%len(levelCombos)]

		wg.Add(1)
		go func(loggers []Logger, levels []Level) {
			exercise(loggers, levels, start, stop, wg)
		}(subset, levels)
	}
}

// prepareTogglerGoroutines prepares a bunch of goroutines for toggling.
func prepareTogglerGoroutines(loggers []Logger, start, stop chan struct{}, wg *sync.WaitGroup) {
	var flagMutex sync.Mutex
	for i := 0; i < numTogglers; i++ {
		go func() {
			toggle(loggers, start, stop, &flagMutex, wg)
		}()
		wg.Add(1)
	}
}

// TestConcurrentLogging tests logging from multiple goroutines.
func TestConcurrentLogging(t *testing.T) {
	var wg sync.WaitGroup

	loggers := createLoggers(numLoggers)

	numCPUs := runtime.NumCPU()
	for _, verbose := range testVerbosity {
		runtime.GOMAXPROCS(numCPUs)
		setup(t, !verbose, 0, false)
		start := make(chan struct{})
		stop := make(chan struct{})

		prepareLoggerGoroutines(loggers, start, stop, &wg)
		prepareTogglerGoroutines(loggers, start, stop, &wg)

		Info("starting %d loggers with %d togglers, %v duration...",
			numLoggers, numTogglers, testDuration)
		close(start)
		time.Sleep(testDuration)
		close(stop)
		wg.Wait()

		numCPUs++
	}
}

// TestConcurrentFmtBackendLogging tests logging from multiple goroutines with the fmt backend.
func TestConcurrentFmtBackendLogging(t *testing.T) {
	var wg sync.WaitGroup

	loggers := createLoggers(numLoggers)

	numCPUs := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPUs)
	SetBackend(FmtBackendName)
	start := make(chan struct{})
	stop := make(chan struct{})

	prepareLoggerGoroutines(loggers, start, stop, &wg)
	prepareTogglerGoroutines(loggers, start, stop, &wg)

	Info("starting %d loggers with %d togglers, %v duration...",
		numLoggers, numTogglers, testDuration)
	close(start)
	time.Sleep(testDuration)
	close(stop)
	wg.Wait()
}

// TestLoggingAndMutating tests logging and then mutating an objects with the fmt backend.
func TestLoggingAndMutating(t *testing.T) {
	numCPUs := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPUs)

	tl := NewLogger("testlog")
	obj := &testObj{0: "zero", 1: "one", 2: "two", 3: "three"}

	stop := make(chan struct{})
	go func() {
		idx := 0
		for {
			switch {
			case (idx & 0x7) == 0:
				(*obj)[3] = "3"
			case (idx & 0x3) == 0:
				(*obj)[3] = "three"
			}
			tl.Info("#%d: obj: %s", idx, obj)
			select {
			case _ = <-stop:
				return
			default:
			}
			idx++
		}
	}()

	time.Sleep(5 * time.Second)
	close(stop)
}

type testObj map[int]string

func (o *testObj) String() string {
	str := "{"
	sep := ""
	for i, s := range *o {
		str += sep + fmt.Sprintf("%d:%v", i, s)
		sep = ","
	}
	str += "}"
	return str
}

func init() {
	RegisterBackend(testLoggerName, createTestLogger)
}
