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
	"strings"
)

//
// Logging backend interface and default fmt-based backend implementation.
//

// BackendFn is a functions that creates a Backend instance.
type BackendFn func() Backend

// Backend can format and emit log messages.
type Backend interface {
	// Name returns the name of this backend.
	Name() string
	// Log emits log messages with the given severity, source, and Printf-like arguments.
	Log(Level, string, string, ...interface{})
	// Block emits a multi-line log messages, with an additional line prefix.
	Block(Level, string, string, string, ...interface{})
	// Flush flushes and stops initial buffering synchronously
	Flush()
	// Sync waits for all messages to get emitted.
	Sync()
	// Stop stops the backend instance.
	Stop()
	// SetSourceAlignment sets the maximum prefix length for optional alignment.
	SetSourceAlignment(int)
}

// RegisterBackend registers a logger backend.
func RegisterBackend(name string, fn BackendFn) {
	log.backend[name] = fn
}

const (
	// FmtBackendName is the name of our simple fmt-based logging backend.
	FmtBackendName = "fmt"
	// fmtBackendQueueLen is the length of the internal fmt message queue.
	fmtBackendQueueLen = 1024
)

const (
	levelNop Level = iota + levelHighest
	levelStop
)

// severity tags fmtBackend uses to prefix emitted messages with.
var fmtTags = map[Level]string{
	LevelDebug: "D: ",
	LevelInfo:  "I: ",
	LevelWarn:  "W: ",
	LevelError: "E: ",
	LevelFatal: "FATAL ERROR: ",
	LevelPanic: "PANIC: ",
}

// fmtBackend is our simple, default fmt.Printf-based Backend.
type fmtBackend struct {
	q     chan *fmtReq // request channel
	align int          // source alignment
}

// fmtReq is the request the fmt logger goroutine uses to emit messages.
type fmtReq struct {
	level  Level         // logging severity level
	source string        // logger source
	prefix string        // block prefix
	msg    string        // formatted log message
	sync   chan struct{} // reverse-ack for synchronous requests
	flush  bool          // Flush() and stop queueing
}

// createFmtBackend creates an fmt Backend and starts its emitter goroutine.
func createFmtBackend() Backend {
	f := &fmtBackend{
		q: make(chan *fmtReq, fmtBackendQueueLen),
	}
	go f.run()
	return f
}

func (*fmtBackend) Name() string {
	return FmtBackendName
}

func (f *fmtBackend) Log(level Level, source, format string, args ...interface{}) {
	f.log(level, source, "", format, args...)
}

func (f *fmtBackend) Block(level Level, source, prefix, format string, args ...interface{}) {
	f.log(level, source, prefix, format, args...)
}

func (f *fmtBackend) Flush() {
	sync := make(chan struct{})
	f.q <- &fmtReq{
		level: levelNop,
		flush: true,
		sync:  sync,
	}
	_ = <-sync
	close(sync)
}

func (f *fmtBackend) Sync() {
	sync := make(chan struct{})
	f.q <- &fmtReq{
		level: levelNop,
		sync:  sync,
	}
	_ = <-sync
	close(sync)
}

func (f *fmtBackend) Stop() {
	sync := make(chan struct{})
	f.q <- &fmtReq{
		level: levelStop,
		sync:  sync,
	}
	_ = <-sync
	close(sync)
}

func (f *fmtBackend) SetSourceAlignment(len int) {
	f.align = len
}

// log pushes a new log message for emitting.
func (f *fmtBackend) log(level Level, source, prefix, format string, args ...interface{}) {
	var sync chan struct{}

	// fatal errors are synchronous
	if level > LevelError {
		sync = make(chan struct{})
	}

	req := &fmtReq{
		level:  level,
		source: source,
		prefix: prefix,
		msg:    fmt.Sprintf(format, args...),
		sync:   sync,
		flush:  level >= LevelError, // errors force buffer flush
	}
	f.q <- req

	if sync != nil {
		_ = <-sync
		close(sync)
	}
}

// run emits log messages for the fmtBackend.
func (f *fmtBackend) run() {
	buf := make([]*fmtReq, 0, fmtBackendQueueLen)

	//
	// logging loop
	//
	//   1. if we're not buffering, emit immediately
	//   2. if we're buffering
	//      - run out of space or asked to flush, flush buffer then emit message
	//

	for req := range f.q {
		if buf == nil {
			// past initial buffering => emit
			f.emit(req)
		} else {
			// flush request or buffer full => flush, stop buffering, emit
			if req.flush || len(buf) == cap(buf) {
				for _, r := range buf {
					f.emit(r)
				}
				f.emit(req)
				buf = nil
			} else {
				buf = append(buf, req)
			}
		}
		if req.sync != nil {
			req.sync <- struct{}{}
		}
		if req.level == levelStop {
			return
		}
	}
}

// emit formats and emits a single log message.
func (f *fmtBackend) emit(req *fmtReq) {
	if req.level > levelHighest {
		return
	}
	length := len(req.source)
	suflen := (f.align - length) / 2
	prelen := (f.align - (length + suflen))
	source := "[" + fmt.Sprintf("%*s", prelen, "") + req.source + fmt.Sprintf("%*s", suflen, "") + "]"

	if req.prefix == "" {
		for _, line := range strings.Split(req.msg, "\n") {
			fmt.Println(fmtTags[req.level], source, line)
		}
	} else {
		for _, line := range strings.Split(req.msg, "\n") {
			fmt.Println(fmtTags[req.level], source, req.prefix, line)
		}
	}
}

func init() {
	RegisterBackend(FmtBackendName, createFmtBackend)
}
