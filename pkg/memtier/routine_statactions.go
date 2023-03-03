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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
	// IntervalCommandRunner executes the IntervalCommand.
	// "exec" forks and executes the command in a child process.
	// "memtier" runs memtier command.
	// "memtier-prompt" runs a single-string memtier-prompt
	// command that is allowed to contain pipe to shell.
	IntervalCommandRunner string
	// PageOutMB is the total size of memory in megabytes that
	// has been process_statactions. If advised memory exceeds the
	// interval, the shell command will be executed.
	PageOutMB int
	// PageOutCommand is executed when new PageOutMB is reached.
	PageOutCommand []string
	// PageOutCommandRunner executes the PageOutCommand.
	// See IntervalCommandRunner for options.
	PageOutCommandRunner string
}

type RoutineStatActions struct {
	config           *RoutineStatActionsConfig
	lastPageOutPages uint64
	cgLoop           chan interface{}
}

type commandRunnerFunc func([]string) error

const (
	commandRunnerDefault       = ""
	commandRunnerExec          = "exec"
	commandRunnerMemtier       = "memtier"
	commandRunnerMemtierPrompt = "memtier-prompt"
)

var commandRunners map[string]commandRunnerFunc

func init() {
	commandRunners = map[string]commandRunnerFunc{
		commandRunnerExec:          runCommandExec,
		commandRunnerDefault:       runCommandExec,
		commandRunnerMemtier:       runCommandMemtier,
		commandRunnerMemtierPrompt: runCommandMemtierPrompt,
	}
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

func runCommandExec(command []string) error {
	if len(command) == 0 {
		return nil
	}
	cmd := exec.Command(command[0], command[1:]...)
	err := cmd.Run()
	stats.Store(StatsHeartbeat{fmt.Sprintf("RoutineStatActions.command.exec: %s... status: %s error: %s", command[0], cmd.ProcessState, err)})
	return err
}

func runCommandMemtier(command []string) error {
	if len(command) == 0 {
		return nil
	}
	prompt := NewPrompt("", nil, bufio.NewWriter(os.Stdout))
	status := prompt.RunCmdSlice(command)
	stats.Store(StatsHeartbeat{fmt.Sprintf("RoutineStatActions.command.memtier: %s... status: %d", command[0], status)})
	return nil
}

func runCommandMemtierPrompt(command []string) error {
	if len(command) == 0 {
		return nil
	}
	if len(command) > 1 {
		return fmt.Errorf("invalid command for command runner %q: expected a list with a single string, got %q", commandRunnerMemtierPrompt, command)
	}
	prompt := NewPrompt("", nil, bufio.NewWriter(os.Stdout))
	status := prompt.RunCmdString(command[0])
	stats.Store(StatsHeartbeat{fmt.Sprintf("RoutineStatActions.command.memtier-prompt: %q status: %d", command[0], status)})
	return nil
}

func runCommand(runner string, command []string) error {
	commandRunner, ok := commandRunners[runner]
	if !ok {
		return fmt.Errorf("invalid command runner '%s'", runner)
	}
	return commandRunner(command)
}

func (r *RoutineStatActions) loop() {
	log.Debugf("RoutineStatActions: online\n")
	defer log.Debugf("RoutineStatActions: offline\n")
	ticker := time.NewTicker(time.Duration(r.config.IntervalMs) * time.Millisecond)
	defer ticker.Stop()

	// Get initial values of stats that trigger actions
	r.lastPageOutPages = stats.MadvisedPageCount(-1, -1)
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
		nowPageOutPages := stats.MadvisedPageCount(-1, -1)

		// IntervalCommand runs on every round
		if len(r.config.IntervalCommand) > 0 {
			if err := runCommand(r.config.IntervalCommandRunner, r.config.IntervalCommand); err != nil {
				log.Errorf("routines statactions intervalcommand: %s", err)
			}
			stats.Store(StatsHeartbeat{"RoutineStatActions.IntervalCommand"})
		}

		// PageOutCommand runs if PageOutMB is reached
		if len(r.config.PageOutCommand) > 0 &&
			(nowPageOutPages-r.lastPageOutPages)*constUPagesize/uint64(1024*1024) >= uint64(r.config.PageOutMB) {
			r.lastPageOutPages = nowPageOutPages
			stats.Store(StatsHeartbeat{"RoutineStatActions.PageOutCommand"})
			if err := runCommand(r.config.PageOutCommandRunner, r.config.PageOutCommand); err != nil {
				log.Errorf("routines statactions pageoutcommand: %s", err)
			}
		}
	}
	close(r.cgLoop)
	r.cgLoop = nil
}
