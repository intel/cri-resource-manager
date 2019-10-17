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
	"flag"
	"os"
	re "regexp"
	"strings"
	"time"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// DefaultDumpConfig is the default dump configuration.
	DefaultDumpConfig = "full:.*,off:Version,.*List.*,.*Status.*,.*Info.*,.*Log.*,.*Reopen.*"
)

// Types to tell requests and replies/errors apart in a HandlerFunc.
type handlerRequest struct {
	kind   string
	method string
	msg    interface{}
}

type handlerReply struct {
	kind    string
	method  string
	msg     interface{}
	latency time.Duration
}

// A HandlerFunc dumps method <request, response> tuplets.
type HandlerFunc func(interface{})

// Handler is the interface used to introspect and access a HandlerFunc.
type Handler interface {
	// Provide the name the dump handler should be referenced by.
	Name() string
	// Provide a verbose description of this handler.
	Description() string
	// Dump the given method with its request and reply.
	Dump(msg interface{})
}

// Dumper is the request dumper interface.
type Dumper interface {
	// RegisterHandler registers the given dump handler function.
	RegisterHandler(Handler)
	// Verify verifies that all the named dump handlers are resolvable.
	Verify() error
	// Parse parses the given dump specification string.
	Parse(string) error
	// DumpRequest handles the dumping of the given method request.
	DumpRequest(string, string, interface{})
	// DumpReply handles the dumping of the given method reply.
	DumpReply(string, string, interface{}, time.Duration)
	// Dumper implements command line option handling.
	flag.Value
}

// Request dumper.
type dumper struct {
	methods  map[string]*spec   // specs for exact method names
	regexps  []*spec            // specs for matching regexps
	handlers map[string]Handler // registered dump handlers
	resolve  bool               // unresolved handlers present
	enabled  bool               // dumping globally enabled
}

// A request dump spec
type spec struct {
	method  string     // method name, or regular expression
	regexp  *re.Regexp // compiled regexp, if any
	handler string     // handler name
	h       Handler    // resolved handler
}

// Our default dumper (created later from an init()).
var defaultDumper Dumper

// Name of file to also save dumps to.
var dumpFileName string

// File to also save dumps to.
var dumpFile *os.File

// Our logger instance.
var log = logger.NewLogger("message")

// DefaultDumper returns the default dumper.
func DefaultDumper() Dumper {
	if dumpFileName != "" && dumpFile == nil {
		var err error

		if dumpFile, err = os.Create(dumpFileName); err != nil {
			log.Error("failed to open dump file '%s': %v", dumpFileName, err)
		}

		log.Info("also saving message dumps to file '%s'...", dumpFileName)
	}

	return defaultDumper
}

// NewDumper creates a new request dumper instance.
func NewDumper(specs ...string) (Dumper, error) {
	log.Info("creating request dumper...")

	d := &dumper{
		methods:  make(map[string]*spec),
		regexps:  []*spec{},
		handlers: make(map[string]Handler),
		resolve:  false,
		enabled:  true,
	}

	RegisterDefaultHandlers(d)

	for _, spec := range specs {
		if err := d.Parse(spec); err != nil {
			return &dumper{}, err
		}
	}

	return d, nil
}

// Register the given dump handler.
func (d *dumper) RegisterHandler(h Handler) {
	name := h.Name()
	_, duplicate := d.handlers[name]
	d.handlers[name] = h

	if duplicate {
		for _, spec := range d.methods {
			if spec.handler == name {
				spec.h = h
			}
		}

		for _, spec := range d.regexps {
			if spec.handler == name {
				spec.h = h
			}
		}
	}
}

