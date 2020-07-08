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
	"strings"
)

const (
	cgroupTasks     = "tasks"
	cpusetCgroupDir = "/sys/fs/cgroup/cpuset/"
)

// GetContainerCgroupDir brute-force searches for a container directory under parentDir.
func GetContainerCgroupDir(parentDir, containerID string) string {
	var containerDir string

	filepath.Walk(parentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if containerDir != "" {
			return filepath.SkipDir
		}
		// Assume any directory that contains containerID is the one we look for.
		if strings.Contains(filepath.Base(path), containerID) {
			containerDir = path
		}
		return nil
	})
	return containerDir
}

// isProcess finds out whether the task is a process or a thread.
func isProcess(processID string) bool {
	file, err := os.Open("/proc/" + processID + "/status")
	if err != nil {
		return true
	}
	defer file.Close()

	tgid := ""
	pid := ""

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry := scanner.Text()
		if strings.HasPrefix(entry, "Tgid:") {
			tgid = strings.TrimSpace(entry[len("Tgid:"):])
			if len(pid) > 0 {
				return tgid == pid
			}
		} else if strings.HasPrefix(entry, "Pid:") {
			pid = strings.TrimSpace(entry[len("Pid:"):])
			if len(tgid) > 0 {
				return tgid == pid
			}
		}
	}
	return true
}

func getTasksInContainer(cgroupParentDir, containerID string, onlyProcesses bool) ([]string, error) {
	var entries []string

	// Find Cpuset sub-cgroup directory of this container
	containerDir := ""
	// Probe known per-container directories
	if cgroupParentDir != "" {
		dirs := []string{
			filepath.Join(cpusetCgroupDir, cgroupParentDir, "cri-containerd-"+containerID+".scope"),
			filepath.Join(cpusetCgroupDir, cgroupParentDir, "crio-"+containerID+".scope"),
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
			return nil, fmt.Errorf("failed to find corresponding cgroups directory for container %s", containerID)
		}
	}

	// Find all processes listed in cgroup tasks file and apply to RDT CLOS
	cgroupTasksFileName := path.Join(containerDir, cgroupTasks)

	file, err := os.Open(cgroupTasksFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s", cgroupTasksFileName)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry := scanner.Text()
		if !onlyProcesses || isProcess(entry) {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// GetProcessesInContainer gets the IDs of all processes in the container.
func GetProcessesInContainer(cgroupParentDir, containerID string) ([]string, error) {
	return getTasksInContainer(cgroupParentDir, containerID, true)
}

// GetTasksInContainer gets the IDs of all tasks in the container.
func GetTasksInContainer(cgroupParentDir, containerID string) ([]string, error) {
	return getTasksInContainer(cgroupParentDir, containerID, false)
}
