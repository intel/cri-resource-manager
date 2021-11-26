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
	"strings"
	"time"
)

type PolicyHeatConfig struct {
	Tracker       string
	TrackerConfig string
	HeatmapConfig string
	MoverConfig   string
	Cgroups       []string // list of paths
	Interval      int

	// HeatNumas maps heat class values into NUMA node lists where
	// pages of each heat class should be located. If a heat class
	// is missing, the NUMA node is "don't care".
	HeatNumas map[int][]int
}

const policyHeatDefaults string = `{
        "Tracker":"damon",
        "TrackerConfig":"{\"Connection\":\"perf\",\"SamplingUs\":1000,\"AggregationUs\":100000,\"RegionsUpdateUs\":5000000,\"MinTargetRegions\":1000,\"MaxTargetRegions\":100000}",
        "HeatmapConfig":"{\"HeatMax\":1.0,\"HeatRetention\":0.9513,\"HeatClasses\":10}",
        "MoverConfig":"{\"Interval\":10,\"Bandwidth\":100}",
        "Interval":30
}`

type PolicyHeat struct {
	config       *PolicyHeatConfig
	cgPidWatcher *PidWatcherCgroup
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
	if p.cgPidWatcher, err = NewPidWatcherCgroup(); err != nil {
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

func (p *PolicyHeat) Dump() string {
	lines := []string{}
	lines = append(lines, "heatmap:", p.heatmap.Dump())
	return strings.Join(lines, "\n")
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
	if p.mover != nil {
		p.mover.Stop()
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
	if err := p.tracker.Start(); err != nil {
		return fmt.Errorf("tracker start error: %w", err)
	}
	p.chLoop = make(chan interface{})
	p.cgPidWatcher.SetSources(p.config.Cgroups)
	if len(p.config.Cgroups) > 0 {
		p.cgPidWatcher.Start(p.tracker)
	}
	if err := p.mover.Start(); err != nil {
		return fmt.Errorf("mover start error: %w", err)
	}
	go p.loop()
	return nil
}

func (p *PolicyHeat) loop() {
	log.Debugf("PolicyHeat: online\n")
	defer log.Debugf("PolicyHeat: offline\n")
	ticker := time.NewTicker(time.Duration(p.config.Interval) * time.Second)
	defer ticker.Stop()
	quit := false
	n := uint64(0)
	for !quit {
		timestamp := time.Now().UnixNano()
		newCounters := p.tracker.GetCounters()
		p.tracker.ResetCounters()
		log.Debugf("PolicyHeat: updating heatmap with %d address ranges\n", len(*newCounters))
		p.heatmap.UpdateFromCounters(newCounters, timestamp)
		if p.mover.TaskCount() == 0 {
			p.startMoves(timestamp)
		}
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

func (p *PolicyHeat) startMoves(timestamp int64) {
	moverTasks := 0
	for _, pid := range p.heatmap.Pids() {
		p.heatmap.ForEachRange(pid, func(hr *HeatRange) int {
			// TODO: config: is the information fresh enough for a decision?
			if timestamp-hr.updated > 10*int64(time.Second) {
				return 0
			}
			// TODO: config: has the range stable (old) enough?
			if timestamp-hr.created < 20*int64(time.Second) {
				return 0
			}
			heatClass := p.heatmap.HeatClass(hr)
			numas, ok := p.config.HeatNumas[heatClass]
			if !ok || len(numas) == 0 {
				// No NUMAs for this heat class
				return 0
			}
			// TODO: calculate numas in mems_allowed
			destNode := Node(numas[0])
			// TODO: check current NUMA nodes of the
			// range, do not move if already there.
			ar := NewAddrRanges(pid, hr.AddrRange())
			ppages, err := ar.PagesMatching(PMPresentSet | PMExclusiveSet)
			if err != nil {
				return -1
			}
			ppages = ppages.NotOnNode(destNode)
			if len(ppages.pages) == 0 {
				return 0
			}
			moverTasks += 1
			task := NewMoverTask(ppages, destNode)
			p.mover.AddTask(task)
			return 0
		})
	}
	if moverTasks > 0 {
		log.Debugf("created %d mover tasks\n", moverTasks)
	}
}
