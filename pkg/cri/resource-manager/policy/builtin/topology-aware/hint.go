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

package topologyaware

import (
	"fmt"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	"strconv"
	"strings"
)

// Calculate the hint score of the given hint and CPUSet.
func cpuHintScore(hint system.TopologyHint, CPUs cpuset.CPUSet) float64 {
	hCPUs, err := cpuset.Parse(hint.CPUs)
	if err != nil {
		log.Warn("invalid hint CPUs '%s' from %s", hint.CPUs, hint.Provider)
		return 0.0
	}
	common := hCPUs.Intersection(CPUs)
	return float64(common.Size()) / float64(hCPUs.Size())
}

// Calculate the NUMA node score of the given hint and NUMA node.
func numaHintScore(hint system.TopologyHint, sysIDs ...system.ID) float64 {
	for _, idstr := range strings.Split(hint.NUMAs, ",") {
		hID, err := strconv.ParseInt(idstr, 0, 0)
		if err != nil {
			log.Warn("invalid hint NUMA node %s from %s", idstr, hint.Provider)
			return 0.0
		}

		for _, id := range sysIDs {
			if hID == int64(id) {
				return 1.0
			}
		}
	}

	return 0.0
}

// Calculate the socket node score of the given hint and NUMA node.
func socketHintScore(hint system.TopologyHint, sysID system.ID) float64 {
	for _, idstr := range strings.Split(hint.Sockets, ",") {
		id, err := strconv.ParseInt(idstr, 0, 0)
		if err != nil {
			log.Warn("invalid hint socket '%s' from %s", idstr, hint.Provider)
			return 0.0
		}
		if id == int64(sysID) {
			return 1.0
		}
	}

	return 0.0
}

// return the cpuset for the CPU, NUMA or socket hints, preferred in this particular order.
func (cs *cpuSupply) hintCpus(h system.TopologyHint) cpuset.CPUSet {
	var cpus cpuset.CPUSet

	switch {
	case h.CPUs != "":
		cpus = cpuset.MustParse(h.CPUs)

	case h.NUMAs != "":
		for _, idstr := range strings.Split(h.NUMAs, ",") {
			if id, err := strconv.ParseInt(idstr, 0, 0); err == nil {
				if node := cs.node.System().Node(system.ID(id)); node != nil {
					cpus = cpus.Union(node.CPUSet())
				}
			}
		}

	case h.Sockets != "":
		for _, idstr := range strings.Split(h.Sockets, ",") {
			if id, err := strconv.ParseInt(idstr, 0, 0); err == nil {
				if pkg := cs.node.System().Package(system.ID(id)); pkg != nil {
					cpus = cpus.Union(pkg.CPUSet())
				}
			}
		}
	}

	return cpus
}

// a fake hint is of the format: target=[cpus:cpus[/nodes:nodes[/sockets:sockets]]];...
func (fh fakehints) parse(value string) error {
	hintSpecs := strings.Split(value, ";")
	for _, hintSpec := range hintSpecs {
		keyval := strings.Split(hintSpec, "=")
		if len(keyval) != 2 {
			return policyError("invalid fake hint '%s'", hintSpec)
		}

		target := keyval[0]
		spec := keyval[1]

		if spec == "-" {
			log.Debug("marking fake hints of %s for deletion", target)
			fh[target] = system.TopologyHints{}
			continue
		}

		hints, ok := fh[target]
		if !ok {
			hints = system.TopologyHints{}
		}

		fake := system.TopologyHint{}
		for _, keyval := range strings.Split(spec, "/") {
			kv := strings.Split(keyval, ":")
			if len(kv) != 2 {
				return policyError("invalid fake hint '%s', should be '{cpu,node,socket}:hint'",
					spec)
			}

			switch strings.ToLower(kv[0]) {
			case "cpu", "cpus":
				fake.CPUs = kv[1]
			case "node", "nodes", "numa", "numas":
				fake.NUMAs = kv[1]
			case "socket", "sockets":
				fake.Sockets = kv[1]
			default:
				return policyError("invalid hint type '%s' in hint '%s'", kv[0], spec)
			}
		}

		reenum := []system.TopologyHint{}
		count := 0
		for p, h := range hints {
			h.Provider = fmt.Sprintf("fake-hint#%d", count)
			count++
			reenum = append(reenum, h)
			delete(hints, p)
		}
		for _, h := range reenum {
			hints[h.Provider] = h
		}
		fake.Provider = fmt.Sprintf("fake-hint#%d", count)
		hints[fake.Provider] = fake

		fh[target] = hints
	}

	log.Debug("fake hints after parsing: %s", fh.String())

	return nil
}

// a fake hint is of the format: target=[cpus:cpus[/nodes:nodes[/sockets:sockets]]];...
func (fh fakehints) String() string {
	str := ""
	sep := ""

	for target, hints := range fh {
		hstr := ""
		hsep := ""
		for _, h := range hints {
			if h.CPUs != "" {
				hstr = "cpus:" + h.CPUs
				hsep = "/"
			}
			if h.NUMAs != "" {
				hstr += hsep + "nodes:" + h.NUMAs
				hsep = "/"
			}
			if h.Sockets != "" {
				hstr += hsep + "sockets:" + h.Sockets
			}
		}
		str += sep + target + "=" + hstr
		sep = ";"
	}

	return str
}
