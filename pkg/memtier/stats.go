package memtier

import (
	"fmt"
	"strings"
	"syscall"
	"time"
)

type Stats struct {
	namePulse mapStringPStatsPulse
	pidMoves  map[int]*StatsPidMoved
	pidScans  map[int]*StatsPidScanned
}

type StatsPidMoved struct {
	sumSyscalls       uint64
	sumReqs           uint64
	sumDestNode       uint64
	sumOtherNode      uint64
	sumDestNodePages  mapIntUint64
	sumErrorCounts    mapIntUint64
	lastMoveWithError StatsMoved
	lastMove          StatsMoved
}

type StatsPidScanned struct {
	sumTimeUs   uint64
	sumScanned  uint64
	sumAccessed uint64
	sumWritten  uint64
	count       uint64
}

type StatsPulse struct {
	sumBeats   uint64
	firstBeat  int64
	latestBeat int64
}

type StatsHeartbeat struct {
	name string
}

type StatsMoved struct {
	pid            int
	sysRet         uint
	firstPageAddr  uintptr
	destNode       int
	reqCount       int
	destNodeCount  int
	otherNodeCount int
	errorCounts    map[int]int
}

type StatsPageScan struct {
	pid      int
	timeUs   int64
	scanned  uint64
	accessed uint64
	written  uint64
}

var stats *Stats = newStats()

func newStats() *Stats {
	return &Stats{
		namePulse: make(mapStringPStatsPulse),
		pidMoves:  make(map[int]*StatsPidMoved),
		pidScans:  make(map[int]*StatsPidScanned),
	}
}

func newStatsPulse() *StatsPulse {
	return &StatsPulse{}
}

func newStatsPidMoved() *StatsPidMoved {
	return &StatsPidMoved{
		sumDestNodePages: make(mapIntUint64),
		sumErrorCounts:   make(map[int]uint64),
	}
}

func newStatsPidScanned() *StatsPidScanned {
	return &StatsPidScanned{}
}

func GetStats() *Stats {
	return stats
}

func (s *Stats) Store(entry interface{}) {
	switch v := entry.(type) {
	case StatsHeartbeat:
		pulse, ok := s.namePulse[v.name]
		if !ok {
			pulse = newStatsPulse()
			pulse.firstBeat = time.Now().UnixNano()
			s.namePulse[v.name] = pulse
		}
		pulse.sumBeats += 1
		pulse.latestBeat = time.Now().UnixNano()
	case StatsMoved:
		// keep separate statistics for every pid
		spm, ok := s.pidMoves[v.pid]
		if !ok {
			spm = newStatsPidMoved()
			s.pidMoves[v.pid] = spm
		}
		spm.sumSyscalls += 1
		spm.sumReqs += uint64(v.reqCount)
		spm.sumDestNode += uint64(v.destNodeCount)
		spm.sumOtherNode += uint64(v.otherNodeCount)
		spm.sumDestNodePages[v.destNode] += uint64(v.destNodeCount)
		for e, cnt := range v.errorCounts {
			spm.sumErrorCounts[e] += uint64(cnt)
		}
		if len(v.errorCounts) > 0 {
			spm.lastMoveWithError = v
		}
		spm.lastMove = v
	case StatsPageScan:
		sps, ok := s.pidScans[v.pid]
		if !ok {
			sps = newStatsPidScanned()
			s.pidScans[v.pid] = sps
		}
		sps.count += 1
		sps.sumTimeUs += uint64(v.timeUs)
		sps.sumScanned += v.scanned
		sps.sumAccessed += v.accessed
		sps.sumWritten += v.written
	}
}

func (s *Stats) LastMove(pid int) *StatsMoved {
	spm, ok := s.pidMoves[pid]
	if !ok {
		return nil
	}
	return &spm.lastMove
}

func (s *Stats) LastMoveWithError(pid int) *StatsMoved {
	spm, ok := s.pidMoves[pid]
	if !ok {
		return nil
	}
	return &spm.lastMoveWithError
}

