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
// The default dump handlers are:
//
//   off:   suppress dumping message
//   name:  dump method name and its success/failure
//   full:  dump method name and full request, reply/error
//   count: count calls to method over a collection interval
//

import (
	"fmt"
	"strings"
	"time"

	"github.com/ghodss/yaml"
)

const (
	// DefaultHandler is the default dump handler.
	DefaultHandler = "full"
	// DefaultPeriod is the default count period for the 'count' dumper.
	DefaultPeriod = 60 * time.Second
	// stampLayout is the timestamp format used in dump files.
	stampLayout = "2006-Jan-02 15:04:05.000"
	// latencyLen is the length we adjust our printed latencies to.
	latencyLen = len(stampLayout)
)

// Definition for a handler defined by us.
type handlerDef struct {
	names       string      // names to register handler with
	description string      // handler description
	fn          HandlerFunc // handler function
}

// Default 'count' dumper instance.
var cnt = &counter{calls: make(map[string]*call), Period: DefaultPeriod}

// Default handlers we will register.
var defaults = []handlerDef{
	{
		names:       "off,suppress",
		description: "suppress dumping matching methods",
		fn:          dumpOff,
	},
	{
		names:       "name,short",
		description: "dump names of matching methods",
		fn:          dumpName,
	},
	{
		names:       "full,long",
		description: "dump full matching method, request, and reply",
		fn:          dumpFull,
	},
	{
		names:       "count",
		description: "dump call counts of matching methods",
		fn:          cnt.dumpCount,
	},
}

// DefaultHandlerNames returns the names of default handlers as a comma-separated string.
func DefaultHandlerNames() string {
	str := ""
	sep := ""

	for _, h := range defaults {
		str += sep + strings.Split(h.names, ",")[0]
		sep = ","
	}

	return str
}

// DefaultHandlerFlagHelp gives a list of default handlers, for a flag help message.
func DefaultHandlerFlagHelp(indent string) string {
	str := ""
	sep := indent
	max := 0

	// get longest default handler name for aligning
	for _, h := range defaults {
		l := len(strings.Split(h.names, ",")[0])
		if max < l {
			max = l
		}
	}
	namefmt := "- %" + fmt.Sprintf("-%d.%ds", max, max)

	for _, h := range defaults {
		str += sep + fmt.Sprintf(namefmt, strings.Split(h.names, ",")[0])
		str += ": " + h.description
		sep = "\n" + indent
	}

	return str + "\n"
}

// RegisterDefaultHandlers registers our default handlers.
func RegisterDefaultHandlers(d Dumper) {
	for _, h := range defaults {
		for _, name := range strings.Split(h.names, ",") {
			d.RegisterHandler(&handler{
				name:        strings.Trim(name, " "),
				description: h.description,
				fn:          h.fn,
			})
		}
	}
}

// A handler as implemented by us.
type handler struct {
	name        string      // handler name (dump target)
	description string      // handler description
	fn          HandlerFunc // handler dumper function
}

// Make sure our handlers implement the dump Handler interface.
var _ Handler = &handler{}

// Handler interface: Name().
func (h *handler) Name() string {
	return h.name
}

// Handler interface: Description().
func (h *handler) Description() string {
	return h.description
}

// Handler interface: Dump().
func (h *handler) Dump(msg interface{}) {
	if h != nil && h.fn != nil {
		h.fn(msg)
	}
}

// Supress dumping anything about a method.
var dumpOff HandlerFunc

func msgMetadata(msg interface{}) (string, string, string) {
	switch msg.(type) {
	case *handlerRequest:
		msg := msg.(*handlerRequest)
		return msg.kind, msg.method, time.Now().Format(stampLayout)
	case *handlerReply:
		msg := msg.(*handlerReply)
		latency := fmt.Sprintf("+%f", msg.latency.Seconds())
		return msg.kind, msg.method, fmt.Sprintf("%*s", latencyLen, latency)
	}

	return "", "", ""
}

func dumpToFile(format string, args ...interface{}) {
	if dumpFile == nil {
		return
	}
	fmt.Fprintf(dumpFile, format+"\n", args...)
}

