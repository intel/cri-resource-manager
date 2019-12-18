// Copyright 2019 Intel Corporation. All Rights Reserved.
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

package utils

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	intelRdtTasks   = "tasks"
	cpusetCgroupDir = "/sys/fs/cgroup/cpuset/"
)

// GetContainerCgroupDir finds container path in one specified subsystem directory
func GetContainerCgroupDir(subsystemDir, containerID string) string {
	var containerDir string

	filepath.Walk(subsystemDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		// Assume directory name contains containerID is what we want
		if strings.Contains(filepath.Base(path), containerID) {
			containerDir = path
		}
		return nil
	})

	return containerDir
}

// GetProcessInContainer gets the IDs of all processes in the container.
func GetProcessInContainer(cgroupParentDir, containerID string) ([]int, error) {
	var pids []int

	// Find Cpuset sub-cgroup directory of this container
	containerDir := ""
	// Probe known per-container directories, in order of decreasing probability
	if cgroupParentDir != "" {
		dirs := []string{
			filepath.Join(cpusetCgroupDir, cgroupParentDir, "docker-"+containerID+".scope"),
			filepath.Join(cpusetCgroupDir, cgroupParentDir, containerID),
		}
		for _, d := range dirs {
			info, err := os.Stat(d)
			if err == nil && info.IsDir() {
				containerDir = d
				break
			}
		}
	}

	// Try generic way to search container directory under one cgroups subsytem directory
	if containerDir == "" {
		containerDir = GetContainerCgroupDir(cpusetCgroupDir, containerID)
		if containerDir == "" {
			return pids, fmt.Errorf("failed to find corresponding cgroups directory for container %s", containerID)
		}
	}

	// Find all processes listed in cgroup tasks file and apply to RDT CLOS
	cgroupTasksFileName := path.Join(containerDir, intelRdtTasks)

	file, err := os.Open(cgroupTasksFileName)
	if err != nil {
		return pids, fmt.Errorf("failed to open file %s", cgroupTasksFileName)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		pid, err := strconv.Atoi(scanner.Text())
		if err != nil {
			return pids, err
		}
		pids = append(pids, pid)
	}
	return pids, nil
}
