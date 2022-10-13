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

package topologyaware

import (
	"time"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/events"
)

// trigger cold start for the container if necessary.
func (p *policy) triggerColdStart(c cache.Container) error {
	log.Info("coldstart: triggering coldstart for %s...", c.PrettyName())
	g, ok := p.allocations.grants[c.GetCacheID()]
	if !ok {
		log.Warn("coldstart: no grant found, nothing to do...")
		return nil
	}

	coldStart := g.ColdStart()
	if coldStart <= 0 {
		log.Info("coldstart: no coldstart, nothing to do...")
		return nil
	}

	// Start a timer to restore the grant memset to full. Store the
	// timer so that we can release it if the grant is destroyed before
	// the timer elapses.
	duration := coldStart
	timer := time.AfterFunc(duration, func() {
		p.stopLock.Lock()
		defer p.stopLock.Unlock()
		if p.stopped {
			return
		}
		e := &events.Policy{
			Type:   ColdStartDone,
			Source: PolicyName,
			Data:   c.GetID(),
		}
		if err := p.services.SendEvent(e); err != nil {
			// we should retry this later, the channel is probably full...
			log.Error("Ouch... we'should retry this later.")
		}
	})
	g.AddTimer(timer)
	return nil
}

// finish an ongoing coldstart for the container.
func (p *policy) finishColdStart(c cache.Container) (bool, error) {
	g, ok := p.allocations.grants[c.GetCacheID()]
	if !ok {
		log.Warn("coldstart: no grant found, nothing to do...")
		return false, policyError("coldstart: no grant found for %s", c.PrettyName())
	}

	log.Info("restoring memset to grant %v", g)
	g.RestoreMemset()
	g.ClearTimer()

	return true, nil
}
