// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package metrics

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	model "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metrics"
	// pull in all metrics collectors
	_ "github.com/intel/cri-resource-manager/pkg/metrics/register"
)

const (
	// DefaultAvxThreshold is the cutoff below which a cgroup/container is not an AVX user.
	DefaultAvxThreshold = float64(0.1)
)

// Event is a set of metrics events we deliver to be acted upon.
type Event struct {
	Avx *AvxEvent // AVX512 container usage changes
}

// Options describes options for metrics collection and processing.
type Options struct {
	// PollInterval is the interval for polling raw metrics.
	PollInterval time.Duration
	// Events is the channel for delivering metrics events.
	Events chan interface{}
	// AvxThreshold is the threshold (0 - 1) for a cgroup to be considered AVX512-active
	AvxThreshold float64
}

// Metrics implements collecting, caching and processing of raw metrics.
type Metrics struct {
	opts Options               // metrics collecting options
	g    prometheus.Gatherer   // prometheus/raw metrics gatherer
	stop chan interface{}      // channel to stop polling goroutine
	raw  []*model.MetricFamily // latest set of raw metrics
}

// Our logger instance.
var log = logger.NewLogger("metrics")

// NewMetrics creates a new instance for metrics collecting and processing.
func NewMetrics(opts Options) (*Metrics, error) {
	if opts.Events == nil {
		return nil, metricsError("invalid options, nil Event channel")
	}
	if opts.AvxThreshold == 0.0 {
		opts.AvxThreshold = DefaultAvxThreshold
	}

	g, err := metrics.NewMetricGatherer()
	if err != nil {
		return nil, metricsError("failed to create raw metrics gatherer: %v", err)
	}

	return &Metrics{
		opts: opts,
		raw:  make([]*model.MetricFamily, 0),
		g:    g,
	}, nil
}

// Start starts metrics collection and processing.
func (m *Metrics) Start() error {
	if m.stop != nil {
		return nil
	}

	stop := make(chan interface{})
	go func() {
		pollTimer := time.NewTicker(m.opts.PollInterval)
		for {
			select {
			case _ = <-stop:
				pollTimer.Stop()
				return
			case _ = <-pollTimer.C:
				if err := m.poll(); err != nil {
					log.Error("failed to poll raw metrics: %v", err)
					continue
				}

				if err := m.process(); err != nil {
					log.Error("failed to deliver metrics event: %v", err)
				}
			}
		}
	}()
	m.stop = stop

	return nil
}

// Stop stops metrics collection and processing.
func (m *Metrics) Stop() {
	close(m.stop)
	m.stop = nil
}

// poll does a single round of raw metrics collection.
func (m *Metrics) poll() error {
	f, err := m.g.Gather()
	if err != nil {
		return metricsError("failed to poll raw metrics: %v", err)
	}
	m.raw = f
	return nil
}

// process processes the collected raw metrics.
func (m *Metrics) process() error {
	raw := map[string]*model.MetricFamily{}
	for _, f := range m.raw {
		dump(" <metric "+*f.Name+"> ", f)
		raw[*f.Name] = f
	}

	event := &Event{
		Avx: m.collectAvxEvents(raw),
	}

	return m.sendEvent(event)
}

// sendEvent sends a metrics-based event for processing.
func (m *Metrics) sendEvent(e *Event) error {
	select {
	case m.opts.Events <- e:
		return nil
	default:
		return metricsError("failed to deliver event (channel full?)")
	}
}

// dump debug-dumps the given MetricFamily data
func dump(prefix string, f *model.MetricFamily) {
	if !log.DebugEnabled() {
		return
	}
	buf := &bytes.Buffer{}
	if _, err := expfmt.MetricFamilyToText(buf, f); err != nil {
		return
	}
	log.DebugBlock("  <"+prefix+"> ", "%s", strings.TrimSpace(buf.String()))
}

// metricsError returns a new formatted error specific to metrics-processing.
func metricsError(format string, args ...interface{}) error {
	return fmt.Errorf("metrics: "+format, args...)
}
