// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package cgroups

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"strings"

	v1 "k8s.io/api/core/v1"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// Tasks is a cgroup's "tasks" entry.
	Tasks = "tasks"
	// Procs is cgroup's "cgroup.procs" entry.
	Procs = "cgroup.procs"
	// CpuShares is the cpu controller's "cpu.shares" entry.
	CpuShares = "cpu.shares"
	// CpuPeriod is the cpu controller's "cpu.cfs_period_us" entry.
	CpuPeriod = "cpu.cfs_period_us"
	// CpuQuota is the cpu controller's "cpu.cfs_quota_us" entry.
	CpuQuota = "cpu.cfs_quota_us"
	// CpusetCpus is the cpuset controller's cpuset.cpus entry.
	CpusetCpus = "cpuset.cpus"
	// CpusetMems is the cpuset controller's cpuset.mems entry.
	CpusetMems = "cpuset.mems"
)

var (
	// mount is the parent directory for per-controller cgroupfs mounts.
	mountDir = "/sys/fs/cgroup"
	// v2Dir is the parent directory for per-controller cgroupfs mounts.
	v2Dir = path.Join(mountDir, "unified")
	// KubeletRoot is the --cgroup-root option the kubelet is running with.
	KubeletRoot = ""
	// cpusetDir is the absolute path to the cpuset controller.
	cpusetDir = path.Join(mountDir, Cpuset.String())
	// cpuDir is the absolute path to the cpu controller.
	cpuDir = path.Join(mountDir, Cpu.String())
	// blkIODir is the absolute path to the blkio controller.
	blkioDir = path.Join(mountDir, Blkio.String())

	// our logger instance
	pathlog = logger.NewLogger("cgroups")
)

// GetMountDir returns the common mount point for cgroup v1 controllers.
func GetMountDir() string {
	return mountDir
}

// SetMountDir sets the common mount point fot the cgroup v1 controllers.
func SetMountDir(dir string) {
	cpuset, _ := filepath.Rel(mountDir, cpusetDir)
	cpu, _ := filepath.Rel(mountDir, cpuDir)
	blkio, _ := filepath.Rel(mountDir, blkioDir)
	v2, _ := filepath.Rel(mountDir, v2Dir)

	mountDir = dir

	if cpuset != "" {
		cpusetDir = path.Join(mountDir, cpuset)
	}
	if cpu != "" {
		cpuDir = path.Join(mountDir, cpu)
	}
	if blkio != "" {
		blkioDir = path.Join(mountDir, blkio)
	}
	if v2 != "" {
		v2Dir = path.Join(mountDir, v2)
	}
}

// GetV2Dir() returns the cgroup v2 unified mount directory.
func GetV2Dir() string {
	return v2Dir
}

// SetV2Dir sets the unified cgroup v2 mount directory.
func SetV2Dir(dir string) {
	if dir[0] == '/' {
		v2Dir = dir
	} else {
		v2Dir = path.Join(mountDir, v2Dir)
	}
}

// FindPodDir brute-force searches for a pod cgroup parent dir.
func FindPodDir(UID string, qos v1.PodQOSClass) (string, v1.PodQOSClass) {
	var classes []v1.PodQOSClass
	var cgpaths []string

	if qos != "" {
		classes = []v1.PodQOSClass{qos}
		cgpaths = []string{strings.ToLower(string(qos))}
	}
	classes = append(classes,
		v1.PodQOSGuaranteed,
		v1.PodQOSBestEffort,
		v1.PodQOSBurstable,
	)
	cgpaths = append(cgpaths,
		"guaranteed",
		"besteffort",
		"burstable",
	)

	for classIdx, class := range cgpaths {
		for fnIdx, fn := range podPathGenFns {
			for _, dir := range fn(UID, class) {
				if info, err := os.Stat(dir); err == nil {
					if info.Mode().IsDir() {
						// Prefer this function for future lookups.
						if fnIdx != 0 {
							podPathGenFns[fnIdx] = podPathGenFns[0]
							podPathGenFns[0] = fn
						}

						dir = strings.TrimPrefix(dir, cpusetDir)
						qos = classes[classIdx]

						return dir, qos
					}
				}
			}
		}
	}

	return "", qos
}

