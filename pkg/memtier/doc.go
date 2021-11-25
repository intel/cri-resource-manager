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

/*

	Package memtier implements process memory access tracking and
	policies for moving pages between NUMA nodes.

	Component types

	1. Policies (policy*.go) orchestrate trackers, watchers and
	page moving. They control what to track, when to track, how to
	interpret tracker counters, and trigger the Mover to handle
	page moving.

	2. Trackers (tracker*.go) track memory activity and export
	observations in TrackerCounters. Watchers (watcher*.go) tell
	trackers which processes to start/stop tracking. Performance
	and accuracy of a tracker depends on the method (softdirty,
	idlepages, damon), and configuration.

	3. The Mover (mover.go) moves pages between NUMA
	nodes.

	The Heat policy (policy_heat.go)

	The Heat policy updates a heatmap (counters_heatmap.go) from
	tracker counters. The heatmap gives a heat class for memory
	address ranges. The policy triggers page moving when a user
	has configured target NUMA nodes for a heat class of an
	address range, and if the address range is not already in any
	of the NUMA nodes.

	The Age policy (policy_age.go)

	The Age policy calculates active and idle ages of address
	ranges. Active age is the time during which an address range
	has been in accessed every time when observed. Idle age means
	the time since last access.

	The Damon tracker (tracker_damon.go)

	The Damon tracker reads address ranges and their activity from
	damon in the Linux kernel (since 5.15). Damon samples
	processes address space in varying ranges. The tracker reports
	all memory accesses on possibly overlapping address ranges of
	different lengths.

	The Idlepage tracker (tracker_idlepage.go)

	The idlepage tracker uses /sys/kernel/mm/page_idle/bitmap for
	setting and getting idle bits of pages in address ranges. The
	number of pages in an address range is a configurable
	constant. The tracker reports all memory accesses.

	The Softdirty tracker (tracker_softdirty.go)

	The softdirty tracker uses /proc/PID/clear_refs and the soft
	dirty bit in /proc/PID/pagemap to detect memory accesses. The
	number of pages in an address range is a configurable
	constant. The tracker detects only if a memory is written.

	Mover (mover.go)

	The mover moves address ranges from a NUMA node to
	another. The memory bandwidth (MB/s) can be limited, and move
	interval (ms) configured.

	Starting the Heat policy

		+-----------+               +---------------+
		|Heat Policy|-start/config->|Cgroups Watcher|
		+--+-----+--+               |(watch pids)   |
		   |     |                  +---------------+
		   |     |
		   |     |                  +----------------+
		   |     +----start/config->|Idlepage Tracker|
		   |                        +----------------+
		   |
		   |                        +-----+
		   +----------start/config->|Mover|
		                            +-----+

	Running the Heat policy

		+-----------+               +---------------+
		|Heat Policy|               |Cgroups Watcher|
		+--+-----+--+               |(watch pids)   |
		   |     ^                  +--+------------+
		   |     |                     V add/remove pid
		   |     |                  +----------------+
		   |     +----counters------|Idlepage Tracker|
		   |                        +----------------+
		   |
		   |                        +-----+
		   +----------add-tasks---->|Mover|
		                            +-----+

	Supporting modules

	The main components are supported by lower-level modules
	1. Process (process.go) has address ranges.
	2. AddrRanges (addrrange.go) has pages.
	3. Pages (page.go) can be moved and their statuses inspected.
	4. proc.go contains read/write/iteration of /proc and /sys files.
*/

package memtier
