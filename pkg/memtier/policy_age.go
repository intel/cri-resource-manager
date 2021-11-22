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

type PolicyAgeConfig struct {
	Tracker        string
	TrackerConfig  string
	MoverConfig    string
	Cgroups        []string // list of paths
	Interval       int      // how often tracker counters are read
	IdleDuration   int      // if 0, skip moving idle pages
	IdleNUMA       int
	ActiveDuration int // if 0, skip moving active pages
	ActiveNUMA     int
}

const policyAgeDefaults string = `{"Tracker":"idlepage","Interval":5,"ActiveDuration":10,"IdleDuration":30}`

type PolicyAge struct {
	config       *PolicyAgeConfig
	cgPidWatcher *PidWatcherCgroup
	cgLoop       chan interface{}
	tracker      Tracker
	palt         *pidAddrLenTc // pid - address - length - memory trackercounter's age
	mover        *Mover
}

type tcAge struct {
	LastSeen    int64
	LastChanged int64
	LastRounds  uint64 // bitmap, i^th bit indicates if changed i rounds ago
	Tc          *TrackerCounter
}

type pidAddrLenTc map[int]map[uint64]map[uint64]*tcAge

func init() {
	PolicyRegister("age", NewPolicyAge)
}

func NewPolicyAge() (Policy, error) {
	var err error
	p := &PolicyAge{
		mover: NewMover(),
	}
	if p.cgPidWatcher, err = NewPidWatcherCgroup(); err != nil {
		return nil, fmt.Errorf("cgroup pid watcher error: %s", err)
	}

	err = p.SetConfigJson(policyAgeDefaults)
	if err != nil {
		return nil, fmt.Errorf("default configuration error: %s", err)
	}
	return p, nil
}

