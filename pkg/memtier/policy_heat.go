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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type PolicyHeatConfig struct {
	PidWatcher PidWatcherConfig
	Tracker    TrackerConfig
	Heatmap    HeatmapConfig
	Forecaster *HeatForecasterConfig
	Mover      MoverConfig
	// Cgroups is a list of cgroup paths in the filesystem. The
	// policy manages processes in listed cgroups and recursively
	// in their subgroups.
	// DEPRECATED, use PidWatcher "cgroup" instead.
	Cgroups []string
	// Pids is a list of process id's to be tracked.
	// DEPRECATED, use PidWatcher "pid" instead.
	Pids []int
	// IntervalMs is the length of the period in milliseconds
	// in which new heats are calculated for pages based on gathered
	// tracker values, and page move tasks are triggered.
	IntervalMs int
	// HeatNumas maps heat class values into NUMA node lists where
	// pages of each heat class should be located. If a heat class
	// is missing, the NUMA node is "don't care".
	HeatNumas map[int][]int
	// NumaSize sets the amount of memory that is usable on each
	// NUMA node. If a node is missing from the map, it's memory
	// use is not limited. The size is expressed in syntax:
	// <NUM>(k|M|G|%). If all the memory in a heat class exceeds
	// NumaSize of the NUMA nodes of that heat, the remaining
	// pages are moved to NUMA nodes of lower heats if there is
	// free capacity.
	NumaSize map[int]string
}

type PolicyHeat struct {
	config       *PolicyHeatConfig
	pidwatcher   PidWatcher
	chLoop       chan interface{} // for communication to the main loop of the policy
	tracker      Tracker
	heatmap      *Heatmap
	pidAddrDatas map[int]*AddrDatas
	mover        *Mover
	forecaster   HeatForecaster
	numaUsed     map[Node]int // used capacity (in pages) on each NUMA node
	numaSize     map[Node]int // total capacity (in pages) on each NUMA node
}

type pageInfo struct {
	node Node // NUMA node where a page is located
}

const (
	constNUMASIZE_UNLIMITED = -1
)

func init() {
	PolicyRegister("heat", NewPolicyHeat)
}

func NewPolicyHeat() (Policy, error) {
	p := &PolicyHeat{
		heatmap:      NewCounterHeatmap(),
		pidAddrDatas: make(map[int]*AddrDatas),
		mover:        NewMover(),
		numaUsed:     make(map[Node]int),
		numaSize:     make(map[Node]int),
	}
	return p, nil
}

func (p *PolicyHeat) SetConfigJson(configJson string) error {
	config := &PolicyHeatConfig{}
	if err := unmarshal(configJson, config); err != nil {
		return err
	}
	return p.SetConfig(config)
}

func (p *PolicyHeat) SetConfig(config *PolicyHeatConfig) error {
	if config.IntervalMs <= 0 {
		return fmt.Errorf("invalid heat policy IntervalMs: %d, > 0 expected", config.IntervalMs)
	}
	if len(config.Cgroups) > 0 {
		return deprecatedPolicyCgroupsConfig("heat")
	}
	if len(config.Pids) > 0 {
		return deprecatedPolicyPidsConfig("heat")
	}
	if config.PidWatcher.Name == "" {
		return fmt.Errorf("pidwatcher name missing from the age policy configuration")
	}
	newPidWatcher, err := NewPidWatcher(config.PidWatcher.Name)
	if err != nil {
		return err
	}
	if err = newPidWatcher.SetConfigJson(config.PidWatcher.Config); err != nil {
		return fmt.Errorf("configuring pidwatcher %q for the age policy failed: %w", config.PidWatcher.Name, err)
	}

	if config.Tracker.Name == "" {
		return fmt.Errorf("tracker name missing from the heat policy configuration")
	}
	newTracker, err := NewTracker(config.Tracker.Name)
	if err != nil {
		return err
	}
	if config.Tracker.Config != "" {
		if err = newTracker.SetConfigJson(config.Tracker.Config); err != nil {
			return fmt.Errorf("configuring tracker %q for the heat policy failed: %s", config.Tracker.Name, err)
		}
	}
	newNumaUsed := make(map[Node]int)
	newNumaSize := make(map[Node]int)
	for _, numas := range config.HeatNumas {
		for _, nodeInt := range numas {
			newNumaSize[Node(nodeInt)] = constNUMASIZE_UNLIMITED // the default is unlimited
		}
	}
	for nodeInt, sizeString := range config.NumaSize {
		node := Node(nodeInt)
		sizeBytes, err := ParseBytes(sizeString)
		if err != nil {
			return fmt.Errorf("NumaSize[%d]: %s", node, err)
		}
		newNumaSize[node] = int(sizeBytes / constPagesize)
	}
	err = p.heatmap.SetConfig(&config.Heatmap)
	if err != nil {
		return fmt.Errorf("heatmap configuration error: %s", err)
	}
	if err = p.mover.SetConfig(&config.Mover); err != nil {
		return fmt.Errorf("configuring mover failed: %s", err)
	}
	if config.Forecaster != nil {
		p.forecaster, err = NewHeatForecaster(config.Forecaster.Name)
		if err != nil || p.forecaster == nil {
			return fmt.Errorf("creating heat forecaster %q failed: %s", config.Forecaster.Name, err)
		}
		if err = p.forecaster.SetConfigJson(config.Forecaster.Config); err != nil {
			return fmt.Errorf("configuring heat forecaster %q failed: %s", config.Forecaster.Name, err)
		}
	}
	p.switchToTracker(newTracker)
	p.pidwatcher = newPidWatcher
	p.numaUsed = newNumaUsed
	p.numaSize = newNumaSize
	p.config = config
	return nil
}

