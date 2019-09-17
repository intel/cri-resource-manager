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
	"os"
	"strconv"
	"strings"
)

// TraceConfig represents a pre-defined instrumentation configuration.
type TraceConfig string

const (
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
)

type traceValue float64

// options encapsulates our configurable instrumentation parameters.
type options struct {
	trace     traceValue // instrumentation configuration
	collector string     // collector endpoint
	agent     string     // agent endpoint
	metrics   string     // metrics exporter address
}

// Our instrumentation options.
var opt = options{}

// symbolicTraceNames returns the valid predefined trace configuration names.
func symbolicTraceNames() string {
	names := []string{traceTesting, traceProduction, traceDisabled, traceAlways}
	return strings.Join(names, ", ")
}

func (t *traceValue) Set(value string) error {
	switch value {
	case traceTesting:
		*t = sampleTesting
	case traceProduction:
		*t = sampleProduction
	case traceDisabled:
		*t = sampleDisabled
	case traceAlways:
		*t = sampleAlways
	case traceNever:
		*t = sampleNever
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
		*t = traceValue(mul * v)
	}

	return nil
}

func (t *traceValue) String() string { return strconv.FormatFloat(float64(*t), 'f', -1, 64) }

func stringEnvVal(p *string, name, env, value, usage string) (*string, string, string, string) {
	usage += " Default is inherited from the environment variable '" + env + "', if set."

	if e := os.Getenv(env); e != "" {
		value = e
	}

	return p, name, value, usage
}

// Register our command-line flags.
func init() {
	// Default for opt.trace
	opt.trace = sampleDisabled

	flag.Var(&opt.trace, "trace",
		"Tracing configuration to use. The possible values are: "+symbolicTraceNames()+", "+
			"or a trace sampling probability.")
	flag.StringVar(stringEnvVal(&opt.collector, "trace-jaeger-collector", "JAEGER_COLLECTOR",
		"http://localhost:14268/api/traces",
		"Jaeger collector endpoint URL to use."))
	flag.StringVar(stringEnvVal(&opt.agent, "trace-jaeger-agent", "JAEGER_AGENT",
		"localhost:6831",
		"Jaeger agent address to use."))
	flag.StringVar(stringEnvVal(&opt.metrics, "trace-prometheus-metrics", "PROMETHEUS_ENDPOINT",
		":8888",
		"Address to serve Prometheus /metrics on."))
}
