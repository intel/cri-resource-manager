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

func (pp *Pages) Pid() int {
	return pp.pid
}

func (pp *Pages) Pages() []Page {
	return pp.pages
}

// InAddrRanges returns process pages that are in any of given address ranges
func (pp *Pages) InAddrRanges(addrRanges ...AddrRange) *Pages {
	// TODO: Implement me!

	return &Pages{pid: pp.pid}
}

func (pp *Pages) MoveTo(node Node, count uint) error {
	pageCount, pages := pp.countAddrs()
	if count > pageCount {
		count = pageCount
	}
	flags := MPOL_MF_MOVE
	pages = pages[:count]
	nodes := make([]int, count)
	for i := range nodes {
		nodes[i] = int(node)
	}
	_, _, err := movePagesSyscall(pp.pid, count, pages, nodes, flags)
	return err
}

// OnNode returns only those Pages that are on the given node.
func (pp *Pages) OnNode(node Node) *Pages {
	currentStatus, err := pp.status()
	if err != nil {
		return nil
	}
	np := &Pages{pid: pp.pid}
	intNode := int(node)
	for i, p := range pp.pages {
		if currentStatus[i] == intNode {
			np.pages = append(np.pages, p)
		}
	}
	return np
}

// OnNode returns only those Pages that are not on the given node.
func (pp *Pages) NotOnNode(node Node) *Pages {
	currentStatus, err := pp.status()
	if err != nil {
		return nil
	}
	np := &Pages{pid: pp.pid}
	intNode := int(node)
	for i, p := range pp.pages {
		if currentStatus[i] != intNode {
			np.pages = append(np.pages, p)
		}
	}
	return np
}

func (pp *Pages) countAddrs() (uint, []uintptr) {
	count := uint(len(pp.pages))
	addrs := make([]uintptr, count)
	for i := uint(0); i < count; i++ {
		addrs[i] = uintptr(pp.pages[i].addr)
	}
	return count, addrs
}

func (pp *Pages) status() ([]int, error) {
	pageCount, pages := pp.countAddrs()
	flags := MPOL_MF_MOVE
	_, currentStatus, err := movePagesSyscall(pp.pid, pageCount, pages, nil, flags)
	return currentStatus, err
}

// NodePageCount returns map: numanode -> number of pages on the node
func (pp *Pages) NodePageCount() map[Node]uint {
	currentStatus, err := pp.status()
	if err != nil {
		return nil
	}

	pageErrors := 0
	nodePageCount := make(map[Node]uint)

	for _, pageStatus := range currentStatus {
		if pageStatus < 0 {
			pageErrors += 1
			continue
		}
		nodePageCount[Node(pageStatus)] += 1
	}
	return nodePageCount
}

func (pp *Pages) Nodes() []Node {
	nodePageCount := pp.NodePageCount()
	nodes := make([]Node, len(nodePageCount))
	for node, _ := range nodePageCount {
		nodes = append(nodes, node)
	}
	return nodes
}