// Dump method name and success/failure status.
func dumpName(msg interface{}) {
	kind, method, stamp := msgMetadata(msg)

	switch msg.(type) {
	case *handlerRequest:
	case *handlerReply:
		switch msg.(*handlerReply).msg.(type) {
		case error:
			log.Info("(%s) FAILED %s", kind, method)
			dumpToFile("[%s] (%s) FAILED %s", stamp, kind, method)
		default:
			log.Info("(%s) REQUEST %s", kind, method)
			dumpToFile("[%s] (%s) REQUEST %s", stamp, kind, method)
		}
	}
}

// Dump method name with full request and reply/error contents.
func dumpFull(msg interface{}) {
	kind, method, stamp := msgMetadata(msg)

	switch msg.(type) {
	case *handlerRequest:
		msg = msg.(*handlerRequest).msg
		line, _ := yaml.Marshal(msg)
		lines := strings.Split(strings.Trim(string(line), "\n"), "\n")
		if len(lines) > 1 {
			log.Info("(%s) REQUEST %s", kind, method)
			dumpToFile("[%s] (%s) REQUEST %s", stamp, kind, method)
			for _, l := range lines {
				log.Info("    %s => %s", method, l)
				dumpToFile("[%s]    %s => %s", stamp, method, l)
			}
		} else {
			log.Info("(%s) REQUEST %s => %s", kind, method, lines[0])
			dumpToFile("[%s] (%s) REQUEST %s => %s", stamp, kind, method, lines[0])
		}

	case *handlerReply:
		msg = msg.(*handlerReply).msg
		switch msg.(type) {
		case error:
			log.Warn("(%s) FAILED %s", kind, method)
			log.Warn("  %s <= %s", method, msg.(error))
			dumpToFile("[%s] (%s) FAILED %s", stamp, kind, method)
			dumpToFile("[%s]  %s <= %s", stamp, method, msg.(error))
		default:
			line, _ := yaml.Marshal(msg)
			lines := strings.Split(strings.Trim(string(line), "\n"), "\n")
			if len(lines) > 1 {
				log.Info("(%s) REPLY %s", kind, method)
				dumpToFile("[%s] (%s) REPLY %s (%d bytes as yaml)", stamp, kind, method, len(line))
				for _, l := range lines {
					log.Info("    %s <= %s", method, l)
					dumpToFile("[%s]    %s <= %s", stamp, method, l)
				}
			} else {
				log.Info("(%s) REPLY %s <= %s", kind, method, lines[0])
				dumpToFile("[%s] (%s) REPLY %s <= %s (%d bytes as yaml)", stamp, kind, method,
					lines[0], len(line))
			}
		}
	}
}

//
// Collect and periodically dump method call counts, averaged over a period.
//
// XXX TODO:
//   This is really primitive now. It works okayish if there is a relatively
//   constant flow of calls to counted methods, since we trigger reporting
//   only from a call to the counted methods itself. Otherwise is sucks.
//   We should change it to be smrter and timer-based.
//

type counter struct {
	calls  map[string]*call // per method counts
	Period time.Duration    // collection period
}

// Counter for a single call.
type call struct {
	start  uint64        // count at begining of period
	count  uint64        // current count
	began  time.Time     // beginning of period
	period time.Duration // measurement period
}

// Create counter for the given method call.
func newCall(now time.Time, period time.Duration) *call {
	return &call{start: 0, count: 1, began: now, period: period}
}

// Get current counter value.
func (c *call) Value() uint64 {
	return c.count
}

// Get counter value for current period.
func (c *call) Count() uint64 {
	return c.count - c.start
}

// Increase counter.
func (c *call) Increase(now time.Time) bool {
	c.count++
	return c.Age(now) > c.period
}

// Get the length of the current measurement period.
func (c *call) Age(now time.Time) time.Duration {
	return now.Sub(c.began)
}

// Calculate average for the current period.
func (c *call) Average(now time.Time) float64 {
	sec := float64(c.period / time.Second)
	avg := float64(c.Count()) / sec

	c.start, c.began = c.count, now

	return avg
}

// Count/dump call count to a method.
func (c *counter) dumpCount(msg interface{}) {
	kind, method, _ := msgMetadata(msg)

	switch msg.(type) {
	case *handlerRequest:
		now := time.Now()
		if call, ok := c.calls[method]; ok {
			if call.Increase(now) {
				log.Info("(%s) REQUEST: %s #%d, %d total, %.2f/s calls",
					kind, method, call.Value(), call.Count(), call.Average(now))
			}
		} else {
			c.calls[method] = newCall(now, c.Period)
		}
	default:
	}
}