func (s *Stats) Summarize() string {
	lines := []string{}
	lines = append(lines, "heartbeats timeint[s] latest[s ago] name")
	now := time.Now().UnixNano()
	for _, name := range s.namePulse.sortedKeys() {
		pulse := s.namePulse[name]
		secondsSinceFirst := float32(now-pulse.firstBeat) / float32(time.Second)
		secondsSinceLatest := float32(now-pulse.latestBeat) / float32(time.Second)
		beatsMinusOne := pulse.sumBeats - 1
		if beatsMinusOne == 0 {
			beatsMinusOne = 1
		}
		lines = append(lines,
			fmt.Sprintf("%10d %10.3f %13.3f %s",
				pulse.sumBeats,
				(secondsSinceFirst-secondsSinceLatest)/float32(beatsMinusOne),
				secondsSinceLatest,
				name))
	}
	for pid, spm := range s.pidMoves {
		lines = append(lines,
			fmt.Sprintf("move_pages on pid: %d", pid),
			fmt.Sprintf("    calls: %d", spm.sumSyscalls),
			fmt.Sprintf("    requested: %d pages (%d MB)", spm.sumReqs, spm.sumReqs*constUPagesize/(1024*1024)),
			fmt.Sprintf("    on target: %d pages (%d MB)", spm.sumDestNode, spm.sumDestNode*constUPagesize/(1024*1024)))
		for _, node := range spm.sumDestNodePages.sortedKeys() {
			lines = append(lines,
				fmt.Sprintf("        to node %d: %d pages (%d MB)",
					node,
					spm.sumDestNodePages[node],
					spm.sumDestNodePages[node]*constUPagesize/(1024*1024)))
		}
		errorPages := uint64(0)
		errorSumIndex := len(lines)
		lines = append(lines, "") // placeholder for total error count
		for _, errno := range spm.sumErrorCounts.sortedKeys() {
			errorPages += spm.sumErrorCounts[errno]
			lines = append(lines,
				fmt.Sprintf("        %d (%s): %d pages (%d MB)",
					errno, syscall.Errno(errno), spm.sumErrorCounts[errno], spm.sumErrorCounts[errno]*constUPagesize/(1024*1024)))
		}
		lines[errorSumIndex] = fmt.Sprintf("    errors: %d pages (%d MB)", errorPages, errorPages*constUPagesize/(1024*1024))
	}

	for pid, sps := range s.pidScans {
		lines = append(lines,
			fmt.Sprintf("memory scans on pid: %d", pid),
			fmt.Sprintf("    scans: %d", sps.count),
			fmt.Sprintf("    scan time: %d ms (%d ms/scan)",
				sps.sumTimeUs/1000,
				sps.sumTimeUs/1000/sps.count),
			fmt.Sprintf("    scanned: %d pages (%d pages/scan, %d MB/scan)", sps.sumScanned, sps.sumScanned/sps.count, sps.sumScanned*constUPagesize/sps.count/1024/1024),
			fmt.Sprintf("    accessed: %d pages (%d pages/scan, %d MB/scan)", sps.sumAccessed, sps.sumAccessed/sps.count, sps.sumAccessed*constUPagesize/sps.count/1024/1024),
			fmt.Sprintf("    written: %d pages (%d pages/scan, %d MB/scan)", sps.sumWritten, sps.sumWritten/sps.count, sps.sumWritten*constUPagesize/sps.count/1024/1024))
	}
	return strings.Join(lines, "\n")
}

func (s *Stats) String() string {
	return fmt.Sprintf("%v", s.pidMoves)
}

func (sm *StatsMoved) String() string {
	return fmt.Sprintf("move_pages(pid=%d, pagecount=%d, firstpage=%x dest=%d) => (return=%d on_dest=%d on_other=%d [errno:pagecount]=%v)",
		// inputs
		sm.pid, sm.reqCount, sm.firstPageAddr, sm.destNode,
		// results
		sm.sysRet, sm.destNodeCount, sm.otherNodeCount, sm.errorCounts)
}
