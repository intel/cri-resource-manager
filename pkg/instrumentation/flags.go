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

package instrumentation

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// TraceConfig represents a pre-defined instrumentation configuration.
type TraceConfig string

const (
	// Flag for selecting an instrumentation configuration.
	optTrace = "trace"
	// Flag for specifying Jaeger collector endpoint.
	optCollector = "trace-jaeger-collector"
	// Flag for specifying Jaeger agent endpoint.
	optAgent = "trace-jaeger-agent"
	// Flag for specifying the prometheus address to use.
	optMetrics = "trace-prometheus-metrics"
	// Environment variable name for specifying Jaeger collector endpoint.
	envCollector = "JAEGER_COLLECTOR"
	// Environment variable name for specifying Jaeger agent endpoint.
	envAgent = "JAEGER_AGENT"
	// Environment variable for specifying prometheus metrics address.
	envMetrics = "PROMETHEUS_ENDPOINT"
	// Symbolic trace names and their correponding sampling values.
	traceTesting     = "testing"
	traceProduction  = "production"
	traceDisabled    = "disabled"
	traceAlways      = "always"
	traceNever       = "never"
	sampleTesting    = 1.0
	sampleProduction = 0.1
	sampleDisabled   = 0.0
	sampleAlways     = 1.0
	sampleNever      = 0.0
	// Default option values.
	defaultTrace     = sampleDisabled
	defaultCollector = "http://localhost:14268/api/traces"
	defaultAgent     = "localhost:6831"
	defaultMetrics   = ":8888"
)

// options encapsulates our configurable instrumentation parameters.
type options struct {
	trace     float64 // instrumentation configuration
	collector string  // collector endpoint
	agent     string  // agent endpoint
	metrics   string  // metrics exporter address
}

// Our instrumentation options.
var opt = options{
	trace:     defaultTrace,
	collector: defaultCollector,
	agent:     defaultAgent,
	metrics:   defaultMetrics,
}

// symbolicTraceNames returns the valid predefined trace configuration names.
func symbolicTraceNames() string {
	names := []string{traceTesting, traceProduction, traceDisabled, traceAlways}
	return strings.Join(names, ", ")
}

func (o *options) parseTrace(value string) error {
	switch value {
	case traceTesting:
		o.trace = sampleTesting
	case traceProduction:
		o.trace = sampleProduction
	case traceDisabled:
		o.trace = sampleDisabled
	case traceAlways:
		o.trace = sampleAlways
	case traceNever:
		o.trace = sampleNever
	default:
		mul := 1.0
		val := value
		if strings.HasSuffix(value, "%") {
			mul = 0.01
			val = value[0 : len(value)-1]
		}
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return err
		}
		o.trace = mul * v
	}

	return nil
}

func (o *options) Set(name, value string) error {
	switch name {
	case optTrace:
		return o.parseTrace(value)
	case optCollector:
		o.collector = value
	case optAgent:
		o.agent = value
	case optMetrics:
		o.metrics = value
	default:
		return traceError("unknown relay option '%s' with value '%s'", name, value)
	}

	return nil
}

func (o *options) Get(name string) string {
	switch name {
	case optTrace:
		switch o.trace {
		case sampleTesting:
			return traceTesting
		case sampleProduction:
			return traceProduction
		case sampleDisabled:
			return traceDisabled
		default:
			return strconv.FormatFloat(o.trace, 'f', -1, 64)
		}
	case optCollector:
		return o.collector
	case optAgent:
		return o.agent
	case optMetrics:
		return o.metrics
	default:
		return fmt.Sprintf("<no value, unknown instrumnetation option '%s'>", name)
	}
}

type wrappedOption struct {
	name string
	opt  *options
}

func wrapOption(name, usage string) (*wrappedOption, string, string) {
	return &wrappedOption{name: name, opt: &opt}, name, usage
}

func wrapOptionEnv(name, env, usage string) (*wrappedOption, string, string) {
	wo := &wrappedOption{name: name, opt: &opt}
	usage += " Default is inherited from the environment variable '" + env + "', if set."

	if e := os.Getenv(env); e != "" {
		wo.Set(e)
	}

	return wo, name, usage
}

func (wo *wrappedOption) Name() string {
	return wo.name
}

func (wo *wrappedOption) Set(value string) error {
	return wo.opt.Set(wo.Name(), value)
}

func (wo *wrappedOption) String() string {
	return wo.opt.Get(wo.Name())
}

// Register our command-line flags.
func init() {
	flag.Var(wrapOption(optTrace,
		"Tracing configuration to use. The possible values are: "+symbolicTraceNames()+", "+
			"or a trace sampling probability."))
	flag.Var(wrapOptionEnv(optCollector, envCollector, "Jaeger collector endpoint URL to use."))
	flag.Var(wrapOptionEnv(optAgent, envAgent, "Jaeger agent address to use."))
	flag.Var(wrapOptionEnv(optMetrics, envMetrics, "Address to serve Prometheus /metrics on."))
}
