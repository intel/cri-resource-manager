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

package sysfs

import (
	"sort"
	"strconv"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	// Unknown represents an unknown id.
	Unknown Id = -1
)

// Id is nn integer id, used to identify packages, CPUs, nodes, etc.
type Id int

// IdSet is an unordered set of integer ids.
type IdSet map[Id]struct{}

// NewIdSet creates a new unordered set of (integer) ids.
func NewIdSet(ids ...Id) IdSet {
	s := make(map[Id]struct{})

	for _, id := range ids {
		s[id] = struct{}{}
	}

	return s
}

// NewIdSetFromIntSlice creates a new unordered set from an integer slice.
func NewIdSetFromIntSlice(ids ...int) IdSet {
	s := make(map[Id]struct{})

	for _, id := range ids {
		s[Id(id)] = struct{}{}
	}

	return s
}

// Clone returns a copy of this IdSet.
func (s IdSet) Clone() IdSet {
	return NewIdSet(s.Members()...)
}

// Add adds the given ids into the set.
func (s IdSet) Add(ids ...Id) {
	for _, id := range ids {
		s[id] = struct{}{}
	}
}

// Del deletes the given ids from the set.
func (s IdSet) Del(ids ...Id) {
	if s != nil {
		for _, id := range ids {
			delete(s, id)
		}
	}
}

// Size returns the number of ids in the set.
func (s IdSet) Size() int {
	return len(s)
}

// Has tests if all the ids are present in the set.
func (s IdSet) Has(ids ...Id) bool {
	if s == nil {
		return false
	}

	for _, id := range ids {
		_, ok := s[id]
		if !ok {
			return false
		}
	}

	return true
}

// Members returns all ids in the set as a randomly ordered slice.
func (s IdSet) Members() []Id {
	if s == nil {
		return []Id{}
	}
	ids := make([]Id, len(s))
	idx := 0
	for id := range s {
		ids[idx] = id
		idx++
	}
	return ids
}

// SortedMembers returns all ids in the set as a sorted slice.
func (s IdSet) SortedMembers() []Id {
	ids := s.Members()
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

// CPUSet returns a cpuset.CPUSet corresponding to an id set.
func (s IdSet) CPUSet() cpuset.CPUSet {
	b := cpuset.NewBuilder()
	for id := range s {
		b.Add(int(id))
	}
	return b.Result()
}

// FromCPUSet returns an id set corresponding to a cpuset.CPUSet.
func FromCPUSet(cset cpuset.CPUSet) IdSet {
	return NewIdSetFromIntSlice(cset.ToSlice()...)
}

// String returns the set as a string.
func (s IdSet) String() string {
	return s.StringWithSeparator(",")
}

// StringWithSeparator returns the set as a string, separated with the given separator.
func (s IdSet) StringWithSeparator(args ...string) string {
	if s == nil || len(s) == 0 {
		return ""
	}

	var sep string

	if len(args) == 1 {
		sep = args[0]
	} else {
		sep = ","
	}

	str := ""
	t := ""
	for _, id := range s.SortedMembers() {
		str = str + t + strconv.Itoa(int(id))
		t = sep
	}

	return str
}
