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
	// Flush turns of any optional initial message queuing, emitting any queued messages.
	Flush()
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

// severity tags fmtBackend uses to prefix emitted messages with.
var fmtTags = map[Level]string{
	LevelDebug: "D: ",
	LevelInfo:  "I: ",
	LevelWarn:  "W: ",
	LevelError: "E: ",
	LevelFatal: "Fatal error: ",
	LevelPanic: "PANIC: ",
}

// fmtBackend is our simple, default fmt.Printf-based Backend.
type fmtBackend struct {
	ch    chan *fmtReq // request channel
	align int          // source alignment
}

// fmtReq is the request the fmt logger goroutine uses to emit messages.
type fmtReq struct {
	level  Level         // logging severity level
	source string        // logger source
	prefix string        // block prefix
	format string        // log message format string
	args   []interface{} // any arguments for format
	flush  bool          // Flush() and stop queueing
	stop   bool          // stop run()ning.
	ack    chan struct{} // reverse-ack for synchronous requests
}

// createFmtBackend creates an fmt Backend and starts its emitter goroutine.
func createFmtBackend() Backend {
	f := &fmtBackend{
		ch: make(chan *fmtReq, fmtBackendQueueLen),
	}
	go f.run()
	return f
}

// Name returns the name FmtBackendName.
func (*fmtBackend) Name() string {
	return FmtBackendName
}

// Log pushes a new log message for emitting.
func (f *fmtBackend) Log(level Level, source, format string, args ...interface{}) {
	f.log(level, source, "", format, args...)
}

// Block pushes a new block log message for emitting.
func (f *fmtBackend) Block(level Level, source, prefix, format string, args ...interface{}) {
	f.log(level, source, prefix, format, args...)
}

// log pushes a new log message for emitting.
func (f *fmtBackend) log(level Level, source, prefix, format string, args ...interface{}) {
	if level <= LevelError {
		f.ch <- &fmtReq{
			level:  level,
			source: source,
			prefix: prefix,
			format: format,
			args:   args,
			flush:  level == LevelError,
		}
		return
	}

	f.ch <- &fmtReq{
		level:  level,
		source: source,
		prefix: prefix,
		format: format,
		args:   args,
		flush:  true,
	}
	f.flush(false)
}

// Flush requests flushing the logs.
func (f *fmtBackend) Flush() {
	f.flush(false)
}

// Stop stops this fmtBackend instance.
func (f *fmtBackend) Stop() {
	f.flush(true)
}

// SetSourceAlignment sets the maximum source length for alignment.
func (f *fmtBackend) SetSourceAlignment(len int) {
	f.align = len
}

// flush requests flushing or stopping the logs.
func (f *fmtBackend) flush(stop bool) {
	ack := make(chan struct{})
	f.ch <- &fmtReq{
		flush: true,
		stop:  stop,
		ack:   ack,
	}
	_ = <-ack
	close(ack)
}

// run emits log messages for the fmtBackend.
func (f *fmtBackend) run() {
	q := make([]*fmtReq, 0, fmtBackendQueueLen)
	for {
		req := <-f.ch
		switch {
		case q == nil:
			f.emit(req)
		case !req.flush && len(q) < cap(q) && req.level < LevelError:
			q = append(q, req)
		default:
			for _, r := range q {
				f.emit(r)
			}
			q = nil
			if req.format != "" {
				f.emit(req)
			}
		}
		if req.ack != nil {
			req.ack <- struct{}{}
		}
		if req.stop {
			return
		}
	}
}

// emit formats and emits a single log message.
func (f *fmtBackend) emit(req *fmtReq) {
	length := len(req.source)
	suflen := (f.align - length) / 2
	prelen := (f.align - (length + suflen))
	source := "[" + fmt.Sprintf("%*s", prelen, "") + req.source + fmt.Sprintf("%*s", suflen, "") + "]"

	if req.prefix == "" {
		for _, line := range strings.Split(fmt.Sprintf(req.format, req.args...), "\n") {
			fmt.Println(fmtTags[req.level], source, line)
		}
	} else {
		for _, line := range strings.Split(fmt.Sprintf(req.format, req.args...), "\n") {
			fmt.Println(fmtTags[req.level], source, req.prefix, line)
		}
	}
}

func init() {
	RegisterBackend(FmtBackendName, createFmtBackend)
}
