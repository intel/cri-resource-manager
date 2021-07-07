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
	"os"
	"sigs.k8s.io/yaml"
	"strings"
	"sync"
	"time"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// stampLayout is the timestamp format used in dump files.
	stampLayout = "2006-Jan-02 15:04:05.000"
	// stampLen is the length we adjust our printed latencies to.
	stampLen = len(stampLayout)
)

// dumper encapsulates the runtime state of our message dumper.
type dumper struct {
	sync.RWMutex                  // protect concurrent dumping/reconfiguration
	rules        ruleset          // dumping rules
	details      map[string]level // corresponding dump details per method
	disabled     bool             // dumping globally disabled
	debug        bool             // dump as debug messages
	path         string           // extra dump file path
	file         *os.File         // extra dump file
	methods      []string         // training set for config
	q            chan *dumpreq
}

// dumpreq is a request to dump a (CRI) request or a reply
type dumpreq struct {
	dir       direction
	kind      string
	method    string
	qualifier string
	msg       interface{}
	latency   time.Duration
	sync      chan struct{}
}

// direction is a message direction, a request or a reply
type direction int

const (
	request = iota
	reply
	nop
)

// Our global dumper instance.
var dump = newDumper()

// Our logger instances, one for generic logging and another for message dumps.
var log = logger.NewLogger("dump")
var message = logger.NewLogger("message")

// Train trains the message dumper for the given set of methods.
func Train(methods []string) {
	dump.Lock()
	defer dump.Unlock()
	dump.train(methods)
}

// RequestMessage dumps a CRI request.
func RequestMessage(kind, name, qualifier string, req interface{}, sync bool) {
	if !dump.disabled {
		var ch chan struct{}
		if sync {
			ch = make(chan struct{})
		}
		dump.q <- &dumpreq{
			dir:       request,
			kind:      kind,
			method:    name,
			qualifier: qualifier,
			msg:       req,
			sync:      ch,
		}
		if ch != nil {
			_ = <-ch
		}
	}
}

// ReplyMessage dumps a CRI reply.
func ReplyMessage(kind, name, qualifier string, rpl interface{}, latency time.Duration, sync bool) {
	if !dump.disabled {
		var ch chan struct{}
		if sync {
			ch = make(chan struct{})
		}
		dump.q <- &dumpreq{
			dir:       reply,
			kind:      kind,
			method:    name,
			qualifier: qualifier,
			msg:       rpl,
			latency:   latency,
			sync:      ch,
		}
		if ch != nil {
			_ = <-ch
		}
	}
}

// Sync returns once the last message currently being dumped is finished.
func Sync() {
	if !dump.disabled {
		dump.sync()
	}
}

// newDumper creates a dumper instance.
func newDumper() *dumper {
	d := &dumper{q: make(chan *dumpreq, 16)}
	d.run()
	return d
}

// run runs the dumping goroutine of the dumper.
func (d *dumper) run() {
	go func() {
		for req := range d.q {
			if req.dir != nop {
				method := methodName(req.method)
				d.RLock()
				detail, ok := d.details[method]
				if !ok {
					detail = d.rules.detailOf(method)
				}
				d.RUnlock()
				switch detail {
				case Name:
					d.name(req.dir, req.kind, method, req.qualifier, req.msg, req.latency)
				case Full:
					d.full(req.dir, req.kind, method, req.qualifier, req.msg, req.latency)
				}
			}
			if req.sync != nil {
				close(req.sync)
			}
		}
	}()
}

// sync waits until all the persent messages in the queue are dumped.
func (d *dumper) sync() {
	ch := make(chan struct{})
	dump.q <- &dumpreq{dir: nop, sync: ch}
	_ = <-ch
}

// configure (re)configures dumper
func (d *dumper) configure(o *options) {
	d.Lock()
	defer d.Unlock()

	d.debug = o.Debug
	d.rules = o.rules.duplicate()

	if d.path != o.File || d.disabled != o.Disabled {
		if d.file != nil {
			log.Infof("closing old message dump file %q...", d.path)
			d.file.Close()
			d.file = nil
		}
		d.disabled = o.Disabled

		if d.disabled {
			return
		}

		d.path = o.File
		if d.path != "" {
			var err error
			log.Infof("opening new message dump file %q...", d.path)
			d.file, err = os.Create(d.path)
			if err != nil {
				log.Errorf("failed to open file %q: %v", d.path, err)
			}
		}
	}

	d.train(nil)
}