func (p *PolicyHeat) switchToTracker(newTracker Tracker) {
	if p.tracker != nil {
		p.tracker.Stop()
	}
	p.tracker = newTracker
}

func (p *PolicyHeat) GetConfigJson() string {
	if p.config == nil {
		return ""
	}
	pconfig := *p.config
	if p.tracker != nil {
		pconfig.Tracker.Config = p.tracker.GetConfigJson()
	}
	if configStr, err := json.Marshal(&pconfig); err == nil {
		return string(configStr)
	}
	return ""
}

func (p *PolicyHeat) PidWatcher() PidWatcher {
	return p.pidwatcher
}

func (p *PolicyHeat) Mover() *Mover {
	return p.mover
}

func (p *PolicyHeat) Tracker() Tracker {
	return p.tracker
}

func (p *PolicyHeat) Dump(args []string) string {
	dumpHelp := "dump <forecast [PARAMS]|heatmap|heatgram [CLASSES]|numa>"
	if len(args) == 0 {
		return dumpHelp
	}
	if args[0] == "forecast" {
		if p.forecaster == nil {
			return "no forecaster"
		}
		return p.forecaster.Dump(args[1:])
	}
	if args[0] == "heatmap" {
		lines := []string{}
		lines = append(lines, "heatmap:", p.heatmap.Dump())
		return strings.Join(lines, "\n")
	}
	if args[0] == "heatgram" {
		classCount := p.heatmap.config.HeatClasses
		var err error
		if len(args) > 1 {
			if classCount, err = strconv.Atoi(args[1]); err != nil || classCount < 1 {
				return "invalid argument, expected CLASSES > 0, syntax: heatgram CLASSES"
			}
		}
		lines := []string{}
		// Find the following properties of the heatmap:
		hrCount := 0            // number of heatranges
		pageCount := uint64(0)  // total number of pages in the heatmap (all pids)
		maxHeat := float64(0.0) // maximum heat that appears in the heatmap
		pidMaxHeat := map[int]float64{}
		lines = append(lines, "", "table: maximum heat in heatmaps")
		lines = append(lines, fmt.Sprintf("     pid  maxHeat"))
		for _, pid := range sortInts(p.heatmap.Pids()) {
			p.heatmap.ForEachRange(pid, func(hr *HeatRange) int {
				if hr.heat > maxHeat {
					maxHeat = hr.heat
				}
				if hr.heat > pidMaxHeat[pid] {
					pidMaxHeat[pid] = hr.heat
				}
				hrCount += 1
				pageCount += hr.length
				return 0
			})
			lines = append(lines, fmt.Sprintf("%8d %8.4f", pid, pidMaxHeat[pid]))
		}
		// Build statistics on each pid and class separately.
		lines = append(lines, "", "table: memory in heat classes")
		for _, pid := range sortInts(p.heatmap.Pids()) {
			classPages := map[int]uint64{} // pages per class in this pid
			totPages := uint64(0)          // total pages in this pid
			p.heatmap.ForEachRange(pid, func(hr *HeatRange) int {
				hrClass := int(float64(classCount) * hr.heat / p.heatmap.config.HeatMax)
				if hrClass >= classCount {
					hrClass--
				}
				classPages[hrClass] += hr.length
				totPages += hr.length
				return 0
			})
			lines = append(lines, "     pid class pidmem[%] totmem[%]    mem[G]")
			for classNum := 0; classNum < classCount; classNum++ {
				lines = append(lines, fmt.Sprintf("%8d %5d %9.2f %9.2f %9.3f",
					pid,
					classNum,
					float32(100*classPages[classNum])/float32(totPages),
					float32(100*classPages[classNum])/float32(pageCount),
					float32(classPages[classNum]*constUPagesize)/float32(1024*1024*1024)))
			}
		}
		return strings.Join(lines, "\n")
	}
	if args[0] == "numa" {
		lines := []string{}
		lines = append(lines, "node      pid pageData numaUsed numaSize")
		for _, pid := range sortInts(p.heatmap.Pids()) {
			nodeUsed := mapNodeUint64{}
			addrDatas := p.pidAddrDatas[pid]
			if addrDatas == nil {
				continue
			}
			addrDatas.ForEach(func(ar *AddrRange, data interface{}) int {
				arpi := data.(pageInfo)
				nodeUsed[arpi.node] += ar.length * constUPagesize
				return 0
			})
			for _, node := range nodeUsed.sortedKeys() {
				used := nodeUsed[node]
				lines = append(lines, fmt.Sprintf("%4d %8d %7dM %7dM %7dM",
					node,
					pid,
					used/(1024*1024),
					int64(p.numaUsed[node])*constPagesize/(1024*1024),
					int64(p.numaSize[node])*constPagesize/(1024*1024)))
			}
		}
		return strings.Join(lines, "\n")
	}
	return dumpHelp
}

