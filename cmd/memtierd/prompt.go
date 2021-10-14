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
	"strings"

	"github.com/intel/cri-resource-manager/pkg/memtier"
)

type Prompt struct {
	r     *bufio.Reader
	w     *bufio.Writer
	f     *flag.FlagSet
	mover *memtier.Mover
	pages *memtier.Pages
	ps1   string
}

type promptAction int

const (
	paCommandOk promptAction = iota
	paQuit
)

func NewPrompt(ps1 string, reader *bufio.Reader, writer *bufio.Writer) *Prompt {
	return &Prompt{
		r:     reader,
		w:     writer,
		ps1:   ps1,
		mover: memtier.NewMover(),
	}
}

func (p *Prompt) output(format string, a ...interface{}) {
	if p.w == nil {
		return
	}
	p.w.WriteString(fmt.Sprintf(format, a...))
	p.w.Flush()
}

func (p *Prompt) interact() {
	pa := paCommandOk
	for pa != paQuit {
		p.output(p.ps1)
		cmd, err := p.r.ReadString(byte('\n'))
		if err != nil {
			p.output("quitting prompt: %s\n", err)
			break
		}
		cmdSlice := strings.Split(strings.TrimSpace(cmd), " ")
		if len(cmdSlice) == 0 {
			continue
		}
		p.f = flag.NewFlagSet(cmdSlice[0], flag.ContinueOnError)
		switch cmdSlice[0] {
		case "q", "quit":
			pa = p.cmdQuit(cmdSlice[1:])
		case "stats":
			pa = p.cmdStats(cmdSlice[1:])
		case "mover":
			pa = p.cmdMover(cmdSlice[1:])
		case "pages":
			pa = p.cmdPages(cmdSlice[1:])
		case "":
			pa = paCommandOk
		default:
			p.output("unknown command\n")
			pa = paCommandOk
		}
	}
	p.output("quitting prompt.\n")
}

func (p *Prompt) cmdPages(args []string) promptAction {
	pid := p.f.Int("pid", -1, "look for pages of PID")
	attrs := p.f.String("attrs", "", "include only <Exclusive,Dirty,NotDirty,InHeap,InAnonymous> pages")
	ranges := p.f.String("ranges", "", "-ranges=START-STOP[,START-STOP...] include only given ranges")
	fromNode := p.f.Int("node", -1, "include only pages currently on NODE")
	if err := p.f.Parse(args); err != nil {
		return paCommandOk
	}
	if *pid <= 0 {
		p.output("missing valid -pid=PID\n")
		return paCommandOk
	}
	process := memtier.NewProcess(*pid)
	ar, err := process.AddressRanges()
	if err != nil {
		p.output("error reading address ranges of process %d: %v\n", *pid, err)
		return paCommandOk
	}
	if ar == nil {
		p.output("address ranges not found for process %d\n", *pid)
		return paCommandOk
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
		return paCommandOk
	}
	pageAttributes, err := parseOptPages(*attrs)
	if err != nil {
		p.output("invalid -attrs: %v\n", err)
		return paCommandOk
	}
	pp, err := ar.PagesMatching(pageAttributes)
	if err != nil {
		p.output("finding pages from address ranges failed: %v\n", err)
	}
	p.output("found %d pages matching the attributes\n", len(pp.Pages()))
	if *fromNode >= 0 {
		pp = pp.OnNode(memtier.Node(*fromNode))
	}
	p.output("current pages:\n")
	for node, pageCount := range pp.NodePageCount() {
		p.output("   node %d: %d\n", node, pageCount)
	}
	p.pages = pp
	return paCommandOk
}

func (p *Prompt) cmdMover(args []string) promptAction {
	config := p.f.String("config", "", "reconfigure mover with JSON string")
	pagesTo := p.f.Int("pages-to", -1, "move pages to NODE int")
	pause := p.f.Bool("pause", false, "pause moving")
	cont := p.f.Bool("continue", false, "continue moving")
	tasks := p.f.Bool("tasks", false, "print current tasks")
	if err := p.f.Parse(args); err != nil {
		return paCommandOk
	}
	if *config != "" {
		if err := p.mover.SetConfigJson(*config); err != nil {
			p.output("mover reconfiguration error: %v\n", err)
		}
	}
	if *pagesTo >= 0 {
		p.mover.Start()
		if p.pages == nil {
			p.output("mover error: set pages before moving\n")
			return paCommandOk
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
		p.output("IMPLEMENT ME\n")
	}
	return paCommandOk
}

func (p *Prompt) cmdStats(args []string) promptAction {
	moves := p.f.Bool("m", false, "dump all moves")
	if err := p.f.Parse(args); err != nil {
		return paCommandOk
	}
	if *moves {
		p.output(memtier.Stats().Dump() + "\n")
	} else {
		p.output(memtier.Stats().Summarize() + "\n")
	}
	return paCommandOk
}

func (p *Prompt) cmdQuit(args []string) promptAction {
	help := p.f.Bool("h", false, "print help")
	p.f.Parse(args)
	if *help {
		p.output("quit interactive prompt\n")
		return paCommandOk
	}
	return paQuit
}