// train trains the dumper with the given set of messages.
func (d *dumper) train(names []string) {
	if names != nil {
		d.methods = make([]string, len(names), len(names))
	} else {
		names = d.methods
	}
	d.details = make(map[string]level)
	for idx, name := range names {
		method := methodName(name)
		detail := d.rules.detailOf(method)
		log.Infof("%s: %v", method, detail)
		d.methods[idx] = method
		d.details[method] = detail
	}
}

// name does a name-only dump of the given message.
func (d *dumper) name(dir direction, kind, method, qualifier string, msg interface{}, latency time.Duration) {
	var hdr string

	switch dir {
	case request:
		return
	case reply:
		if qualifier != "" {
			hdr = qualifier + " " + method + " " + dir.arrow() + " "
		} else {
			hdr = method + " " + dir.arrow() + " "
		}
		if err, ok := msg.(error); ok {
			d.warn(dir, latency, hdr+"(%s) FAILED: %v", kind, err)
		} else {
			d.line(dir, latency, hdr+"(%s) REQUEST", kind)
		}
	}
}

// full does a full dump of the given message.
func (d *dumper) full(dir direction, kind, method, qualifier string, msg interface{}, latency time.Duration) {
	var hdr string

	if qualifier != "" {
		hdr = qualifier + " " + method + " " + dir.arrow() + " "
	} else {
		hdr = method + " " + dir.arrow() + " "
	}

	switch dir {
	case request:
		raw, _ := yaml.Marshal(msg)
		str := strings.TrimRight(string(raw), "\n")
		if strings.LastIndexByte(str, '\n') > 0 {
			d.line(dir, latency, hdr+"(%s) REQUEST", kind)
			d.block(dir, latency, hdr+"    ", str)
		} else {
			d.line(dir, latency, hdr+"(%s) REQUEST %s", kind, str)
		}

	case reply:
		if err, ok := msg.(error); ok {
			d.warn(dir, latency, hdr+"(%s) FAILED", kind)
			d.warn(dir, latency, hdr+"    %v", err)
		} else {
			raw, _ := yaml.Marshal(msg)
			str := strings.TrimRight(string(raw), "\n")
			if strings.LastIndexByte(str, '\n') > 0 {
				d.line(dir, latency, hdr+"(%s) REPLY", kind)
				d.block(dir, latency, hdr+"    ", str)
			} else {
				d.line(dir, latency, hdr+"(%s) REPLY %s", kind, str)
			}
		}
	}
}

// line dumps a single line.
func (d *dumper) line(dir direction, latency time.Duration, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if !d.debug {
		message.Infof("%s", msg)
	} else {
		message.Debugf("%s", msg)
	}
	if d.file != nil {
		d.tofile(dir, latency, "%s", msg)
	}
}

// block dumps a block of lines.
func (d *dumper) block(dir direction, latency time.Duration, prefix, msg string) {
	if !d.debug {
		message.InfoBlock(prefix, msg)
	} else {
		message.DebugBlock(prefix, msg)
	}
	if d.file != nil {
		for _, line := range strings.Split(msg, "\n") {
			d.tofile(dir, latency, "%s%s", prefix, line)
		}
	}
}

// warn dumps a single line as a warning.
func (d *dumper) warn(dir direction, latency time.Duration, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	message.Warnf("%s", msg)
	if d.file != nil {
		d.tofile(dir, latency, "%s", msg)
	}
}

// tofile dumps a single line to a file.
func (d *dumper) tofile(dir direction, latency time.Duration, format string, args ...interface{}) {
	fmt.Fprintf(d.file, "["+stamp(dir, latency)+"] "+format+"\n", args...)
}

// stamp produces a stamp from a direction and a latency.
func stamp(dir direction, latency time.Duration) string {
	switch dir {
	case request:
		return time.Now().Format(stampLayout)
	case reply:
		return fmt.Sprintf("%*s", stampLen, fmt.Sprintf("+%f", latency.Seconds()))
	}
	return ""
}

// String returns a string representing the direction.
func (d direction) String() string {
	switch d {
	case request:
		return "request"
	case reply:
		return "reply"
	}
	return "unknown"
}

// arrow returns an 'ASCII arrow' for the direction.
func (d direction) arrow() string {
	switch d {
	case request:
		return "=>"
	case reply:
		return "<="
	}
	return "<=???=>"
}

// methodName returns the basename of a method.
func methodName(method string) string {
	return method[strings.LastIndex(method, "/")+1:]
}

// dumpError produces a formatted package-specific error.
func dumpError(format string, args ...interface{}) error {
	return fmt.Errorf("dump: "+format, args...)
}
