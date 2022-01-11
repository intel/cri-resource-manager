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
	Tracker TrackerConfig
	Heatmap HeatmapConfig
	Mover   MoverConfig
	// Cgroups is a list of cgroup paths in the filesystem. The
	// policy manages processes in listed cgroups and recursively
	// in their subgroups.
	Cgroups []string
	// IntervalMs is the length of the period in milliseconds
	// in which new heats are calculated for pages based on gathered
	// tracker values, and page move tasks are triggered.
	IntervalMs int
	// HeatNumas maps heat class values into NUMA node lists where
	// pages of each heat class should be located. If a heat class
	// is missing, the NUMA node is "don't care".
	HeatNumas map[int][]int
	// NumaSize sets the amount of memory that is usable on each
	// NUMA node. If a node is missing from the map, it's memory
	// use is not limited. The size is expressed in syntax:
	// <NUM>(k|M|G|%). If all the memory in a heat class exceeds
	// NumaSize of the NUMA nodes of that heat, the remaining
	// pages are moved to NUMA nodes of lower heats if there is
	// free capacity.
	NumaSize map[int]string
}

type PolicyHeat struct {
	config       *PolicyHeatConfig
	cgPidWatcher *PidWatcherCgroup
	chLoop       chan interface{} // for communication to the main loop of the policy
	tracker      Tracker
	heatmap      *Heatmap
	pageData     *AddrDatas
	mover        *Mover
	numaFree     map[int]int // free space for pages on each NUMA node
	numaSize     map[int]int // total capacity (in pages) on each NUMA node
}

type pageInfo struct {
	node int // NUMA node where a page is located
}

func init() {
	PolicyRegister("heat", NewPolicyHeat)
}

func NewPolicyHeat() (Policy, error) {
	var err error
	p := &PolicyHeat{
		heatmap:  NewCounterHeatmap(),
		pageInfo: NewAddrDatas(),
		mover:    NewMover(),
		numaFree: make(map[int]int),
		numaSize: make(map[int]int),
	}
	if p.cgPidWatcher, err = NewPidWatcherCgroup(); err != nil {
		return nil, fmt.Errorf("cgroup pid watcher error: %s", err)
	}
	return p, nil
}

func (p *PolicyHeat) SetConfigJson(configJson string) error {
	config := &PolicyHeatConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	return p.SetConfig(config)
}

func (p *PolicyHeat) SetConfig(config *PolicyHeatConfig) error {
	if config.IntervalMs <= 0 {
		return fmt.Errorf("invalid heat policy IntervalMs: %d, > 0 expected", config.IntervalMs)
	}
	if config.Tracker.Name == "" {
		return fmt.Errorf("tracker name missing from the heat policy configuration")
	}
	newTracker, err := NewTracker(config.Tracker.Name)
	if err != nil {
		return err
	}
	if config.Tracker.Config != "" {
		if err = newTracker.SetConfigJson(config.Tracker.Config); err != nil {
			return fmt.Errorf("configuring tracker %q for the heat policy failed: %s", config.Tracker.Name, err)
		}
	}
	newNumaFree := make(map[int]int)
	newNumaSize := make(map[int]int)
	for numa, sizeString := range config.NumaSize {
		sizeBytes, err := ParseBytes(sizeString)
		if err != nil {
			return fmt.Errorf("NumaSize[%d]: %s", numa, err)
		}
		newNumaSize[numa] = int(sizeBytes / constPagesize)
	}
	err = p.heatmap.SetConfig(&config.Heatmap)
	if err != nil {
		return fmt.Errorf("heatmap configuration error: %s", err)
	}
	if err = p.mover.SetConfig(&config.Mover); err != nil {
		return fmt.Errorf("configuring mover failed: %s", err)
	}
	p.switchToTracker(newTracker)
	p.numaFree = newNumaFree
	p.numaSize = newNumaSize
	p.config = config
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
		pconfig.Tracker.Config = p.tracker.GetConfigJson()
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

func (p *PolicyHeat) Dump(args []string) string {
	dumpHelp := "dump <heatmap|heatgram>"
	if len(args) == 0 {
		return dumpHelp
	}
	if args[0] == "heatmap" {
		lines := []string{}
		lines = append(lines, "heatmap:", p.heatmap.Dump())
		return strings.Join(lines, "\n")
	}
	if args[0] == "heatgram" {
		return "not implemented yet: histogram of heat values"
	}
	return dumpHelp
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
	ticker := time.NewTicker(time.Duration(p.config.IntervalMs) * time.Millisecond)
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
	if len(p.numaSize) == 0 {
		p.startMovesNoLimits(timestamp)
	} else {
		p.startMovesFillFastFree(timestamp)
	}
}

func (p *PolicyHeat) startMovesFillFastFree(timestamp int64) {
	for _, pid := range p.heatmap.Pids() {
		hrHotToCold := p.heatmap.Sorted(pid, func(hr0, hr1 *HeatRange) bool {
			if hr0.heat > hr1.heat ||
				(hr0.heat == hr1.heat && hr0.addr < hr1.addr) {
				return true
			}
			return false
		})
		for _, hr := range hrHotToCold {
			fmt.Printf("heat: %.6f\n", hr.heat)
		}

	}
}

func (p *PolicyHeat) startMovesNoLimits(timestamp int64) {
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
