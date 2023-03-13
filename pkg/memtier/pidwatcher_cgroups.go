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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type PidWatcherCgroupsConfig struct {
	IntervalMs int
	Cgroups    []string // list of absolute cgroup directory paths
}

type PidWatcherCgroups struct {
	config       *PidWatcherCgroupsConfig
	pidsReported map[int]setMemberType
	pidListener  PidListener
	stop         bool
}

func init() {
	PidWatcherRegister("cgroups", NewPidWatcherCgroups)
}

func NewPidWatcherCgroups() (PidWatcher, error) {
	w := &PidWatcherCgroups{
		pidsReported: map[int]setMemberType{},
	}
	return w, nil
}

func (w *PidWatcherCgroups) SetConfigJson(configJson string) error {
	config := &PidWatcherCgroupsConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	if config.IntervalMs == 0 {
		config.IntervalMs = 5000
	}
	w.config = config
	return nil
}

func (w *PidWatcherCgroups) GetConfigJson() string {
	if w.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(w.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (w *PidWatcherCgroups) SetPidListener(l PidListener) {
	w.pidListener = l
}

func (w *PidWatcherCgroups) Poll() error {
	w.stop = false
	w.loop(true)
	return nil
}

func (w *PidWatcherCgroups) Start() error {
	w.stop = false
	go w.loop(false)
	return nil
}

func (w *PidWatcherCgroups) Stop() {
	w.stop = true
}

func (w *PidWatcherCgroups) loop(singleshot bool) {
	log.Debugf("PidWatcherCgroups: online\n")
	defer log.Debugf("PidWatcherCgroups: offline\n")
	ticker := time.NewTicker(time.Duration(w.config.IntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		stats.Store(StatsHeartbeat{"PidWatcherCgroups.loop"})
		// Look for all pid files in the current cgroup hierarchy.
		procPaths := map[string]setMemberType{}
		for _, cgroupPath := range w.config.Cgroups {
			for _, path := range findFiles(cgroupPath, "cgroup.procs") {
				procPaths[path] = setMember
			}
		}

		// Read all pids in pid files.
		pidsFound := map[int]setMemberType{}
		for path := range procPaths {
			pidsNow, err := readPids(path)
			if err != nil {
				delete(procPaths, path)
			}
			for _, pid := range pidsNow {
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
				log.Warnf("pidwatcher cgroup: ignoring new pids %v because nobody is listening", newPids)
			}
		}
		if len(oldPids) > 0 {
			if w.pidListener != nil {
				w.pidListener.RemovePids(oldPids)
			} else {
				log.Warnf("pidwatcher cgroup: ignoring disappeared pids %v because nobody is listening", oldPids)
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

func readPids(path string) ([]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	pids := make([]int, 0, len(lines))
	for index, line := range lines {
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("bad pid at %s:%d (%q): %s",
				path, index+1, line, err)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func findFiles(root string, filename string) []string {
	matchingFiles := []string{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == filename {
			matchingFiles = append(matchingFiles, path)
		}
		return nil
	})
	return matchingFiles
}

func (w *PidWatcherCgroups) Dump([]string) string {
	return fmt.Sprintf("%+v", w)
}
