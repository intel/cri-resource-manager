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
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type PidWatcherCgroup struct {
	cgroupPaths  map[string]setMemberType
	pidsReported map[int]setMemberType
	listener     PidListener
	stop         bool
}

func NewPidWatcherCgroup() (*PidWatcherCgroup, error) {
	w := &PidWatcherCgroup{
		cgroupPaths:  map[string]setMemberType{},
		pidsReported: map[int]setMemberType{},
	}
	return w, nil
}

func (w *PidWatcherCgroup) SetSources(sources []string) {
	w.cgroupPaths = map[string]setMemberType{}
	for _, cgroupPath := range sources {
		w.cgroupPaths[cgroupPath] = setMember
	}
}

func (w *PidWatcherCgroup) Poll(l PidListener) error {
	w.listener = l
	w.stop = false
	w.loop(true)
	return nil
}

func (w *PidWatcherCgroup) Start(l PidListener) error {
	w.listener = l
	w.stop = false
	go w.loop(false)
	log.Debugf("PidWatcherCgroup: online\n")
	return nil
}

func (w *PidWatcherCgroup) Stop() {
	w.stop = true
}

func (w *PidWatcherCgroup) loop(singleshot bool) {
	ticker := time.NewTicker(time.Duration(5) * time.Second)
	defer ticker.Stop()
	for {
		// Look for all pid files in the current cgroup hierarchy.
		procPaths := map[string]setMemberType{}
		for cgroupPath := range w.cgroupPaths {
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
			w.listener.AddPids(newPids)
		}
		if len(oldPids) > 0 {
			w.listener.RemovePids(oldPids)
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
	log.Debugf("PidWatcherCgroup: offline\n")
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
