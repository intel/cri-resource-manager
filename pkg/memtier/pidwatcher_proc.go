// Copyright 2023 Intel Corporation. All Rights Reserved.
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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type PidWatcherProcConfig struct {
	IntervalMs int // poll interval
}

type PidWatcherProc struct {
	config       *PidWatcherProcConfig
	pidsReported map[int]setMemberType
	pidListener  PidListener
	stop         bool
}

func init() {
	PidWatcherRegister("proc", NewPidWatcherProc)
}

func NewPidWatcherProc() (PidWatcher, error) {
	w := &PidWatcherProc{
		pidsReported: map[int]setMemberType{},
	}
	// This pidwatcher is expected to work out-of-the-box without
	// any configuration. Set the defaults immediately.
	w.SetConfigJson("")
	return w, nil
}

func (w *PidWatcherProc) SetConfigJson(configJson string) error {
	config := &PidWatcherProcConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	if config.IntervalMs == 0 {
		config.IntervalMs = 5000
	}
	w.config = config
	return nil
}

func (w *PidWatcherProc) GetConfigJson() string {
	if w.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(w.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (w *PidWatcherProc) SetPidListener(l PidListener) {
	w.pidListener = l
}

func (w *PidWatcherProc) Poll() error {
	w.stop = false
	w.loop(true)
	return nil
}

func (w *PidWatcherProc) Start() error {
	w.stop = false
	go w.loop(false)
	return nil
}

func (w *PidWatcherProc) Stop() {
	w.stop = true
}

func (w *PidWatcherProc) Dump([]string) string {
	return fmt.Sprintf("%+v", w)
}

func (w *PidWatcherProc) loop(singleshot bool) {
	log.Debugf("PidWatcherProc: online\n")
	defer log.Debugf("PidWatcherProc: offline\n")
	ticker := time.NewTicker(time.Duration(w.config.IntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		stats.Store(StatsHeartbeat{"PidWatcherProc.loop"})
		// Read all pids under /proc
		pidsFound := map[int]setMemberType{}
		matches, err := filepath.Glob("/proc/*/exe")
		if err != nil {
			stats.Store(StatsHeartbeat{fmt.Sprintf("PidWatcherProc.error: glob failed: %s", err)})
		}
		for _, file := range matches {
			if _, err := os.Readlink(file); err != nil {
				// a kernel thread or the process is already gone
				continue
			}
			if pid, err := strconv.Atoi(strings.Split(file, "/")[2]); err == nil {
				pidsFound[pid] = setMember
			}
		}

		// Gather found pids that have not been reported.
		newPids := []int{}
		for foundPid := range pidsFound {
			if _, ok := w.pidsReported[foundPid]; !ok {
				w.pidsReported[foundPid] = setMember
				newPids = append(newPids, foundPid)
			}
		}

		// Gather reported pids that have disappeared.
		oldPids := []int{}
		for oldPid := range w.pidsReported {
			if _, ok := pidsFound[oldPid]; !ok {
				delete(w.pidsReported, oldPid)
				oldPids = append(oldPids, oldPid)
			}
		}

		// If requested to stop, quit without informing listeners.
		if w.stop {
			break
		}

		// Report if there are any changes in pids.
		if len(newPids) > 0 {
			if w.pidListener != nil {
				w.pidListener.AddPids(newPids)
			} else {
				log.Warnf("pidwatcher proc: ignoring new pids %v because nobody is listening", newPids)
			}
		}
		if len(oldPids) > 0 {
			if w.pidListener != nil {
				w.pidListener.RemovePids(oldPids)
			} else {
				log.Warnf("pidwatcher proc: ignoring disappeared pids %v because nobody is listening", oldPids)
			}
		}

		// If only one execution was requested, quit without waiting.
		if singleshot {
			break
		}

		// Wait for next tick.
		select {
		case <-ticker.C:
			continue
		}
	}
}
