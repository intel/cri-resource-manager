// Copyright 2022 Intel Corporation. All Rights Reserved.
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
	"os/exec"
	"time"
)

type RoutineStatActionsConfig struct {
	// IntervalMs is the length of the period in milliseconds in
	// which StatActions routine checks process_statactions call
	// statistics.
	IntervalMs int
	// IntervalCommand is the command to be executed in specified
	// intervals.
	IntervalCommand []string
	// PageOutMB is the total size of memory in megabytes that
	// has been process_statactions. If adviced memory exceeds the
	// interval, the shell command will be executed.
	PageOutMB int
	// PageOutCommand is executed when new PageOutMB is reached.
	PageOutCommand []string
}

type RoutineStatActions struct {
	config           *RoutineStatActionsConfig
	lastPageOutPages uint64
	cgLoop           chan interface{}
}

func init() {
	RoutineRegister("statactions", NewRoutineStatActions)
}

func NewRoutineStatActions() (Routine, error) {
	return &RoutineStatActions{}, nil
}

func (r *RoutineStatActions) SetConfigJson(configJson string) error {
	config := &RoutineStatActionsConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	if config.IntervalMs <= 0 {
		return fmt.Errorf("invalid stataction routine configuration, IntervalMs must be > 0")
	}
	if len(config.IntervalCommand) == 0 &&
		len(config.PageOutCommand) == 0 {
		return fmt.Errorf("invalid stataction routine configuration, no actions specified (command missing)")
	}
	r.config = config
	return nil
}

func (r *RoutineStatActions) GetConfigJson() string {
	if r.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(r.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (r *RoutineStatActions) Start() error {
	if r.config == nil {
		return fmt.Errorf("cannot start without configuration")
	}
	if r.cgLoop != nil {
		return fmt.Errorf("already started")
	}
	r.cgLoop = make(chan interface{})
	go r.loop()
	return nil
}

func (r *RoutineStatActions) Stop() {
	if r.cgLoop != nil {
		log.Debugf("Stopping statactions routine")
		r.cgLoop <- struct{}{}
	} else {
		log.Debugf("statactions routine already stopped")
	}
}

func (r *RoutineStatActions) Dump(args []string) string {
	return fmt.Sprintf("routine \"statactions\": running=%v", r.cgLoop != nil)
}

func runCommand(command []string) error {
	if len(command) == 0 {
		return nil
	}
	cmd := exec.Command(command[0], command[1:]...)
	err := cmd.Run()
	if err != nil {
		stats.Store(StatsHeartbeat{fmt.Sprintf("RoutineStatActions.command error: %s", err)})

	}
	return err
}

func (r *RoutineStatActions) loop() {
	log.Debugf("RoutineStatActions: online\n")
	defer log.Debugf("RoutineStatActions: offline\n")
	ticker := time.NewTicker(time.Duration(r.config.IntervalMs) * time.Millisecond)
	defer ticker.Stop()

	// Get initial values of stats that trigger actions
	r.lastPageOutPages = stats.MadvicedPageCount(-1, -1)
	quit := false
	for {
		// Wait for the next tick or Stop()
		select {
		case <-r.cgLoop:
			quit = true
			break
		case <-ticker.C:
			stats.Store(StatsHeartbeat{"RoutineStatActions.tick"})
		}
		if quit {
			break
		}
		nowPageOutPages := stats.MadvicedPageCount(-1, -1)

		// IntervalCommand runs on every round
		if len(r.config.IntervalCommand) > 0 {
			runCommand(r.config.IntervalCommand)
			stats.Store(StatsHeartbeat{"RoutineStatActions.IntervalCommand"})
		}

		// PageOutCommand runs if PageOutMB is reached
		if len(r.config.PageOutCommand) > 0 &&
			(nowPageOutPages-r.lastPageOutPages)*constUPagesize/uint64(1024*1024) >= uint64(r.config.PageOutMB) {
			r.lastPageOutPages = nowPageOutPages
			stats.Store(StatsHeartbeat{"RoutineStatActions.PageOutCommand"})
			runCommand(r.config.PageOutCommand)
		}
	}
	close(r.cgLoop)
	r.cgLoop = nil
}
