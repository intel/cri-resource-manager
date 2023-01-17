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

// This file implements interactive prompt and command execution.

package memtier

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type Cmd struct {
	description string
	Run         func([]string) CommandStatus
}

type Prompt struct {
	r            *bufio.Reader
	w            *bufio.Writer
	f            *flag.FlagSet
	mover        *Mover
	pages        *Pages
	aranges      *AddrRanges
	tracker      Tracker
	policy       Policy
	routines     []Routine
	routineInUse int
	cmds         map[string]Cmd
	ps1          string
	echo         bool
	quit         bool
}

type CommandStatus int

const (
	csOk CommandStatus = iota
	csUnknownCommand
	csPipeCreateError
	csPipeProcessStartError
	csError
)

func NewPrompt(ps1 string, reader *bufio.Reader, writer *bufio.Writer) *Prompt {
	p := Prompt{
		r:     reader,
		w:     writer,
		ps1:   ps1,
		mover: NewMover(),
	}
	p.cmds = map[string]Cmd{
		"q":        Cmd{"quit interactive prompt.", p.cmdQuit},
		"tracker":  Cmd{"manage tracker, track memory accesses.", p.cmdTracker},
		"stats":    Cmd{"print statistics.", p.cmdStats},
		"swap":     Cmd{"swap in/out, print swapped pages.", p.cmdSwap},
		"pages":    Cmd{"select pages, print selected page nodes and flags.", p.cmdPages},
		"arange":   Cmd{"select/split/filter address ranges.", p.cmdArange},
		"mover":    Cmd{"manage mover, move selected pages.", p.cmdMover},
		"policy":   Cmd{"manage policy, start/stop memory tiering.", p.cmdPolicy},
		"routines": Cmd{"manage routines.", p.cmdRoutines},
		"help":     Cmd{"print help.", p.cmdHelp},
		"nop":      Cmd{"no operation.", p.cmdNop},
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

func (p *Prompt) RunCmdSlice(cmdSlice []string) CommandStatus {
	if len(cmdSlice) == 0 {
		return csOk
	}
	if cmdSlice[0] == "" {
		cmdSlice[0] = "nop"
	}
	p.f = flag.NewFlagSet(cmdSlice[0], flag.ContinueOnError)
	cmd, ok := p.cmds[cmdSlice[0]]
	if !ok {
		if len(cmdSlice[0]) > 0 {
			p.output("unknown command %q\n", cmdSlice[0])
		}
		return csUnknownCommand
	}
	// Call cmd<Function>
	return cmd.Run(cmdSlice[1:])
}

func (p *Prompt) RunCmdString(cmdString string) CommandStatus {
	var err error
	// If command has "|", run the left-hand-side of the
	// pipe in a shell and pipe the output of the
	// right-hand-side cmd<Function> call to it.
	origOutputWriter := p.w
	pipeCmd := ""
	pipeIndex := strings.Index(cmdString, "|")
	if pipeIndex > -1 {
		pipeCmd = cmdString[pipeIndex+1:]
		cmdString = cmdString[:pipeIndex]
	}
	// TODO: consider shlex-like splitting.
	cmdSlice := strings.Split(strings.TrimSpace(cmdString), " ")

	// If there is a pipe, redirect p.output() (that is, p.w) to
	// the pipe before calling cmd<Function>.
	var pipeProcess *exec.Cmd = nil
	var pipeInput io.WriteCloser = nil
	if pipeCmd != "" {
		pipeProcess = exec.Command("sh", "-c", pipeCmd)
		pipeInput, err = pipeProcess.StdinPipe()
		if err != nil {
			p.output("failed to create pipe for command %q", pipeCmd)
			return csPipeCreateError
		}
		pipeProcess.Stdout = origOutputWriter
		pipeProcess.Stderr = origOutputWriter
		err := pipeProcess.Start()
		if err != nil {
			p.w = origOutputWriter
			p.output("failed to start: sh -c %q: %s", pipeCmd, err)
			pipeInput.Close()
			return csPipeProcessStartError
		}
		p.w = bufio.NewWriter(pipeInput)
	}
	runRv := p.RunCmdSlice(cmdSlice)
	// Wait for pipe process to exit and restore redirect.
	if pipeCmd != "" {
		p.w.Flush()
		pipeInput.Close()
		pipeProcess.Wait()
		p.w = origOutputWriter
	}
	return runRv
}

func (p *Prompt) Interact() {
	for !p.quit {
		p.output(p.ps1)
		cmdString, err := p.r.ReadString(byte('\n'))
		if err != nil {
			p.output("quit: %s\n", err)
			break
		}
		if p.echo {
			p.output("%s", cmdString)
		}
		p.RunCmdString(cmdString)
	}
	p.output("quit.\n")
}

func (p *Prompt) SetEcho(newEcho bool) {
	p.echo = newEcho
}

func (p *Prompt) SetPolicy(policy Policy) {
	p.policy = policy
	p.mover = p.policy.Mover()
	p.tracker = p.policy.Tracker()
}

func (p *Prompt) SetRoutines(routines []Routine) {
	p.routines = routines
}

func sortedStringKeys(m map[string]Cmd) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedNodeKeys(m map[Node]uint) []Node {
	keys := make([]int, 0, len(m))
	nodes := make([]Node, len(m))
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for i, key := range keys {
		nodes[i] = Node(key)
	}
	return nodes
}

func (p *Prompt) cmdNop(args []string) CommandStatus {
	return csOk
}

func (p *Prompt) cmdHelp(args []string) CommandStatus {
	p.output("Available commands:\n")
	for _, name := range sortedStringKeys(p.cmds) {
		p.output("        %-12s %s\n", name, p.cmds[name].description)
	}
	p.output("Syntax:\n")
	p.output("        <command> -h show help on command options.\n")
	p.output("        [command] | <shell-command>\n")
	p.output("                     pipe command output to shell-command.\n")
	return csOk
}

func (p *Prompt) cmdArange(args []string) CommandStatus {
	pid := p.f.Int("pid", -1, "look for pages of PID")
	ls := p.f.Bool("ls", false, "list address ranges")
	splitLen := p.f.Uint64("split-length", 0, "split long ranges into many ranges of max LENGTH")
	minLen := p.f.Uint64("min-length", 0, "exclude address ranges sorter than LENGTH")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	if *pid != -1 {
		process := NewProcess(*pid)
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
		p.aranges = p.aranges.Filter(func(ar AddrRange) bool {
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

func (p *Prompt) cmdSwap(args []string) CommandStatus {
	pid := p.f.Int("pid", -1, "look for pages of PID")
	swapIn := p.f.Bool("in", false, "swap in selected ranges")
	swapOut := p.f.Bool("out", false, "swap out selected ranges")
	ranges := p.f.String("ranges", "", "-ranges=RANGE[,RANGE...] select ranges. RANGE syntax: STARTADDR (single page at STARTADDR), STARTADDR-ENDADDR, STARTADDR+SIZE[kMG].")
	status := p.f.Bool("status", false, "print number of swapped out pages")
	vaddrs := p.f.Bool("vaddrs", false, "print vaddrs of swapped out pages")

	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	if *pid <= 0 {
		p.output("missing -pid=PID\n")
		return csOk
	}
	process := NewProcess(*pid)
	ar, err := process.AddressRanges()
	if err != nil {
		p.output("error reading address ranges of process %d: %v\n", *pid, err)
		return csOk
	}

	selectedRanges := []AddrRange{}
	if *ranges != "" {
		selectedRanges, err = parseOptRanges(*ranges)
		if err != nil {
			p.output("%s", err)
			return csOk
		}
	}
	if len(selectedRanges) > 0 {
		ar.Intersection(selectedRanges)
		p.output("using %d address ranges\n", len(ar.Ranges()))
	}
	if len(ar.Ranges()) == 0 {
		p.output("no address ranges from which to find pages\n")
		return csOk
	}
	if *swapIn {
		memFile, err := ProcMemOpen(ar.Pid())
		defer memFile.Close()
		if err != nil {
			p.output("%s\n", err)
			return csOk
		}
		for _, r := range ar.Ranges() {
			if err = memFile.ReadNoData(r.Addr(), r.EndAddr()); err != nil {
				p.output("%s\n", err)
				return csOk
			}
		}
	}
	if *swapOut {
		if err := ar.SwapOut(); err != nil {
			p.output("%s\n", err)
		}
	}

	if *status || *vaddrs {
		pmFile, err := ProcPagemapOpen(*pid)
		if err != nil {
			p.output("%s\n", err)
			return csOk
		}
		defer pmFile.Close()

		pages := 0
		swapped := 0
		vaddrStart := uint64(0)
		vaddrEnd := uint64(0)
		pmFile.ForEachPage(ar.Ranges(), 0,
			func(pmBits, pageAddr uint64) int {
				pages += 1
				if (pmBits>>PMB_SWAP)&1 == 0 {
					if vaddrStart > 0 && *vaddrs {
						p.output("%s\n", NewAddrRange(vaddrStart, vaddrEnd))
					}
					vaddrStart = 0
					return 0
				}
				swapped += 1
				vaddrEnd = pageAddr + constUPagesize
				if vaddrStart == 0 {
					vaddrStart = pageAddr
				}
				return 0
			})
		if vaddrStart > 0 && *vaddrs {
			p.output("%s\n", NewAddrRange(vaddrStart, vaddrEnd))
		}
		p.output("%d / %d pages, %d / %d MB (%.1f %%) swapped out\n",
			swapped, pages,
			int64(swapped)*constPagesize/(1024*1024),
			int64(pages)*constPagesize/(1024*1024),
			float32(100*swapped)/float32(pages))
	}
	return csOk
}

func (p *Prompt) cmdPages(args []string) CommandStatus {
	pid := p.f.Int("pid", -1, "look for pages of PID")
	attrs := p.f.String("attrs", "", "include only <Exclusive,Dirty,NotDirty,InHeap,InAnonymous> pages")
	ranges := p.f.String("ranges", "", "-ranges=RANGE[,RANGE...]. RANGE syntax: STARTADDR (single page at STARTADDR), STARTADDR-ENDADDR, STARTADDR+SIZE[kMG].")
	fromNode := p.f.Int("node", -1, "include only pages currently on NODE")
	pr := p.f.Int("pr", -1, "-pr=NUM: print first NUM address ranges")
	pm := p.f.Int("pm", -1, "-pm=NUM: print pagemap bits and PFNs of first NUM pages in selected ranges")
	pk := p.f.Int("pk", -1, "-pk=PFN: print kpageflags of a page")
	pi := p.f.Int("pi", -1, "-pi=PFN: print idle bit from /sys/kernel/mm/page_idle/bitmap")
	si := p.f.Int("si", -1, "-si=PFN: set idle bit of a page")

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
	if *pi > -1 || *si > -1 {
		bmFile, err := ProcPageIdleBitmapOpen()
		if err != nil {
			p.output("failed to open idle bitmap: %s\n", err)
			return csOk
		}
		defer bmFile.Close()
		if *pi > -1 {
			isIdle, err := bmFile.GetIdle(uint64(*pi))
			if err != nil {
				p.output("failed to read idle bit: %s\n", err)
				return csOk
			}
			p.output("PFN %d idle: %v\n", *pi, isIdle)
		}
		if *si > -1 {
			err := bmFile.SetIdle(uint64(*si))
			if err != nil {
				p.output("failed to set idle bit: %s\n", err)
				return csOk
			}
		}
		return csOk
	}

	if *pk > -1 {
		kpFile, err := ProcKpageflagsOpen()
		if err != nil {
			p.output("opening kpageflags failed: %s\n", err)
			return csOk
		}
		flags, err := kpFile.ReadFlags(uint64(*pk))
		if err != nil {
			p.output("reading flags of PFN %d from kpageflags failed: %s\n", *pk, err)
			return csOk
		}
		p.output(`LOCKED        %d
ERROR         %d
REFERENCED    %d
UPTODATE      %d
DIRTY         %d
LRU           %d
ACTIVE        %d
SLAB          %d
WRITEBACK     %d
RECLAIM       %d
BUDDY         %d
MMAP          %d
ANON          %d
SWAPCACHE     %d
SWAPBACKED    %d
COMPOUND_HEAD %d
COMPOUND_TAIL %d
HUGE          %d
UNEVICTABLE   %d
HWPOISON      %d
NOPAGE        %d
KSM           %d
THP           %d
OFFLINE       %d
ZERO_PAGE     %d
IDLE          %d
PGTABLE       %d
`,
			(flags>>KPFB_LOCKED)&1,
			(flags>>KPFB_ERROR)&1,
			(flags>>KPFB_REFERENCED)&1,
			(flags>>KPFB_UPTODATE)&1,
			(flags>>KPFB_DIRTY)&1,
			(flags>>KPFB_LRU)&1,
			(flags>>KPFB_ACTIVE)&1,
			(flags>>KPFB_SLAB)&1,
			(flags>>KPFB_WRITEBACK)&1,
			(flags>>KPFB_RECLAIM)&1,
			(flags>>KPFB_BUDDY)&1,
			(flags>>KPFB_MMAP)&1,
			(flags>>KPFB_ANON)&1,
			(flags>>KPFB_SWAPCACHE)&1,
			(flags>>KPFB_SWAPBACKED)&1,
			(flags>>KPFB_COMPOUND_HEAD)&1,
			(flags>>KPFB_COMPOUND_TAIL)&1,
			(flags>>KPFB_HUGE)&1,
			(flags>>KPFB_UNEVICTABLE)&1,
			(flags>>KPFB_HWPOISON)&1,
			(flags>>KPFB_NOPAGE)&1,
			(flags>>KPFB_KSM)&1,
			(flags>>KPFB_THP)&1,
			(flags>>KPFB_OFFLINE)&1,
			(flags>>KPFB_ZERO_PAGE)&1,
			(flags>>KPFB_IDLE)&1,
			(flags>>KPFB_PGTABLE)&1)

		return csOk
	}
	// there are arguments, select new set of pages
	if *pid <= 0 {
		p.output("missing -pid=PID\n")
		return csOk
	}
	p.output("searching for address ranges of pid %d\n", *pid)
	process := NewProcess(*pid)
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
	selectedRanges := []AddrRange{}
	if *ranges != "" {
		selectedRanges, err = parseOptRanges(*ranges)
		if err != nil {
			p.output("%s", err)
			return csOk
		}
	}
	if len(selectedRanges) > 0 {
		ar.Intersection(selectedRanges)
		p.output("using %d address ranges\n", len(ar.Ranges()))
	}
	if len(ar.Ranges()) == 0 {
		p.output("no address ranges from which to find pages\n")
		return csOk
	}
	if *pm != -1 {
		pmFile, err := ProcPagemapOpen(*pid)
		if err != nil {
			p.output("%s", err)
			return csOk
		}
		defer pmFile.Close()
		printed := 0
		p.output("bits: softdirty, exclusive, uffd_wp, file, swap, present\n")
		pmFile.ForEachPage(ar.Ranges(), 0,
			func(pmBits, pageAddr uint64) int {
				p.output("%x: bits: %d%d%d%d%d%d pfn: %d\n",
					pageAddr,
					(pmBits>>PMB_SOFT_DIRTY)&1,
					(pmBits>>PMB_MMAP_EXCLUSIVE)&1,
					(pmBits>>PMB_UFFD_WP)&1,
					(pmBits>>PMB_FILE)&1,
					(pmBits>>PMB_SWAP)&1,
					(pmBits>>PMB_PRESENT)&1,
					pmBits&PM_PFN)
				printed += 1
				if printed >= *pm {
					return -1
				}
				return 0
			})
		return csOk
	}
	p.aranges = ar
	p.output("aranges = <%d address ranges of pid %d>\n", len(ar.Ranges()), ar.Pid())
	if *pr > -1 {
		for n, r := range p.aranges.Ranges() {
			if n >= *pr {
				break
			}
			p.output("%s\n", r)
		}
		return csOk
	}
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
		pp = pp.OnNode(Node(*fromNode))
	}
	p.pages = pp
	return csOk
}

func (p *Prompt) cmdMover(args []string) CommandStatus {
	config := p.f.String("config", "", "reconfigure mover with JSON string")
	configDump := p.f.Bool("config-dump", false, "dump current configuration")
	pagesTo := p.f.Int("pages-to", -1, "move pages to NODE int")
	start := p.f.Bool("start", false, "start mover")
	stop := p.f.Bool("stop", false, "stop mover")
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
	if *configDump {
		p.output("%s\n", p.mover.GetConfigJson())
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
		toNode := Node(*pagesTo)
		task := NewMoverTask(p.pages, toNode)
		p.mover.AddTask(task)
	}
	if *stop {
		p.mover.Stop()
	}
	if *start {
		p.mover.Start()
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

func (p *Prompt) cmdStats(args []string) CommandStatus {
	lm := p.f.Int("lm", -1, "show latest move in PID")
	le := p.f.Int("le", -1, "show latest move with error in PID")
	dump := p.f.Bool("dump", false, "dump stats internals")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	remainder := p.f.Args()
	if *lm != -1 {
		p.output("%s\n",
			GetStats().LastMove(*lm))
		return csOk
	}
	if *le != -1 {
		p.output("%s\n",
			GetStats().LastMoveWithError(*le))
		return csOk
	}
	if *dump {
		p.output("%s\n",
			GetStats().Dump(remainder))
		return csOk
	}
	p.output(GetStats().Summarize() + "\n")
	return csOk
}

func (p *Prompt) cmdTracker(args []string) CommandStatus {
	ls := p.f.Bool("ls", false, "list available memory trackers")
	create := p.f.String("create", "", "create new tracker NAME")
	config := p.f.String("config", "", "configure tracker with JSON string")
	configDump := p.f.Bool("config-dump", false, "dump current configuration")
	start := p.f.String("start", "", "start tracking PID[,PID...]")
	reset := p.f.Bool("reset", false, "reset page access counters")
	stop := p.f.Bool("stop", false, "stop tracker")
	counters := p.f.Bool("counters", false, "print tracker raw counters")
	heat := p.f.Bool("heat", false, "print address range heats")
	dump := p.f.Bool("dump", false, "dump tracker internals")

	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	remainder := p.f.Args()
	if *ls {
		p.output(strings.Join(TrackerList(), "\n") + "\n")
		return csOk
	}
	if *create != "" {
		if tracker, err := NewTracker(*create); err != nil {
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
	if *configDump {
		p.output("%s\n", p.tracker.GetConfigJson())
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
	if *heat {
		tcs := p.tracker.GetCounters()
		for _, rh := range tcs.RangeHeat() {
			p.output("%s %d\n", rh.Range, rh.Heat)
		}
	}
	if *reset {
		p.tracker.ResetCounters()
		p.output("tracker counters reset\n")
	}
	if *dump {
		p.output("%s\n", p.tracker.Dump(remainder))
		p.output("\n")
	}
	return csOk
}

func (p *Prompt) cmdPolicy(args []string) CommandStatus {
	ls := p.f.Bool("ls", false, "list available policies")
	create := p.f.String("create", "", "create new policy NAME")
	config := p.f.String("config", "", "reconfigure policy with JSON string")
	configDump := p.f.Bool("config-dump", false, "dump current config")
	configFile := p.f.String("config-file", "", "reconfigure policy with JSON FILE")
	dump := p.f.Bool("dump", false, "dump policy state")
	start := p.f.Bool("start", false, "start policy")
	stop := p.f.Bool("stop", false, "stop policy")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	remainder := p.f.Args()
	if *ls {
		p.output(strings.Join(PolicyList(), "\n") + "\n")
		return csOk
	}
	if *create != "" {
		policy, err := NewPolicy(*create)
		if err != nil {
			p.output("%s", err)
			return csOk
		}
		p.policy = policy
		p.output("policy created\n")
	}
	if p.policy == nil {
		p.output("no policy, create one with -create NAME\n")
		return csOk
	}
	if *configFile != "" {
		configJson, err := ioutil.ReadFile(*configFile)
		if err != nil {
			p.output("reading file %q failed: %s", *configFile, err)
			return csOk
		}
		err = p.policy.SetConfigJson(string(configJson))
		if err != nil {
			p.output("config failed: %s\n", err)
			return csOk
		}
	}
	if *config != "" {
		err := p.policy.SetConfigJson(*config)
		if err != nil {
			p.output("config failed: %s\n", err)
			return csOk
		}
	}
	p.SetPolicy(p.policy)
	if *start {
		err := p.policy.Start()
		if err != nil {
			p.output("start failed: %s\n", err)
			return csOk
		}
	}
	if *configDump {
		p.output("%s\n", p.policy.GetConfigJson())
	}
	if *stop {
		p.policy.Stop()
		p.output("policy stopped\n")
	}
	if *dump {
		p.output("%s\n", p.policy.Dump(remainder))
		p.output("\n")
	}
	return csOk
}

func (p *Prompt) cmdRoutines(args []string) CommandStatus {
	ls := p.f.Bool("ls", false, "list available routine types")
	create := p.f.String("create", "", "create new routine from type NAME")
	config := p.f.String("config", "", "reconfigure the routine with JSON string")
	configDump := p.f.Bool("config-dump", false, "dump current config")
	configFile := p.f.String("config-file", "", "reconfigure the routine with JSON FILE")
	dump := p.f.Bool("dump", false, "dump routine state")
	start := p.f.Bool("start", false, "start routine")
	stop := p.f.Bool("stop", false, "stop routine")
	use := p.f.Int("use", -1, "pass commands to routine at given index (0, 1, ...)")
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	remainder := p.f.Args()
	if *ls {
		p.output(strings.Join(RoutineList(), "\n") + "\n")
		return csOk
	}
	if *create != "" {
		routine, err := NewRoutine(*create)
		if err != nil {
			p.output("%s\n", err)
			return csOk
		}
		p.routines = append(p.routines, routine)
		p.routineInUse = len(p.routines) - 1
		p.output("routine %d created, started using it\n", p.routineInUse)
	}
	if len(p.routines) == 0 {
		p.output("no routines, create one with -create NAME\n")
		return csOk
	}
	if *use > -1 {
		if *use >= len(p.routines) {
			p.output("invalid routine %d, the last one is %d\n", *use, len(p.routines)-1)
			return csOk
		}
		p.routineInUse = *use
	}
	if *configFile != "" {
		configJson, err := ioutil.ReadFile(*configFile)
		if err != nil {
			p.output("reading file %q failed: %s\n", *configFile, err)
			return csOk
		}
		err = p.routines[p.routineInUse].SetConfigJson(string(configJson))
		if err != nil {
			p.output("config failed: %s\n", err)
			return csOk
		}
	}
	if *config != "" {
		err := p.routines[p.routineInUse].SetConfigJson(*config)
		if err != nil {
			p.output("config failed: %s\n", err)
			return csOk
		}
	}
	if *start {
		err := p.routines[p.routineInUse].Start()
		if err != nil {
			p.output("start failed: %s\n", err)
			return csOk
		}
	}
	if *configDump {
		p.output("%s\n", p.routines[p.routineInUse].GetConfigJson())
	}
	if *stop {
		p.routines[p.routineInUse].Stop()
		p.output("routine %d stopped\n", p.routineInUse)
	}
	if *dump {
		p.output("%s\n", p.routines[p.routineInUse].Dump(remainder))
		p.output("\n")
	}
	return csOk
}

func (p *Prompt) cmdQuit(args []string) CommandStatus {
	if err := p.f.Parse(args); err != nil {
		return csOk
	}
	p.quit = true
	return csOk
}

func parseOptPages(pagesStr string) (uint64, error) {
	if pagesStr == "" {
		return (PMPresentSet |
			PMExclusiveSet), nil
	}
	var pageAttributes uint64 = 0
	for _, pageAttrStr := range strings.Split(pagesStr, ",") {
		switch pageAttrStr {
		case "Present":
			pageAttributes |= PMPresentSet
		case "Exclusive":
			pageAttributes |= PMExclusiveSet
		case "Dirty":
			pageAttributes |= PMDirtySet
		case "NotDirty":
			pageAttributes |= PMDirtyCleared
		default:
			return 0, fmt.Errorf("invalid -page: %q", pageAttrStr)
		}
		if pageAttributes&PMDirtySet == PMDirtySet &&
			pageAttributes&PMDirtyCleared == PMDirtyCleared {
			return 0, fmt.Errorf("contradicting page requirements: Dirty,NotDirty")
		}
	}
	return pageAttributes, nil
}

func parseOptRanges(rangeStr string) ([]AddrRange, error) {
	addrRanges := []AddrRange{}
	for _, addrRangeStr := range strings.Split(rangeStr, ",") {
		ar, err := NewAddrRangeFromString(addrRangeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid addresss range %q, expected STARTADDR, STARTADDR-STOPADDR or STARTADDR+SIZE[UNIT]", addrRangeStr)
		}
		addrRanges = append(addrRanges, *ar)
	}
	return addrRanges, nil
}
