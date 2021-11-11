// Copyright 2021 Intel Corporation. All Rights Reserved.
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

package memtier

import (
	"encoding/json"
	"fmt"
	"time"
)

type PolicyHeatConfig struct {
	Tracker       string
	TrackerConfig string
	HeatmapConfig string
	MoverConfig   string
	Cgroups       []string // list of paths
	Interval      int

	// HeatNuma maps heat values into NUMA node lists where pages
	// of each heat should be located. If a heat is missing, NUMA
	// node is "don't care".
	HeatNuma map[int][]int
}

const policyHeatDefaults string = `{"Tracker":"damon"}`

type PolicyHeat struct {
	config       *PolicyHeatConfig
	cgPidWatcher *CgroupPidWatcher
	chLoop       chan interface{} // for communication to the main loop of the policy
	tracker      Tracker
	heatmap      *Heatmap
	mover        *Mover
}

func init() {
	PolicyRegister("heat", NewPolicyHeat)
}

func NewPolicyHeat() (Policy, error) {
	var err error
	p := &PolicyHeat{
		heatmap: NewCounterHeatmap(),
		mover:   NewMover(),
	}
	if p.cgPidWatcher, err = NewCgroupPidWatcher(); err != nil {
		return nil, fmt.Errorf("cgroup pid watcher error: %s", err)
	}

	err = p.SetConfigJson(policyHeatDefaults)
	if err != nil {
		return nil, fmt.Errorf("default configuration error: %s", err)
	}
	return p, nil
}

func (p *PolicyHeat) SetConfigJson(configJson string) error {
	config := PolicyHeatConfig{}
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	if config.Interval <= 0 {
		return fmt.Errorf("invalid interval: %d, > 0 expected", config.Interval)
	}
	if config.Tracker == "" {
		return fmt.Errorf("tracker missing from the configuration")
	}
	newTracker, err := NewTracker(config.Tracker)
	if err != nil {
		return err
	}
	if config.TrackerConfig != "" {
		if err = newTracker.SetConfigJson(config.TrackerConfig); err != nil {
			return fmt.Errorf("configuring tracker %q failed: %s", config.Tracker, err)
		}
	}
	if config.HeatmapConfig != "" {
		if err = p.heatmap.SetConfigJson(config.HeatmapConfig); err != nil {
			return fmt.Errorf("configuring heatmap failed: %s", err)
		}
	}
	if config.MoverConfig != "" {
		if err = p.mover.SetConfigJson(config.MoverConfig); err != nil {
			return fmt.Errorf("configuring mover failed: %s", err)
		}
	}
	p.switchToTracker(newTracker)
	p.config = &config
	return nil
}

func (p *PolicyHeat) switchToTracker(newTracker Tracker) {
	if p.tracker != nil {
		p.tracker.Stop()
	}
	p.tracker = newTracker
}

func (p *PolicyHeat) GetConfigJson() string {
	if p.config == nil {
		return ""
	}
	pconfig := *p.config
	if p.tracker != nil {
		pconfig.TrackerConfig = p.tracker.GetConfigJson()
	}
	if p.mover != nil {
		pconfig.MoverConfig = p.mover.GetConfigJson()
	}
	if configStr, err := json.Marshal(&pconfig); err == nil {
		return string(configStr)
	}
	return ""
}

func (p *PolicyHeat) Mover() *Mover {
	return p.mover
}

func (p *PolicyHeat) Tracker() Tracker {
	return p.tracker
}

func (p *PolicyHeat) Stop() {
	if p.cgPidWatcher != nil {
		p.cgPidWatcher.Stop()
	}
	if p.tracker != nil {
		p.tracker.Stop()
	}
	if p.chLoop != nil {
		p.chLoop <- struct{}{}
	}
}

func (p *PolicyHeat) Start() error {
	if p.chLoop != nil {
		return fmt.Errorf("already started")
	}
	if p.config == nil {
		return fmt.Errorf("unconfigured policy")
	}
	if p.tracker == nil {
		return fmt.Errorf("missing tracker")
	}
	if len(p.config.Cgroups) == 0 {
		return fmt.Errorf("policy has nothing to watch")
	}
	p.tracker.Start()
	p.chLoop = make(chan interface{})
	p.cgPidWatcher.SetSources(p.config.Cgroups)
	if len(p.config.Cgroups) > 0 {
		p.cgPidWatcher.Start(p.tracker)
	}
	p.mover.Start()
	go p.loop()
	return nil
}

func (p *PolicyHeat) move(tcs *TrackerCounters, destNode Node) {
	if p.mover.TaskCount() == 0 {
		for _, tc := range *tcs {
			ppages, err := tc.AR.PagesMatching(PMPresentSet | PMExclusiveSet)
			if err != nil {
				continue
			}
			ppages = ppages.NotOnNode(destNode)
			if len(ppages.Pages()) > 100 {
				task := NewMoverTask(ppages, destNode)
				p.mover.AddTask(task)
			}
		}
	}
}

func (p *PolicyHeat) loop() {
	ticker := time.NewTicker(time.Duration(p.config.Interval) * time.Second)
	defer ticker.Stop()
	quit := false
	n := uint64(0)
	for !quit {
		timestamp := time.Now().UnixNano()
		p.heatmap.UpdateFromCounters(p.tracker.GetCounters(), timestamp)
		n += 1
		select {
		case <-p.chLoop:
			quit = true
			break
		case <-ticker.C:
			// TODO:
			// Go through which pages should be moved.
			continue
		}
	}
	close(p.chLoop)
	p.chLoop = nil
}
