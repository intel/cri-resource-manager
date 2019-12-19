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

package resmgr

import (
	"time"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

// stopEvent is the event used to shut down event processing.
type stopEvent struct{}

// Our event logging instance
var elog = logger.NewLogger("event")

// SendEvent injects the given event to the resource managers event processing loop.
func (m *resmgr) SendEvent(event interface{}) error {
	if m.events == nil {
		return resmgrError("can't send event, no event channel")
	}
	select {
	case m.events <- event:
		return nil
	default:
		return resmgrError("can't send event, event channel full")
	}
}

// startEventProcessing starts the resource manager event processing goroutine.
func (m *resmgr) startEventProcessing() error {
	if err := m.setupMetricsCollection(); err != nil {
		return err
	}

	m.shutdown = make(chan interface{})
	m.events = make(chan interface{}, 8)

	go m.pollMetrics()
	go m.pollRebalance()
	go m.processEvents()

	return nil
}

// stopEventProcessing stops the resource manager event processing loop.
func (m *resmgr) stopEventProcessing() {
	if m.shutdown != nil {
		close(m.shutdown)
	}
}

// pollMetrics periodically polls our metrics gatherer.
func (m *resmgr) pollMetrics() {
	elog.Info("starting metrics polling")
	timer := time.NewTicker(opt.PollMetrics)
	for {
		select {
		case _ = <-m.shutdown:
			elog.Info("stopping metrics polling")
			timer.Stop()
			return
		case _ = <-timer.C:
			m.gatherMetrics()
			m.processMetrics()
		}
	}
}

// pollRebalance periodically checks and triggers rebalancing of containers if necessary.
func (m *resmgr) pollRebalance() {
	elog.Info("starting rebalance polling")
	timer := time.NewTicker(opt.Rebalance)
	for {
		select {
		case _ = <-m.shutdown:
			elog.Info("stopping rebalance polling")
			timer.Stop()
			return
		case _ = <-timer.C:
			if m.rebalance {
				if err := m.Rebalance(); err != nil {
					elog.Error("rebalancing failed: %v", err)
				} else {
					m.rebalance = false
				}
			}
		}
	}
}

// processEvents pulls events from our event channel and processes them.
func (m *resmgr) processEvents() {
	elog.Info("starting event processing")
	for {
		select {
		case _ = <-m.shutdown:
			elog.Info("stopping event processing")
			close(m.events)
			m.events = nil
			return
		case event := <-m.events:
			m.processOneEvent(event)
		}
	}
}

// processOneEvent processes a single resource manager event.
func (m *resmgr) processOneEvent(event interface{}) {
	m.Lock()
	defer m.Unlock()

	switch event.(type) {
	case string:
		elog.Debug("received string event '%s'...", event.(string))

	case *avx512Event:
		e := event.(*avx512Event)
		if e.active {
			if _, wasTagged := e.container.SetTag(cache.TagAVX512, "true"); !wasTagged {
				m.rebalance = true
			}
		} else {
			if _, wasTagged := e.container.DeleteTag(cache.TagAVX512); wasTagged {
				m.rebalance = true
			}
		}

	default:
		elog.Warn("received unknown event %T (%v)", event, event)
	}

	m.cache.Save()
}