// Verify the validity of the effective dump configuration.
func (d *dumper) Verify() error {
	if !d.resolve {
		return nil
	}

	for _, spec := range d.methods {
		if spec.h == nil {
			if h, ok := d.handlers[spec.handler]; ok {
				spec.h = h
			} else {
				return dumpError("unknown dump handler '%s' for method '%s'",
					spec.handler, spec.method)
			}
		}
	}

	for _, spec := range d.regexps {
		if spec.h == nil {
			if h, ok := d.handlers[spec.handler]; ok {
				spec.h = h
			} else {
				dumpError("unknown dump handler '%s' for method matcher '%s'",
					spec.handler, spec.method)
			}
		}
	}

	d.resolve = false

	return nil
}

// Parse the given request dump specification.
func (d *dumper) Parse(specStr string) error {
	handler := DefaultHandler

	for _, req := range strings.Split(specStr, ",") {
		switch {
		case req == "enable" || req == "on":
			d.Enable()
			continue
		case req == "disable" || req == "off":
			d.Disable()
			continue
		case req == "reset":
			d.Reset()
			continue
		}

		spec := &spec{}
		hreq := strings.Split(req, ":")
		method := ""

		// pick optional handler and method spec apart
		switch len(hreq) {
		case 1:
			method = hreq[0]
		case 2:
			handler = hreq[0]
			method = hreq[1]
		default:
			return dumpError("invalid dump spec '%s'", req)
		}

		spec.method = method
		spec.handler = handler

		if h, ok := d.handlers[handler]; ok {
			spec.h = h
		} else {
			d.resolve = true
		}

		// parse method, handle regexps
		if method == "*" || !strings.ContainsAny(method, ".*?+()[]|") {
			log.Info("dumping method '%s': %s", method, handler)
			d.methods[method] = spec
		} else {
			log.Info("dumping methods matching '%s': %s", method, handler)
			regexp, err := re.Compile(method)
			if err != nil {
				return dumpError("invalid method regexp '%s': %v", method, err)
			}
			spec.regexp = regexp
			d.regexps = append(d.regexps, spec)
		}
	}

	return nil
}

// Get the effective dump specification for a method.
func (d *dumper) spec(method string) *spec {
	var spec *spec

	if s, ok := d.methods[method]; ok {
		spec = s
	} else if s, ok := d.methods["*"]; ok {
		spec = s
	} else {
		for _, s := range d.regexps {
			if s.regexp.MatchString(method) {
				spec = s
			}
		}
	}

	if spec == nil {
		return nil
	}

	if spec.h == nil {
		d.Verify()
		if spec.h == nil {
			log.Warn("unknown dump handler '%s' (for dumping '%s')",
				spec.handler, spec.method)
			return nil
		}
	}

	return spec
}

// Dump the given method request.
func (d *dumper) DumpRequest(kind, name string, request interface{}) {
	method := name[strings.LastIndex(name, "/")+1:]

	if spec := d.spec(method); spec != nil {
		spec.h.Dump(&handlerRequest{kind: kind, method: method, msg: request})
	}
}

// Dump the given method reply.
func (d *dumper) DumpReply(kind, name string, reply interface{}, latency time.Duration) {
	method := name[strings.LastIndex(name, "/")+1:]

	if spec := d.spec(method); spec != nil {
		spec.h.Dump(&handlerReply{kind: kind, method: method, msg: reply, latency: latency})
	}
}

// Enable dumping globally.
func (d *dumper) Enable() {
	d.enabled = true
}

// Disable dumping globally.
func (d *dumper) Disable() {
	d.enabled = false
}

// Reset all dump specifications.
func (d *dumper) Reset() {
	d.methods = make(map[string]*spec)
	d.regexps = []*spec{}
}

//
// flag.Value interface for command-line handling.
//

// Parse the given request dump specification given on the command line.
func (d *dumper) Set(value string) error {
	return d.Parse(value)
}

// Return the current dump configuration as a string.
func (d *dumper) String() string {
	cfg := ""
	sep := ""

	for _, spec := range d.methods {
		cfg += sep + spec.handler + ":" + spec.method
		sep = ","
	}
	for _, spec := range d.regexps {
		cfg += sep + spec.handler + ":" + spec.method
		sep = ","
	}

	return cfg
}
