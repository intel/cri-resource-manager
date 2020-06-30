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

package resmgr

import (
	"time"

	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/metrics"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metricsring"

	"gonum.org/v1/gonum/stat"
)

const (
	DefaultMetricsBufferLen = 20
)

// Our logger instance for events.
var evtlog = logger.NewLogger("events")

// setupEventProcessing sets up event and metrics processing.
func (m *resmgr) setupEventProcessing() error {
	var err error

	m.events = make(chan interface{}, 8)
	m.stop = make(chan interface{})
	options := metrics.Options{
		PollInterval: opt.MetricsTimer,
		Events:       m.events,
	}
	if m.metrics, err = metrics.NewMetrics(options); err != nil {
		return resmgrError("failed to create metrics (pre)processor: %v", err)
	}

	return nil
}

// startEventProcessing starts event and metrics processing.
func (m *resmgr) startEventProcessing() error {
	if m.policy.Bypassed() {
		return nil
	}

	if err := m.metrics.Start(); err != nil {
		return resmgrError("failed to start metrics (pre)processor: %v", err)
	}

	stop := m.stop
	go func() {
		var rebalanceTimer *time.Ticker
		var rebalanceChan <-chan time.Time

		if opt.RebalanceTimer > 0 {
			rebalanceTimer = time.NewTicker(opt.RebalanceTimer)
			rebalanceChan = rebalanceTimer.C
		} else {
			m.Info("periodic rebalancing is disabled")
		}
		for {
			select {
			case _ = <-stop:
				if rebalanceTimer != nil {
					rebalanceTimer.Stop()
				}
				return
			case event := <-m.events:
				m.processEvent(event)
			case _ = <-rebalanceChan:
				if err := m.RebalanceContainers(); err != nil {
					evtlog.Error("rebalancing failed: %v", err)
				}
			}
		}
	}()

	return nil
}

// stopEventProcessing stops event and metrics processing.
func (m *resmgr) stopEventProcessing() {
	if m.stop != nil {
		close(m.stop)
		m.metrics.Stop()
		m.stop = nil
	}
}

// SendEvent injects the given event to the resource manager's event processing loop.
func (m *resmgr) SendEvent(event interface{}) error {
	if m.events == nil {
		return resmgrError("can't send event, no event channel")
	}
	select {
	case m.events <- event:
		return nil
	default:
		return resmgrError("can't send event of type %T, event channel full", event)
	}
}

// processEvent processes the given event.
func (m *resmgr) processEvent(e interface{}) {
	evtlog.Debug("received event of type %T...", e)

	switch event := e.(type) {
	case string:
		evtlog.Debug("'%s'...", event)
	case []*criapi.ContainerStats:
		m.processContainerStats(event)
		m.checkActions()
	case *events.Metrics:
		m.processAvx(event.Avx)
	case *events.Policy:
		m.DeliverPolicyEvent(event)
	default:
		evtlog.Warn("event of unexpected type %T...", e)
	}
}

// processAvx processes AVX512 events.
func (m *resmgr) processAvx(e *events.Avx) bool {
	if e == nil {
		return false
	}

	evtlog.Info("* got AVX Metrics: %T (nothing to process)", e)

	return true
}

func (m *resmgr) checkActions() bool {
	m.Lock()
	defer m.Unlock()

	metrics := m.cache.GetMetrics()

	for k, v := range metrics.Containers {

		c, exists := m.cache.LookupContainer(k)
		if !exists {
			continue
		}

		evtlog.Debug("checking container %s", c.PrettyName())

		mr, found := v["avx_switch_count_per_cgroup"]
		if !found {
			continue
		}

		data := mr.GetLastNSamples(mr.GetSize())
		if len(data) != mr.GetSize() {
			evtlog.Debug("need more data...waiting.")
			continue
		}

		ts, found := v["last_update_ns"]
		if !found {
			continue
		}

		lastSeen := ts.GetLastNSamples(1)[0] / 1e9
		evtlog.Debug("Container last seen using AVX %fs ago", lastSeen)

		// POC
		wss, found := v["memory_wss"]
		if found {
			evtlog.Debug("Container Memory WSS: %f", wss.EWMA())
		}

		// We can experiment with Gonum stats easily as the data can be fed
		// to stat.* methods directly. Try with stat.MeansStdDev first.
		if mean, stdDev := stat.MeanStdDev(data, nil); mean > 300 && stdDev < 100 && lastSeen < 10 {
			evtlog.Debug("Mean and stdDev of the samples are %.4f and %.4f, respectively", mean, stdDev)
			if _, wasTagged := c.SetTag(cache.TagAVX512, "true"); !wasTagged {
				evtlog.Info("container %s STARTED using AVX512 instructions", c.PrettyName())
			}

		} else {
			if _, wasTagged := c.DeleteTag(cache.TagAVX512); wasTagged {
				evtlog.Info("container %s STOPPED using AVX512 instructions", c.PrettyName())
			}
		}
	}

	return true
}

// processContainerStats processes collected CRI container statistics.
func (m *resmgr) processContainerStats(e []*criapi.ContainerStats) bool {
	if e == nil {
		return false
	}

	evtlog.Info("* processing CRI container statistics: %T", e)

	pcm, err := m.metrics.GetPrometheusContainerMetrics()
	if err != nil {
		evtlog.Error("failed to get prometheus container metrics: %v", err)
		return false
	}

	// copy to data struct from prometheus
	for _, c := range e {
		cid := c.GetAttributes().GetId()

		// Is it possible ContainerStats has containers that cgroupstats could not find?
		if _, ok := pcm[cid]; !ok {
			continue
		}
		pcm[cid]["core_ns"] = float64(c.GetCpu().GetUsageCoreNanoSeconds().GetValue())
		pcm[cid]["memory_wss"] = float64(c.GetMemory().GetWorkingSetBytes().GetValue())
	}

	m.Lock()
	defer m.Unlock()

	data := m.cache.GetMetrics()

	for cid, val := range pcm {
		var me cache.MetricsEntry
		if _, ok := data.Containers[cid]; !ok {
			me = make(cache.MetricsEntry)
		} else {
			me = data.Containers[cid]
		}
		for k, v := range val {
			// TODO(mythi): add filtering to pick only desired metrics
			if _, ok := me[k]; !ok {
				// TODO(mythi): add flag to set size configurable
				me[k] = metricsring.NewMetricsRing(DefaultMetricsBufferLen)
			}
			me[k].Push(v)
		}
		data.Containers[cid] = me
	}

	m.cache.SetMetrics(data)

	return false
}
