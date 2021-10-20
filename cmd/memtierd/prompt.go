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

// This file implements prompt for memtierd testability.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/intel/cri-resource-manager/pkg/memtier"
)

type Cmd struct {
	description string
	Run         func([]string) commandStatus
}

type Prompt struct {
	r       *bufio.Reader
	w       *bufio.Writer
	f       *flag.FlagSet
	mover   *memtier.Mover
	pages   *memtier.Pages
	aranges *memtier.AddrRanges
	tracker memtier.Tracker
	cmds    map[string]Cmd
	ps1     string
	quit    bool
}

type commandStatus int

const (
	csOk commandStatus = iota
	csErr
)

func NewPrompt(ps1 string, reader *bufio.Reader, writer *bufio.Writer) *Prompt {
	p := Prompt{
		r:     reader,
		w:     writer,
		ps1:   ps1,
		mover: memtier.NewMover(),
	}
	p.cmds = map[string]Cmd{
		"q":       Cmd{"quit interactive prompt", p.cmdQuit},
		"tracker": Cmd{"track memory accesses and manage trackers", p.cmdTracker},
		"stats":   Cmd{"print statistics", p.cmdStats},
		"pages":   Cmd{"select pages, or print current pages", p.cmdPages},
		"arange":  Cmd{"select/split/filter arange", p.cmdArange},
		"mover":   Cmd{"move selected pages or manage mover", p.cmdMover},
		"help":    Cmd{"print help", p.cmdHelp},
	}
	return &p
}

func (p *Prompt) output(format string, a ...interface{}) {
	if p.w == nil {
		return
	}
	p.w.WriteString(fmt.Sprintf(format, a...))
	p.w.Flush()
}

func (p *Prompt) interact() {
	for !p.quit {
		p.output(p.ps1)
		rawcmd, err := p.r.ReadString(byte('\n'))
		if err != nil {
			p.output("quit: %s\n", err)
			break
		}
		cmdSlice := strings.Split(strings.TrimSpace(rawcmd), " ")
		if len(cmdSlice) == 0 {
			continue
		}
		p.f = flag.NewFlagSet(cmdSlice[0], flag.ContinueOnError)
		if cmd, ok := p.cmds[cmdSlice[0]]; ok {
			cmd.Run(cmdSlice[1:])
		} else if len(cmdSlice[0]) > 0 {
			p.output("unknown command\n")
		}
	}
	p.output("quit.\n")
}

func sortedStringKeys(m map[string]Cmd) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedNodeKeys(m map[memtier.Node]uint) []memtier.Node {
	keys := make([]int, 0, len(m))
	nodes := make([]memtier.Node, len(m))
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for i, key := range keys {
		nodes[i] = memtier.Node(key)
	}
	return nodes
}

func (p *Prompt) cmdHelp(args []string) commandStatus {
	for _, name := range sortedStringKeys(p.cmds) {
		p.output("%-12s %s\n", name, p.cmds[name].description)
	}
	return csOk
}

func (p *Prompt) cmdArange(args []string) commandStatus {
	pid := p.f.Int("pid", -1, "look for pages of PID")
	ls := p.f.Bool("ls", false, "list address ranges")
	splitLen := p.f.Uint64("split-length", 0, "split long ranges into many ranges of max LENGTH")
	minLen := p.f.Uint64("min-length", 0, "exclude address ranges sorter than LENGTH")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	if *pid != -1 {
		process := memtier.NewProcess(*pid)
		if ar, err := process.AddressRanges(); err == nil {
			p.aranges = ar
		} else {
			p.output("error selecting aranges for pid %d: %v\n",
				*pid, err)
			return csOk
		}
	}
	if p.aranges == nil {
		p.output("no aranges selected, use -pid PID\n")
		return csOk
	}
	if *splitLen != 0 {
		p.aranges = p.aranges.SplitLength(*splitLen)
	}
	if *minLen != 0 {
		p.aranges = p.aranges.Filter(func(ar memtier.AddrRange) bool {
			return ar.Length() >= *minLen
		})
	}
	if *ls {
		for _, r := range p.aranges.Ranges() {
			p.output("%s\n", r)
		}
	}
	p.output("selected address ranges: %d\n", len(p.aranges.Ranges()))
	return csOk
}

