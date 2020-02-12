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

package topology

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// to mock in tests
var (
	mockRoot = ""
)

const (
	// ProviderKubelet is a constant to distinguish that topology hint comes
	// from parameters passed to CRI create/update requests from Kubelet
	ProviderKubelet = "kubelet"
)

// Hint represents various hints that can be detected from sysfs for the device
type Hint struct {
	Provider string
	CPUs     string
	NUMAs    string
	Sockets  string
}

// Hints represents set of hints collected from multiple providers
type Hints map[string]Hint

func getDevicesFromVirtual(realDevPath string) (devs []string, err error) {
	if !filepath.HasPrefix(realDevPath, "/sys/devices/virtual") {
		return nil, fmt.Errorf("%s is not a virtual device", realDevPath)
	}

	relPath, _ := filepath.Rel("/sys/devices/virtual", realDevPath)

	dir, file := filepath.Split(relPath)
	switch dir {
	case "vfio/":
		iommuGroup := filepath.Join(mockRoot, "/sys/kernel/iommu_groups", file, "devices")
		files, err := ioutil.ReadDir(iommuGroup)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read IOMMU group %s", iommuGroup)
		}
		for _, file := range files {
			realDev, err := filepath.EvalSymlinks(filepath.Join(iommuGroup, file.Name()))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get real path for %s", file.Name())
			}
			devs = append(devs, realDev)
		}
		return devs, nil
	default:
		return nil, nil
	}
}

func getTopologyHint(sysFSPath string) (*Hint, error) {
	hint := Hint{Provider: sysFSPath}
	fileMap := map[string]*string{
		"local_cpulist": &hint.CPUs,
		"numa_node":     &hint.NUMAs,
	}
	if err := readFilesInDirectory(fileMap, sysFSPath); err != nil {
		return nil, err
	}
	// Workarounds for broken information provided by kernel
	if hint.NUMAs == "-1" {
		// non-NUMA aware device or system, ignore it
		hint.NUMAs = ""
	}
	if hint.NUMAs != "" && hint.CPUs == "" {
		// broken topology hint. BIOS reports socket id as NUMA node
		// First, try to get hints from parent device or bus.
		parentHints, er := NewTopologyHints(filepath.Dir(sysFSPath))
		if er == nil {
			cpulist := map[string]bool{}
			numalist := map[string]bool{}
			for _, h := range parentHints {
				if h.CPUs != "" {
					cpulist[h.CPUs] = true
				}
				if h.NUMAs != "" {
					numalist[h.NUMAs] = true
				}
			}
			if cpus := strings.Join(mapKeys(cpulist), ","); cpus != "" {
				hint.CPUs = cpus
			}
			if numas := strings.Join(mapKeys(numalist), ","); numas != "" {
				hint.NUMAs = numas
			}
		}
		// if after parent hints we still don't have CPUs hints, use numa hint as sockets.
		if hint.CPUs == "" && hint.NUMAs != "" {
			hint.Sockets = hint.NUMAs
			hint.NUMAs = ""
		}
	}
	return &hint, nil
}

// NewTopologyHints return array of hints for the device and its slaves (e.g. RAID).
func NewTopologyHints(devPath string) (hints Hints, err error) {
	hints = make(Hints)
	realDevPath, err := filepath.EvalSymlinks(devPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed get realpath for %s", devPath)
	}
	for p := realDevPath; strings.HasPrefix(p, mockRoot+"/sys/devices/"); p = filepath.Dir(p) {
		hint, err := getTopologyHint(p)
		if err != nil {
			return nil, err
		}
		if hint.CPUs != "" || hint.NUMAs != "" || hint.Sockets != "" {
			hints[hint.Provider] = *hint
			break
		}
	}
	fromVirtual, _ := getDevicesFromVirtual(realDevPath)
	slaves, _ := filepath.Glob(filepath.Join(realDevPath, "slaves/*"))
	for _, device := range append(slaves, fromVirtual...) {
		deviceHints, er := NewTopologyHints(device)
		if er != nil {
			return nil, er
		}
		hints = MergeTopologyHints(hints, deviceHints)
	}
	return
}

// MergeTopologyHints combines org and hints.
func MergeTopologyHints(org, hints Hints) (res Hints) {
	if org != nil {
		res = org
	} else {
		res = make(Hints)
	}
	for k, v := range hints {
		if _, ok := res[k]; ok {
			continue
		}
		res[k] = v
	}
	return
}

// String returns the hints as a string.
func (h *Hint) String() string {
	cpus, nodes, sockets, sep := "", "", "", ""

	if h.CPUs != "" {
		cpus = "CPUs:" + h.CPUs
		sep = ", "
	}
	if h.NUMAs != "" {
		nodes = sep + "NUMAs:" + h.NUMAs
		sep = ", "
	}
	if h.Sockets != "" {
		sockets = sep + "sockets:" + h.Sockets
	}

	return "<hints " + cpus + nodes + sockets + " (from " + h.Provider + ")>"
}

// FindSysFsDevice for given argument returns physical device where it is linked to.
// For device nodes it will return path for device itself. For regular files or directories
// this function returns physical device where this inode resides (storage device).
// If result device is a virtual one (e.g. tmpfs), error will be returned.
// For non-existing path, no error returned and path is empty.
func FindSysFsDevice(dev string) (string, error) {
	fi, err := os.Stat(dev)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrapf(err, "unable to get stat for %s", dev)
	}

	devType := "block"
	rdev := fi.Sys().(*syscall.Stat_t).Dev
	if mode := fi.Mode(); mode&os.ModeDevice != 0 {
		rdev = fi.Sys().(*syscall.Stat_t).Rdev
		if mode&os.ModeCharDevice != 0 {
			devType = "char"
		}
	}

	major := unix.Major(rdev)
	minor := unix.Minor(rdev)
	if major == 0 {
		return "", errors.Errorf("%s is a virtual device node", dev)
	}
	devPath := fmt.Sprintf("/sys/dev/%s/%d:%d", devType, major, minor)
	realDevPath, err := filepath.EvalSymlinks(devPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed get realpath for %s", devPath)
	}
	return realDevPath, nil
}

// readFilesInDirectory small helper to fill struct with content from sysfs entry
func readFilesInDirectory(fileMap map[string]*string, dir string) error {
	for k, v := range fileMap {
		b, err := ioutil.ReadFile(filepath.Join(dir, k))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return errors.Wrapf(err, "%s: unable to read file %q", dir, k)
		}
		*v = strings.TrimSpace(string(b))
	}
	return nil
}

// mapKeys is a small helper that returns slice of keys for a given map
func mapKeys(m map[string]bool) []string {
	ret := make([]string, len(m))
	i := 0
	for k := range m {
		ret[i] = k
		i++
	}
	return ret
}