func (p *PolicyHeat) Stop() {
	if p.pidwatcher != nil {
		p.pidwatcher.Stop()
	}
	if p.tracker != nil {
		p.tracker.Stop()
	}
	if p.chLoop != nil {
		p.chLoop <- struct{}{}
	}
	if p.mover != nil {
		p.mover.Stop()
	}
}

func (p *PolicyHeat) Start() error {
	if p.chLoop != nil {
		return fmt.Errorf("already started")
	}
	if p.config == nil {
		return fmt.Errorf("unconfigured policy")
	}
	if p.pidwatcher == nil {
		return fmt.Errorf("missing pidwatcher")
	}
	if p.tracker == nil {
		return fmt.Errorf("missing tracker")
	}
	if err := p.tracker.Start(); err != nil {
		return fmt.Errorf("tracker start error: %w", err)
	}
	p.chLoop = make(chan interface{})
	p.pidwatcher.SetPidListener(p.tracker)
	p.pidwatcher.Start()
	// p.pidwatcher.SetSources(p.config.Cgroups)
	// if len(p.config.Cgroups) > 0 {
	// 	p.pidwatcher.Start(p.tracker)
	// }
	// if len(p.config.Pids) > 0 {
	// 	p.tracker.AddPids(p.config.Pids)
	// }
	if err := p.mover.Start(); err != nil {
		return fmt.Errorf("mover start error: %w", err)
	}
	go p.loop()
	return nil
}

func (p *PolicyHeat) loop() {
	log.Debugf("PolicyHeat: online\n")
	defer log.Debugf("PolicyHeat: offline\n")
	ticker := time.NewTicker(time.Duration(p.config.IntervalMs) * time.Millisecond)
	defer ticker.Stop()
	quit := false
	n := uint64(0)
	for !quit {
		stats.Store(StatsHeartbeat{"PolicyHeat.loop"})
		timestamp := time.Now().UnixNano()
		newCounters := p.tracker.GetCounters()
		p.tracker.ResetCounters()
		log.Debugf("PolicyHeat: updating heatmap with %d address ranges\n", len(*newCounters))
		p.heatmap.UpdateFromCounters(newCounters, timestamp)

		// If we get a heat forecast, make memory moves based on that
		// and restore real heats after moves have been created.
		var realHeats Heats
		if p.forecaster != nil {
			heatForecast, err := p.forecaster.Forecast(&p.heatmap.pidHrs)
			if err != nil {
				stats.Store(StatsHeartbeat{fmt.Sprintf("forecaster %q error: %s", p.config.Forecaster, err)})
			}
			if heatForecast != nil {
				realHeats = p.heatmap.pidHrs
				p.heatmap.pidHrs = *heatForecast
				stats.Store(StatsHeartbeat{"use forecast"})
			}
		}
		p.updatePagedOutLocations(timestamp)
		if p.mover.TaskCount() == 0 {
			p.startMoves(timestamp)
		}
		if realHeats != nil {
			stats.Store(StatsHeartbeat{"rollback from forecast"})
			p.heatmap.pidHrs = realHeats
		}
		n += 1
		select {
		case <-p.chLoop:
			quit = true
			break
		case <-ticker.C:
			// TODO:
			// Go through which pages should be moved.
			continue
		}
	}
	close(p.chLoop)
	p.chLoop = nil
}