func (p *Prompt) cmdPages(args []string) commandStatus {
	pid := p.f.Int("pid", -1, "look for pages of PID")
	attrs := p.f.String("attrs", "", "include only <Exclusive,Dirty,NotDirty,InHeap,InAnonymous> pages")
	ranges := p.f.String("ranges", "", "-ranges=START-STOP[,START-STOP...] include only given ranges")
	fromNode := p.f.Int("node", -1, "include only pages currently on NODE")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	// if launched without arguments, report current status of selected pages
	if len(args) == 0 {
		if p.pages == nil {
			p.output("no pages selelected, try pages -pid PID\n")
			return csOk
		}
		nodePageCount := p.pages.NodePageCount()
		p.output("pages of pid %d\n", p.pages.Pid())
		for _, node := range sortedNodeKeys(nodePageCount) {
			p.output("node %d: %d\n", node, nodePageCount[node])
		}
		return csOk
	}
	// there are arguments, select new set of pages
	if *pid <= 0 {
		p.output("missing -pid=PID\n")
		return csOk
	}
	p.output("selecting pages of pid %d\n", *pid)
	process := memtier.NewProcess(*pid)
	ar, err := process.AddressRanges()
	if err != nil {
		p.output("error reading address ranges of process %d: %v\n", *pid, err)
		return csOk
	}
	if ar == nil {
		p.output("address ranges not found for process %d\n", *pid)
		return csOk
	}
	p.output("found %d address ranges\n", len(ar.Ranges()))
	selectedRanges := []memtier.AddrRange{}
	if *ranges != "" {
		selectedRanges = parseOptRanges(*ranges)
	}
	if len(selectedRanges) > 0 {
		ar.Intersection(selectedRanges)
		p.output("using %d address ranges\n", len(ar.Ranges()))
	}
	if len(ar.Ranges()) == 0 {
		p.output("no address ranges from which to find pages\n")
		return csOk
	}
	p.aranges = ar
	p.output("aranges = <%d address ranges of pid %d>\n", len(ar.Ranges()), ar.Pid())
	pageAttributes, err := parseOptPages(*attrs)
	if err != nil {
		p.output("invalid -attrs: %v\n", err)
		return csOk
	}
	pp, err := ar.PagesMatching(pageAttributes)
	if err != nil {
		p.output("finding pages from address ranges failed: %v\n", err)
	}
	p.output("pages = <%d pages of pid %d>\n", len(pp.Pages()), pp.Pid())
	if *fromNode >= 0 {
		pp = pp.OnNode(memtier.Node(*fromNode))
	}
	p.pages = pp
	return csOk
}

func (p *Prompt) cmdMover(args []string) commandStatus {
	config := p.f.String("config", "", "reconfigure mover with JSON string")
	pagesTo := p.f.Int("pages-to", -1, "move pages to NODE int")
	pause := p.f.Bool("pause", false, "pause moving")
	cont := p.f.Bool("continue", false, "continue moving")
	tasks := p.f.Bool("tasks", false, "print current tasks")
	removeTask := p.f.Int("remove-task", -1, "remove task ID")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	if *config != "" {
		if err := p.mover.SetConfigJson(*config); err != nil {
			p.output("mover reconfiguration error: %v\n", err)
		}
	}
	if *pagesTo >= 0 {
		err := p.mover.Start()
		if err != nil {
			p.output("mover error: %v\n", err)
			return csOk
		}
		if p.pages == nil {
			p.output("mover error: set pages before moving\n")
			return csOk
		}
		toNode := memtier.Node(*pagesTo)
		task := memtier.NewMoverTask(p.pages, toNode)
		p.mover.AddTask(task)
	}
	if *pause {
		p.mover.Pause()
	}
	if *cont {
		p.mover.Continue()
	}
	if *tasks {
		for taskId, task := range p.mover.Tasks() {
			p.output("%-8d %s\n", taskId, task)
		}
	}
	if *removeTask != -1 {
		p.mover.RemoveTask(*removeTask)
	}
	return csOk
}

func (p *Prompt) cmdStats(args []string) commandStatus {
	moves := p.f.Bool("m", false, "dump all moves")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	if *moves {
		p.output(memtier.Stats().Dump() + "\n")
	} else {
		p.output(memtier.Stats().Summarize() + "\n")
	}
	return csOk
}

func (p *Prompt) cmdTracker(args []string) commandStatus {
	ls := p.f.Bool("ls", false, "list available memory trackers")
	create := p.f.String("create", "", "create new tracker NAME")
	config := p.f.String("config", "", "configure tracker with JSON string")
	start := p.f.String("start", "", "start tracking PID[,PID...]")
	reset := p.f.Bool("reset", false, "reset page access counters")
	stop := p.f.Bool("stop", false, "stop tracker")
	counters := p.f.Bool("counters", false, "read page access counters")

	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	if *ls {
		p.output(strings.Join(memtier.TrackerList(), "\n") + "\n")
		return csOk
	}
	if *create != "" {
		if tracker, err := memtier.NewTracker(*create); err != nil {
			p.output("creating tracker failed: %v\n", err)
			return csOk
		} else {
			p.tracker = tracker
			p.output("tracker created\n")
		}
	}
	// Next actions will require existing tracker
	if p.tracker == nil {
		p.output("no tracker, create one with -create NAME [-config CONFIG]\n")
		return csOk
	}
	if *config != "" {
		if err := p.tracker.SetConfigJson(*config); err != nil {
			p.output("tracker configuration error: %v\n", err)
		} else {
			p.output("tracker configured successfully\n")
		}
	}
	if *stop {
		p.tracker.Stop()
	}
	if *start != "" {
		p.tracker.Stop()
		p.tracker.RemovePids(nil)
		for _, pidStr := range strings.Split(*start, ",") {
			if pid, err := strconv.Atoi(pidStr); err == nil {
				p.tracker.AddPids([]int{pid})
			} else {
				p.output("invalid pid: %q\n", pidStr)
				return csOk
			}
		}
		if err := p.tracker.Start(); err != nil {
			p.output("start failed: %v\n", err)
			return csOk
		}
		p.output("tracker started\n")
	}
	if *counters {
		tcs := p.tracker.GetCounters()
		if tcs == nil {
			p.output("cannot get tracker counters\n")
			return csOk
		}
		if len(*tcs) == 0 {
			p.output("no counter entries\n")
			return csOk
		}
		tcs.SortByAccesses()
		p.output(tcs.String() + "\n")
	}
	if *reset {
		p.tracker.ResetCounters()
		p.output("tracker counters reset\n")
	}
	return csOk
}

func (p *Prompt) cmdQuit(args []string) commandStatus {
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	p.quit = true
	return csOk
}
