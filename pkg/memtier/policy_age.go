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
)

type PolicyAgeConfig struct {
	Tracker       string
	TrackerConfig string
	Cgroups       []string // list of paths
}

const policyAgeDefaults string = `{"Tracker":"idlepage"}`

type PolicyAge struct {
	config       *PolicyAgeConfig
	cgPidWatcher *CgroupPidWatcher
	cgLoop       chan interface{}
	tracker      Tracker
}

func init() {
	PolicyRegister("age", NewPolicyAge)
}

func NewPolicyAge() (Policy, error) {
	var err error
	p := &PolicyAge{}
	if p.cgPidWatcher, err = NewCgroupPidWatcher(); err != nil {
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
	return ""
}

func (p *PolicyAge) Stop() {
	if p.cgPidWatcher != nil {
		p.cgPidWatcher.Stop()
	}
	if p.tracker != nil {
		p.tracker.Stop()
	}
}

func (p *PolicyAge) Start() error {
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
	go p.loop()
	return nil
}

func (p *PolicyAge) loop() {
	quit := false
	for !quit {
		select {
		case <-p.cgLoop:
			quit = true
		}
	}
	p.Stop()
}