// FindPodQOSClass finds the pod QOS class corresponding to a cgroup path.
func FindPodQOSClass(cgroupParent, UID string) v1.PodQOSClass {
	if cgroupParent == "" {
		return ""
	}

	classes := []v1.PodQOSClass{
		v1.PodQOSGuaranteed,
		v1.PodQOSBestEffort,
		v1.PodQOSBurstable,
	}
	cgpaths := []string{
		"guaranteed",
		"besteffort",
		"burstable",
	}

	cgroupPath := path.Join(cpusetDir, cgroupParent)
	for classIdx, class := range cgpaths {
		for fnIdx, fn := range podPathGenFns {
			for _, dir := range fn(UID, class) {
				if dir == cgroupPath {
					// Prefer this function for future lookups.
					if fnIdx != 0 {
						podPathGenFns[fnIdx] = podPathGenFns[0]
						podPathGenFns[0] = fn
					}
					return classes[classIdx]
				}
			}
		}
	}

	return ""
}

// FindContainerDir brute-force searches for a container cgroup dir.
func FindContainerDir(podCgroupDir, podID, ID, runtimeClass string) string {
	var dirs []string

	if podCgroupDir == "" {
		return ""
	}

	if runtimeClass == "kata" {
		base := path.Base(podCgroupDir)
		dirs = []string{
			// kata v2
			path.Join(cpusetDir, "vc", "kata_"+base+":cri-containerd:"+podID),
			// kata v1
			path.Join(cpusetDir, "vc", "kata_"+podID),
		}
	} else {
		dirs = []string{
			path.Join(cpusetDir, podCgroupDir, "cri-containerd-"+ID+".scope"),
			path.Join(cpusetDir, podCgroupDir, ID),
			path.Join(cpusetDir, podCgroupDir, "crio-"+ID+".scope"),
		}
	}

	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil {
			if info.Mode().IsDir() {
				return strings.TrimPrefix(dir, cpusetDir)
			}
		}
	}

	return ""
}

// pod path generating functions
var podPathGenFns = []func(UID, class string) []string{
	// systemd driver
	func(UID, class string) []string {
		var kubeDir, qosDir, podDir string
		if KubeletRoot != "" {
			kubeDir = path.Join(KubeletRoot+".slice", "kubepods.slice")
		} else {
			kubeDir = "kubepods.slice"
		}
		UID = strings.ReplaceAll(UID, "-", "_")
		if class[0] == 'g' {
			qosDir = ""
			podDir = "kubepods-pod" + UID + ".slice"
		} else {
			qosDir = "kubepods-" + class + ".slice"
			podDir = "kubepods-" + class + "-pod" + UID + ".slice"
		}
		return []string{
			// with --cgroups-per-qos
			path.Join(cpusetDir, kubeDir, qosDir, podDir),
			// without --cgroups-per-qos
			path.Join(cpusetDir, kubeDir, podDir),
		}
	},
	// cgroups driver
	func(UID, class string) []string {
		var kubeDir, qosDir, podDir string
		kubeDir = path.Join(KubeletRoot, "kubepods")
		if class[0] == 'g' {
			qosDir = ""
		} else {
			qosDir = class
		}
		podDir = UID
		return []string{
			// with --cgroups-per-qos
			path.Join(cpusetDir, kubeDir, qosDir, podDir),
			// without --cgroups-per-qos
			path.Join(cpusetDir, kubeDir, podDir),
		}
	},
}

func init() {
	flag.StringVar(&mountDir, "cgroup-mount", mountDir,
		"directory under which cgroup v1 controllers are mounted")
	flag.StringVar(&v2Dir, "cgroup-v2-dir",
		v2Dir, "cgroup v2 unified mount directory")
	flag.StringVar(&KubeletRoot, "kubelet-cgroup-root", KubeletRoot,
		"--cgroup-root options the kubelet is running with")
}
