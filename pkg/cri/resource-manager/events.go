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

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/metrics"
	logger "github.com/intel/cri-resource-manager/pkg/log"
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
		rebalanceTimer := time.NewTicker(opt.RebalanceTimer)
		for {
			select {
			case _ = <-stop:
				return
			case event := <-m.events:
				m.processEvent(event)
			case _ = <-rebalanceTimer.C:
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

// SendEvent injects the given event to the resource manaager's event processing loop.
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

	m.Lock()
	defer m.Unlock()

	switch event := e.(type) {
	case string:
		evtlog.Debug("'%s'...", event)
	case *metrics.Event:
		m.processAvx(event.Avx)
	default:
		evtlog.Warn("event of unexpected type %T...", e)
	}
}

// processAvx processes AVX512 events.
func (m *resmgr) processAvx(e *metrics.AvxEvent) bool {
	if e == nil {
		return false
	}

	changes := false
	for cgroup, active := range e.Updates {
		c, ok := m.resolveCgroupPath(cgroup)
		if !ok {
			continue
		}
		// XXX This is just for testing, we should effectively drive state transitions
		//     through a low-pass filter.
		if active {
			if _, wasTagged := c.SetTag(cache.TagAVX512, "true"); !wasTagged {
				evtlog.Info("container %s STARTED using AVX512 instructions", c.PrettyName())
			}
		} else {
			if _, wasTagged := c.DeleteTag(cache.TagAVX512); wasTagged {
				evtlog.Info("container %s STOPPED using AVX512 instructions", c.PrettyName())
			}
		}
	}
	return changes
}

// resolveCgroupPath resolves a cgroup path to a container.
func (m *resmgr) resolveCgroupPath(path string) (cache.Container, bool) {
	return m.cache.LookupContainerByCgroup(path)
}
