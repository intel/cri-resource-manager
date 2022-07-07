package memtier

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type Stats struct {
	sync.RWMutex
	namePulse   mapStringPStatsPulse
	pidMadvices mapIntPStatsPidMadviced
	pidMoves    mapIntPStatsPidMoved
	pidScans    mapIntPStatsPidScanned
	recMoves    *[]*StatsMoved
}

var recMovesBufSize int = 1024 * 256

type StatsPidMadviced struct {
	sumSyscalls     uint64
	sumPageCount    uint64
	advicePageCount map[int]uint64
	errnoPageCount  map[int]uint64
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
	sumTimeUs    uint64
	sumScanned   uint64
	sumAccessed  uint64
	sumWritten   uint64
	lastTimeUs   uint64
	lastScanned  uint64
	lastAccessed uint64
	lastWritten  uint64
	maxTimeUs    uint64
	count        uint64
}

type StatsPulse struct {
	sumBeats   uint64
	firstBeat  int64
	latestBeat int64
}

type StatsHeartbeat struct {
	name string
}

type StatsMadviced struct {
	pid       int
	sysRet    int
	errno     int
	advice    int
	pageCount uint64
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
		namePulse:   make(mapStringPStatsPulse),
		pidMadvices: make(mapIntPStatsPidMadviced),
		pidMoves:    make(mapIntPStatsPidMoved),
		pidScans:    make(mapIntPStatsPidScanned),
	}
}

func newStatsPulse() *StatsPulse {
	return &StatsPulse{}
}

func newStatsPidMadviced() *StatsPidMadviced {
	return &StatsPidMadviced{
		advicePageCount: make(map[int]uint64),
		errnoPageCount:  make(map[int]uint64),
	}
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
	s.Lock()
	defer s.Unlock()
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
	case StatsMadviced:
		spm, ok := s.pidMadvices[v.pid]
		if !ok {
			spm = newStatsPidMadviced()
			s.pidMadvices[v.pid] = spm
		}
		spm.sumSyscalls += 1
		spm.sumPageCount += v.pageCount
		spm.advicePageCount[v.advice] += v.pageCount
		spm.errnoPageCount[v.errno] += v.pageCount
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
		if s.recMoves != nil {
			*s.recMoves = append(*s.recMoves, &v)
		}
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
		sps.lastTimeUs = uint64(v.timeUs)
		sps.lastScanned = v.scanned
		sps.lastAccessed = v.accessed
		sps.lastWritten = v.written
		if sps.lastTimeUs > sps.maxTimeUs {
			sps.maxTimeUs = sps.lastTimeUs
		}
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

func (s *Stats) Dump(args []string) string {
	s.Lock()
	defer s.Unlock()
	if len(args) == 0 ||
		args[0] != "moves" ||
		args[0] == "moves" && len(args) != 2 {
		return "Usage: dump moves <start|new|stop>"
	}
	switch args[1] {
	case "start":
		recMoves := make([]*StatsMoved, 0, recMovesBufSize)
		s.recMoves = &recMoves
		return "recording moves started"
	case "stop":
		s.recMoves = nil
		return "recording moves stopped"
	case "new":
		if s.recMoves == nil {
			return "error: not recording"
		}
		movesStrings := make([]string, 0, len(*s.recMoves))
		for _, sm := range *s.recMoves {
			movesStrings = append(movesStrings, sm.String())
		}
		movesString := strings.Join(movesStrings, "\n")
		*s.recMoves = (*s.recMoves)[:0]
		return movesString
	default:
		return fmt.Sprintf("invalid dump moves parameter: %q", args[1])
	}
}

func (s *Stats) Summarize() string {
	lines := []string{}
	lines = append(lines, "", "table: events")
	lines = append(lines, "   count timeint[s] latest[s ago] name")
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
			fmt.Sprintf("%8d %10.3f %13.3f %s",
				pulse.sumBeats,
				(secondsSinceFirst-secondsSinceLatest)/float32(beatsMinusOne),
				secondsSinceLatest,
				name))
	}
	lines = append(lines, "", "table: process_madvice syscalls")
	lines = append(lines, "     pid    calls req[pages]  ok[pages]    ok[G] advice:mem[G]")
	for _, pid := range s.pidMadvices.sortedKeys() {
		spm := s.pidMadvices[pid]
		advMem := fmt.Sprintf("PAGEOUT:%.3f;COLD:%.3f",
			float64(spm.advicePageCount[unix.MADV_PAGEOUT]*constUPagesize)/float64(1024*1024*1024),
			float64(spm.advicePageCount[unix.MADV_COLD]*constUPagesize)/float64(1024*1024*1024))
		lines = append(lines, fmt.Sprintf("%8d %8d %10d %10d %8.3f %s",
			pid,
			spm.sumSyscalls,
			spm.sumPageCount,
			spm.errnoPageCount[0],
			float64(spm.errnoPageCount[0]*constUPagesize)/float64(1024*1024*1024),
			advMem))
	}
	lines = append(lines, "", "table: move_pages syscalls")
	lines = append(lines, "     pid    calls req[pages]  ok[pages] moved[G] targetnode:moved[G]")
	for _, pid := range s.pidMoves.sortedKeys() {
		spm := s.pidMoves[pid]
		node_moved_list := []string{}
		for _, node := range spm.sumDestNodePages.sortedKeys() {
			node_moved_list = append(node_moved_list, fmt.Sprintf("%d:%.3f",
				node,
				float64(spm.sumDestNodePages[node]*constUPagesize)/float64(1024*1024*1024)))
		}
		node_moved := strings.Join(node_moved_list, ";")
		lines = append(lines, fmt.Sprintf("%8d %8d %10d %10d %8.3f %s",
			pid,
			spm.sumSyscalls,
			spm.sumReqs,
			spm.sumDestNode,
			float64(spm.sumDestNode*constUPagesize)/float64(1024*1024*1024),
			node_moved))
	}
	lines = append(lines, "", "table: move_pages syscall errors in page statuses")
	lines = append(lines, "     pid    pages  size[G]    errno error")
	for pid, spm := range s.pidMoves {
		for _, errno := range spm.sumErrorCounts.sortedKeys() {
			lines = append(lines, fmt.Sprintf("%8d %8d %8.3f %8d %s",
				pid,
				spm.sumErrorCounts[errno],
				float64(spm.sumErrorCounts[errno]*constUPagesize)/float64(1024*1024*1024),
				errno,
				syscall.Errno(errno)))
		}
	}
	lines = append(lines, "", "table: memory scans")
	lines = append(lines, "     pid    scans   tot[pages] avg[s]   last max[s]   avg[G]  last[G] a+w[%%] last")
	for _, pid := range s.pidScans.sortedKeys() {
		sps := s.pidScans[pid]
		lines = append(lines, fmt.Sprintf("%8d %8d %12d %6.3f %6.3f %6.3f %8.3f %8.3f %5.2f %5.2f",
			pid,
			sps.count,
			sps.sumScanned,
			float64(sps.sumTimeUs)/float64(1000*1000*sps.count),
			float64(sps.lastTimeUs)/float64(1000*1000),
			float64(sps.maxTimeUs)/float64(1000*1000),
			float64(sps.sumScanned*constUPagesize)/float64(sps.count*1024*1024*1024),
			float64(sps.lastScanned*constUPagesize)/float64(1024*1024*1024),
			float64(100*(sps.sumAccessed+sps.sumWritten))/float64(sps.sumScanned),
			float64(100*(sps.lastAccessed+sps.lastWritten))/float64(sps.lastScanned),
		))
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