// updatePagedOutLocations: go through expected location of memory.
// If an address range is expected to be on swap, check if it is still
// there. If a single page in the range has been swapped back in
// memory, mark the location (NUMA node / swap) of the whole address
// range unknown.
func (p *PolicyHeat) updatePagedOutLocations(timestamp int64) {
	// TODO: limit checks to n address ranges per call to avoid
	// too large cost on processes with huge amounts of swapped
	// out memory.
	checkPidArs := make(map[int][]*AddrRange)
	for pid, addrDatas := range p.pidAddrDatas {
		addrDatas.ForEach(func(ar *AddrRange, data interface{}) int {
			arpi := data.(pageInfo)
			if arpi.node == NODE_SWAP {
				checkPidArs[pid] = append(checkPidArs[pid], ar)
			}
			return 0
		})
	}
	for pid, ars := range checkPidArs {
		pmFile, err := ProcPagemapOpen(pid)
		if err != nil {
			continue
		}
		for _, ar := range ars {
			pmFile.ForEachPage([]AddrRange{*ar}, 0,
				func(pmBits, pageAddr uint64) int {
					if (pmBits>>PMB_SWAP)&1 == 1 {
						return 0 // swapped out as expected
					}
					p.pidAddrDatas[pid].SetData(*ar, pageInfo{node: NODE_UNDEFINED})
					return -1
				})
		}
		pmFile.Close()
	}
}

func (p *PolicyHeat) startMoves(timestamp int64) {
	if len(p.numaSize) == 0 {
		p.startMovesNoLimits(timestamp)
	} else {
		p.startMovesFillFastFree(timestamp)
	}
}