func (p *PolicyAge) SetConfigJson(configJson string) error {
	config := PolicyAgeConfig{}
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	if config.Interval <= 0 {
		return fmt.Errorf("invalid interval: %d, > 0 expected", config.Interval)
	}
	if config.ActiveDuration < 0 {
		return fmt.Errorf("invalid activeDuration: %d, >= 0 expected", config.ActiveDuration)
	}
	if config.IdleDuration < 0 {
		return fmt.Errorf("invalid idleDuration: %d, >= 0 expected", config.IdleDuration)
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
	if config.MoverConfig != "" {
		if err = p.mover.SetConfigJson(config.MoverConfig); err != nil {
			return fmt.Errorf("configuring mover failed: %s", err)
		}
	}
	p.switchToTracker(newTracker)
	p.config = &config
	return nil
}

func (p *PolicyAge) switchToTracker(newTracker Tracker) {
	if p.tracker != nil {
		p.tracker.Stop()
	}
	p.tracker = newTracker
}

func (p *PolicyAge) GetConfigJson() string {
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

func (p *PolicyAge) Mover() *Mover {
	return p.mover
}

func (p *PolicyAge) Tracker() Tracker {
	return p.tracker
}

func (p *PolicyAge) Dump() string {
	// TODO: describe policy state
	return ""
}

func (p *PolicyAge) Stop() {
	if p.cgPidWatcher != nil {
		p.cgPidWatcher.Stop()
	}
	if p.tracker != nil {
		p.tracker.Stop()
	}
	if p.cgLoop != nil {
		p.cgLoop <- struct{}{}
	}
}

func (p *PolicyAge) Start() error {
	if p.cgLoop != nil {
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
	p.cgLoop = make(chan interface{})
	p.cgPidWatcher.SetSources(p.config.Cgroups)
	if len(p.config.Cgroups) > 0 {
		p.cgPidWatcher.Start(p.tracker)
	}
	p.mover.Start()
	go p.loop()
	return nil
}

func (p *PolicyAge) updateCounter(tc *TrackerCounter, timestamp int64) {
	pid := tc.AR.Pid()
	addr := tc.AR.Ranges()[0].Addr()
	length := tc.AR.Ranges()[0].Length()
	alt, ok := (*(p.palt))[pid]
	if !ok {
		alt = map[uint64]map[uint64]*tcAge{}
		(*p.palt)[pid] = alt
	}
	lt, ok := alt[addr]
	if !ok {
		lt = map[uint64]*tcAge{}
		alt[addr] = lt
	}
	prevTc, ok := lt[length]
	if !ok {
		copyOfTc := *tc
		prevTc = &tcAge{
			LastSeen:    timestamp,
			LastChanged: timestamp,
			LastRounds:  1,
			Tc:          &copyOfTc,
		}
		lt[length] = prevTc
	} else {
		prevTc.LastSeen = timestamp
		prevTc.LastRounds = prevTc.LastRounds << 1
		if prevTc.Tc.Accesses != tc.Accesses ||
			prevTc.Tc.Reads != tc.Reads ||
			prevTc.Tc.Writes != tc.Writes {
			prevTc.LastChanged = timestamp
			prevTc.LastRounds |= 1
			prevTc.Tc.Accesses = tc.Accesses
			prevTc.Tc.Reads = tc.Reads
			prevTc.Tc.Writes = tc.Writes
		}
	}
}

func (p *PolicyAge) deleteDeadCounters(timestamp int64) {
	aliveThreshold := timestamp - int64(2*time.Duration(p.config.Interval)*time.Second)
	for pid, alt := range *p.palt {
		for addr, lt := range alt {
			for length, tcage := range lt {
				if tcage.LastSeen < aliveThreshold {
					delete(lt, length)
				}
			}
			if len(lt) == 0 {
				delete(alt, addr)
			}
		}
		if len(alt) == 0 {
			delete(*p.palt, pid)
		}
	}
}

func (p *PolicyAge) activeCounters() *TrackerCounters {
	tcs := &TrackerCounters{}
	activeRounds := int(p.config.ActiveDuration / p.config.Interval)
	activeRoundMask := uint64(0x1)
	for i := 0; i < activeRounds; i++ {
		activeRoundMask = (activeRoundMask << 1) | 1
	}
	for _, alt := range *p.palt {
		for _, lt := range alt {
			for _, tcage := range lt {
				if tcage.LastRounds&activeRoundMask == activeRoundMask {
					*tcs = append(*tcs, *tcage.Tc)
				}
			}
		}
	}
	return tcs
}

func (p *PolicyAge) idleCounters(timestamp int64) *TrackerCounters {
	tcs := &TrackerCounters{}
	idleThreshold := timestamp - int64(time.Duration(p.config.IdleDuration)*time.Second)
	for _, alt := range *p.palt {
		for _, lt := range alt {
			for _, tcage := range lt {
				if tcage.LastChanged < idleThreshold {
					*tcs = append(*tcs, *tcage.Tc)
				}
			}
		}
	}
	return tcs
}

func (p *PolicyAge) move(tcs *TrackerCounters, destNode Node) {
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

func (p *PolicyAge) loop() {
	log.Debugf("PolicyAge: online\n")
	defer log.Debugf("PolicyAge: offline\n")
	ticker := time.NewTicker(time.Duration(p.config.Interval) * time.Second)
	defer ticker.Stop()

	p.palt = &pidAddrLenTc{}

	quit := false
	n := uint64(0)
	for !quit {
		timestamp := time.Now().UnixNano()
		for _, tc := range *p.tracker.GetCounters() {
			p.updateCounter(&tc, timestamp)
		}
		p.deleteDeadCounters(timestamp)
		if p.config.IdleDuration > 0 {
			// Moving idle pages is enabled.
			itcs := p.idleCounters(timestamp).RegionsMerged()
			for _, tc := range *itcs {
				log.Debugf("%d sec idle: %s\n", p.config.IdleDuration, tc.AR.Ranges()[0])
			}
			// TODO: skip already moved regions
			p.move(itcs, Node(p.config.IdleNUMA))

		}
		if p.config.ActiveDuration > 0 {
			// Moving active pages is enabled.
			atcs := p.activeCounters().RegionsMerged()
			for _, tc := range *atcs {
				log.Debugf("%d sec active: %s\n", p.config.ActiveDuration, tc.AR.Ranges()[0])
			}
			// TODO: skip already moved regions
			p.move(atcs, Node(p.config.ActiveNUMA))
		}
		n += 1
		select {
		case <-p.cgLoop:
			quit = true
			break
		case <-ticker.C:
			continue
		}
	}
	close(p.cgLoop)
	p.cgLoop = nil
}
