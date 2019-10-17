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
	"github.com/intel/cri-resource-manager/pkg/config"
	"os"
	"strconv"
	"strings"
)

const (
	// Select runtime tracing configuration to use.
	optSampling = "sampling"
	// Specify Jaeger collector endpoint to use.
	optCollector = "jaeger-collector"
	// Specify the Jaeger agent to use.
	optAgent = "jaeger-agent"
	// Specify the HTTP endpoint to server Prometheus metrics on.
	optMetrics = "prometheus-metrics"

	// Symbolic trace configuration names and their correponding sampling values.
	configTesting    = "testing"
	configProduction = "production"
	configDisabled   = "disabled"
	configAlways     = "always"
	configNever      = "never"
	sampleTesting    = 1.0
	sampleProduction = 0.1
	sampleDisabled   = 0.0
	sampleAlways     = 1.0
	sampleNever      = 0.0
)

// Options captures our configurable instrumentation parameters.
type options struct {
	sampling  traceValue // instrumentation sampling configuration
	collector string     // collector endpoint
	agent     string     // agent endpoint
	metrics   string     // metrics exporter address
}

// Our configuration module and configurable options.
var cfg *config.Module
var opt = options{}

// traceValue is a samplingrepresents a pre-defined instrumentation configuration.
type traceValue float64

// symbolicTraceNames returns the valid predefined trace configuration names.
func symbolicTraceNames() string {
	names := []string{configTesting, configProduction, configDisabled, configAlways}
	return strings.Join(names, ", ")
}

func (t *traceValue) Set(value string) error {
	switch value {
	case configTesting:
		*t = sampleTesting
	case configProduction:
		*t = sampleProduction
	case configDisabled:
		*t = sampleDisabled
	case configAlways:
		*t = sampleAlways
	case configNever:
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
	cfg = config.Register("trace", "tracing and instrumentation")

	// Default for opt.trace
	opt.sampling = sampleDisabled

	cfg.Var(&opt.sampling, optSampling,
		"Tracing configuration to use. One of "+symbolicTraceNames()+" or a sampling probability")
	cfg.StringVar(&opt.collector, optCollector, "http://localhost:14268/api/traces",
		"Tracing Jaeger collector endpoint URL to use.")
	cfg.StringVar(&opt.agent, optAgent, "localhost:6831",
		"Tracing Jaeger agent address to use.")
	cfg.StringVar(&opt.metrics, optMetrics, ":8888",
		"Tracing HTTP server address to serve Prometheus /metrics on.")
}