func (p *PolicyHeat) startMovesFillFastFree(timestamp int64) {
	moverTasks := 0
	for _, pid := range p.heatmap.Pids() {
		hrHotToCold := p.heatmap.Sorted(pid, func(hr0, hr1 *HeatRange) bool {
			if hr0.heat > hr1.heat ||
				(hr0.heat == hr1.heat && hr0.addr < hr1.addr) {
				return true
			}
			return false
		})
		for _, hr := range hrHotToCold {
			currNode := NODE_UNDEFINED
			heatClass := p.heatmap.HeatClass(hr)
			numas, ok := p.config.HeatNumas[heatClass]
			if !ok || len(numas) == 0 {
				// No NUMAs for this heat class, do nothing
				continue
			}
			addrDatas, ok := p.pidAddrDatas[pid]
			if !ok {
				addrDatas = NewAddrDatas()
				p.pidAddrDatas[pid] = addrDatas
			}
			// TODO: heatrange hr may be on many nodes
			// when using variable address ranges (damon).
			// Now using just only the start address of a
			// heatrange, which can be used as a fast path
			// for stable ranges.
			pagesOnUnknownNode := false
			if addrData, ok := addrDatas.Data(hr.addr); ok {
				addrInfo, _ := addrData.(pageInfo)
				currNode = addrInfo.node
			}
			if currNode == NODE_UNDEFINED {
				pagesOnUnknownNode = true
			}
			var ppages *Pages = nil
			var err error
			if pagesOnUnknownNode {
				// We do not know where these pages
				// are. Let's figure it out.
				ppages, err = NewAddrRanges(pid, hr.AddrRange()).PagesMatching(PMPresentSet | PMExclusiveSet)
				if err != nil {
					continue
				}
				firstPageAddress := uint64(0)
				prevPageAddress := uint64(0)
				prevPageNode := NODE_UNDEFINED
				node := prevPageNode
				nodeInts, err := ppages.status()
				if err != nil {
					continue
				}
				for pageIndex, nodeInt := range nodeInts {
					node = Node(nodeInt)
					pageAddress := ppages.pages[pageIndex].Addr()
					if prevPageAddress+constUPagesize != pageAddress || prevPageNode != node {
						// previous page was
						// the last one in a
						// contiguous sequence
						// of pages on a node
						if firstPageAddress != 0 {
							addrDatas.SetData(*NewAddrRange(firstPageAddress, prevPageAddress+constUPagesize), pageInfo{node: prevPageNode})
							p.numaUsed[node] += int((prevPageAddress + constUPagesize - firstPageAddress) / constUPagesize)
						}
						firstPageAddress = pageAddress
						prevPageAddress = pageAddress
						prevPageNode = node
					}
					prevPageAddress = pageAddress
				}
				if firstPageAddress > 0 {
					addrDatas.SetData(*NewAddrRange(firstPageAddress, prevPageAddress+constUPagesize), pageInfo{node: Node(prevPageNode)})
					p.numaUsed[node] += int((prevPageAddress + constUPagesize - firstPageAddress) / constUPagesize)
					// log.Debugf("found last %d pages at %x on node %d\n", (prevPageAddress+constUPagesize-firstPageAddress)/constUPagesize, firstPageAddress, prevPageNode)
				}
				currNode = node
			}
			if sliceContainsInt(numas, int(currNode)) {
				// Already on a good node, do nothing.
				continue
			}
			if currNode == NODE_UNDEFINED {
				// Failed to find out where the pages are.
				continue
			}
			// We know pages are on a wrong node. Choose
			// new node with largest free space for the
			// pages. TODO: filter mems_allowed from numas
			destNode := NODE_UNDEFINED
			destFree := -1
			for _, candNodeInt := range numas {
				candNode := Node(candNodeInt)
				candFree := 0
				if p.numaSize[candNode] != constNUMASIZE_UNLIMITED {
					candFree = p.numaSize[candNode] - p.numaUsed[candNode] - int(hr.length)
				}
				if candFree > destFree {
					destNode = candNode
					destFree = candFree
				}
			}
			if destNode == NODE_UNDEFINED {
				// Failed to find proper destination node.
				continue
			}
			// Is there enough free space for pages of
			// this heat range?
			if p.numaSize[destNode] != constNUMASIZE_UNLIMITED && destFree < int(hr.length) {
				// Failed to find a destination node with enough quota.
				continue
			}
			if ppages == nil {
				ppages, err = NewAddrRanges(pid, hr.AddrRange()).PagesMatching(PMPresentSet | PMExclusiveSet)
				if err != nil {
					// Error in finding page list.
					continue
				}
			}
			if len(ppages.pages) == 0 {
				// The address range contains no pages that could be moved.
				continue
			}
			moverTasks += 1
			task := NewMoverTask(ppages, destNode)
			p.mover.AddTask(task)
			addrDatas.SetData(hr.AddrRange(), pageInfo{node: destNode})
			p.numaUsed[destNode] += int(hr.length)
			// numaUsed[-1] will contain the number of pages moved away from
			// an unknown node.
			p.numaUsed[currNode] -= int(hr.length)
		}
	}
}

func (p *PolicyHeat) startMovesNoLimits(timestamp int64) {
	moverTasks := 0
	for _, pid := range p.heatmap.Pids() {
		p.heatmap.ForEachRange(pid, func(hr *HeatRange) int {
			// TODO: config: is the information fresh enough for a decision?
			if timestamp-hr.updated > 10*int64(time.Second) {
				return 0
			}
			// TODO: config: has the range stable (old) enough?
			if timestamp-hr.created < 20*int64(time.Second) {
				return 0
			}
			heatClass := p.heatmap.HeatClass(hr)
			numas, ok := p.config.HeatNumas[heatClass]
			if !ok || len(numas) == 0 {
				// No NUMAs for this heat class
				return 0
			}
			// TODO: calculate numas in mems_allowed
			destNode := Node(numas[0])
			// TODO: check current NUMA nodes of the
			// range, do not move if already there.
			ar := NewAddrRanges(pid, hr.AddrRange())
			ppages, err := ar.PagesMatching(PMPresentSet | PMExclusiveSet)
			if err != nil {
				return -1
			}
			ppages = ppages.NotOnNode(destNode)
			if len(ppages.pages) == 0 {
				return 0
			}
			moverTasks += 1
			task := NewMoverTask(ppages, destNode)
			p.mover.AddTask(task)
			return 0
		})
	}
	if moverTasks > 0 {
		log.Debugf("created %d mover tasks\n", moverTasks)
	}
}
