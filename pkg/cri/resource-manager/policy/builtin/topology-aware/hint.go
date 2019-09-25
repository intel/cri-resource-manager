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
func (o *options) parseFakeHint(value string) error {
	specs := strings.Split(value, ";")
	for _, spec := range specs {
		targetfake := strings.Split(spec, "=")
		if len(targetfake) != 2 {
			return policyError("invalid fake hint spec '%s' among fake hints '%s'", spec, value)
		}

		target := targetfake[0]
		fake := targetfake[1]

		if fake == "-" {
			// mark for deletion during merge
			log.Debug("marking fake hints of %s for deletion...", target)
			opt.Hints[target] = system.TopologyHints{}
			continue
		}

		hints, ok := opt.Hints[target]
		if !ok {
			hints = system.TopologyHints{}
		}
		hintCnt := len(hints)
		hint := system.TopologyHint{Provider: fmt.Sprintf("fake-hint#%d", hintCnt)}

		for _, keyval := range strings.Split(fake, "/") {
			kv := strings.Split(keyval, ":")
			if len(kv) != 2 {
				return policyError("invalid fake hint '%s' among fake hint '%s'", keyval, fake)
			}

			switch kv[0] {
			case "cpu", "cpus":
				hint.CPUs = kv[1]
			case "node", "nodes", "numas":
				hint.NUMAs = kv[1]
			case "socket", "sockets":
				hint.Sockets = kv[1]
			default:
				return policyError("invalid hint parameter %s in fake hint %s", kv[0], keyval)
			}
		}

		hints[hint.Provider] = hint
		opt.Hints[target] = hints
	}

	return nil
}

// mergeFakeHints merges two sets of fake hints, removing effective duplicates.
func (o *options) mergeFakeHints(n *options) {
	if o.Hints == nil {
		o.Hints = make(map[string]system.TopologyHints)
	}

	for c, ohints := range o.Hints {
		if len(ohints) == 0 {
			log.Debug("deleting hints of %s", c)
			delete(o.Hints, c)
			delete(n.Hints, c)
		}
	}

	for c, nhints := range n.Hints {
		if ohints, ok := o.Hints[c]; !ok {
			if len(nhints) != 0 {
				o.Hints[c] = nhints
			}
		} else {
			for _, nh := range nhints {
				duplicate := false
				for _, oh := range ohints {
					if nh.CPUs == oh.CPUs && nh.NUMAs == oh.NUMAs && nh.Sockets == oh.Sockets {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				oh := nh
				oh.Provider = fmt.Sprintf("fake-hint#%d", len(o.Hints[c]))
				o.Hints[c][oh.Provider] = oh
			}
		}
	}
}
