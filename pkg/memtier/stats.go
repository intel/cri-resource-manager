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
	pidMadvises mapIntPStatsPidMadvised
	pidMoves    mapIntPStatsPidMoved
	pidScans    mapIntPStatsPidScanned
	recMoves    *[]*StatsMoved
}

var recMovesBufSize int = 1024 * 256

type StatsPidMadvised struct {
	sumSyscalls     uint64
	sumPageCount    uint64
	advisePageCount map[int]uint64
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

type StatsMadvised struct {
	pid       int
	sysRet    int
	errno     int
	advise    int
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
		pidMadvises: make(mapIntPStatsPidMadvised),
		pidMoves:    make(mapIntPStatsPidMoved),
		pidScans:    make(mapIntPStatsPidScanned),
	}
}

func newStatsPulse() *StatsPulse {
	return &StatsPulse{}
}

func newStatsPidMadvised() *StatsPidMadvised {
	return &StatsPidMadvised{
		advisePageCount: make(map[int]uint64),
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

// MadvisedPageCount returns the number of pages on which
// process_madvise(pid, advise) has been called. If pid==-1 or
// advise==-1, then return the sum of pages of all pids and advises.
func (s *Stats) MadvisedPageCount(pid int, advise int) uint64 {
	totalPages := uint64(0)
	for spid, spm := range s.pidMadvises {
		if pid != -1 && pid != spid {
			continue
		}
		for adv, pageCount := range spm.advisePageCount {
			if advise != -1 && adv != advise {
				continue
			}
			totalPages += pageCount
		}
	}
	return totalPages
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
	case StatsMadvised:
		spm, ok := s.pidMadvises[v.pid]
		if !ok {
			spm = newStatsPidMadvised()
			s.pidMadvises[v.pid] = spm
		}
		spm.sumSyscalls += 1
		spm.sumPageCount += v.pageCount
		spm.advisePageCount[v.advise] += v.pageCount
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

func (s *Stats) tableEvents(format string) []string {
	lines := []string{"table: events"}
	header := []interface{}{"count", "timeint[s]", "latest[s ago]", "name"}
	headerFmt := map[string]string{
		"csv": "%s,%s,%s,%s",
		"txt": "%8s %10s %13s %s",
	}
	rowFmt := map[string]string{
		"csv": "%d,%.6f,%.6f,%s",
		"txt": "%8d %10.3f %13.3f %s",
	}
	lines = append(lines, fmt.Sprintf(headerFmt[format], header...))
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
			fmt.Sprintf(rowFmt[format],
				pulse.sumBeats,
				(secondsSinceFirst-secondsSinceLatest)/float32(beatsMinusOne),
				secondsSinceLatest,
				name))
	}
	return lines
}

func (s *Stats) tableProcessMadvise(format string) []string {
	lines := []string{"table: process_madvise syscalls"}
	headers := []interface{}{"pid", "calls", "req[pages]", "ok[pages]", "ok[G]", "PAGEOUT[G]"}
	headerFmt := map[string]string{
		"csv": "%s,%s,%s,%s,%s,%s",
		"txt": "%8s %8s %10s %10s %8s %s",
	}
	rowFmt := map[string]string{
		"csv": "%d,%d,%d,%d,%8.6f,%.6f",
		"txt": "%8d %8d %10d %10d %8.3f %.3f",
	}
	lines = append(lines, fmt.Sprintf(headerFmt[format], headers...))
	for _, pid := range s.pidMadvises.sortedKeys() {
		spm := s.pidMadvises[pid]
		lines = append(lines, fmt.Sprintf(rowFmt[format],
			pid,
			spm.sumSyscalls,
			spm.sumPageCount,
			spm.errnoPageCount[0],
			float64(spm.errnoPageCount[0]*constUPagesize)/float64(1024*1024*1024),
			float64(spm.advisePageCount[unix.MADV_PAGEOUT]*constUPagesize)/float64(1024*1024*1024)))
	}
	return lines
}

func (s *Stats) tableMovePages(format string) []string {
	lines := []string{"table: move_pages syscalls"}
	headers := []interface{}{"pid", "calls", "req[pages]", "ok[pages]", "moved[G]", "targetnode:moved[G]"}
	headerFmt := map[string]string{
		"csv": "%s,%s,%s,%s,%s,%s",
		"txt": "%8s %8s %10s %10s %8s %s",
	}
	rowFmt := map[string]string{
		"csv": "%d,%d,%d,%d,%.6f,%s",
		"txt": "%8d %8d %10d %10d %8.3f %s",
	}
	lines = append(lines, fmt.Sprintf(headerFmt[format], headers...))
	for _, pid := range s.pidMoves.sortedKeys() {
		spm := s.pidMoves[pid]
		node_moved_list := []string{}
		for _, node := range spm.sumDestNodePages.sortedKeys() {
			node_moved_list = append(node_moved_list, fmt.Sprintf("%d:%.3f",
				node,
				float64(spm.sumDestNodePages[node]*constUPagesize)/float64(1024*1024*1024)))
		}
		node_moved := strings.Join(node_moved_list, ";")
		lines = append(lines, fmt.Sprintf(rowFmt[format],
			pid,
			spm.sumSyscalls,
			spm.sumReqs,
			spm.sumDestNode,
			float64(spm.sumDestNode*constUPagesize)/float64(1024*1024*1024),
			node_moved))
	}
	return lines
}

func (s *Stats) tableMovePagesErrors(format string) []string {
	lines := []string{"table: move_pages syscall errors in page statuses"}
	headers := []interface{}{"pid", "pages", "size[G]", "errno", "error"}
	headerFmt := map[string]string{
		"csv": "%s,%s,%s,%s,%s",
		"txt": "%8s %8s %8s %8s %s",
	}
	rowFmt := map[string]string{
		"csv": "%d,%d,%.6f,%d,%s",
		"txt": "%8d %8d %8.3f %8d %s",
	}
	lines = append(lines, fmt.Sprintf(headerFmt[format], headers...))
	for pid, spm := range s.pidMoves {
		for _, errno := range spm.sumErrorCounts.sortedKeys() {
			lines = append(lines, fmt.Sprintf(rowFmt[format],
				pid,
				spm.sumErrorCounts[errno],
				float64(spm.sumErrorCounts[errno]*constUPagesize)/float64(1024*1024*1024),
				errno,
				syscall.Errno(errno)))
		}
	}
	return lines
}

func (s *Stats) tableMemoryScans(format string) []string {
	lines := []string{"table: memory scans"}
	headers := []interface{}{"pid", "scans", "tot[pages]", "avg[s]", "last", "max[s]", "avg[G]", "last[G]", "a+w[%%]", "last"}
	headerFmt := map[string]string{
		"csv": "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s",
		"txt": "%8s %8s %12s %6s %6s %6s %8s %8s %5s %5s",
	}
	rowFmt := map[string]string{
		"csv": "%d,%d,%d,%.6f,%.6f,%.6f,%.6f,%.6f,%.6f %.6f",
		"txt": "%8d %8d %12d %6.3f %6.3f %6.3f %8.3f %8.3f %5.2f %5.2f",
	}
	lines = append(lines, fmt.Sprintf(headerFmt[format], headers...))
	lines = append(lines, "")
	lines = append(lines)
	for _, pid := range s.pidScans.sortedKeys() {
		sps := s.pidScans[pid]
		lines = append(lines, fmt.Sprintf(rowFmt[format],
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
	return lines
}

func (s *Stats) Summarize(format string, tables ...string) string {
	allTables := []string{"events", "process_madvise", "move_pages", "move_pages_errors", "memory_scans"}
	lines := []string{}
	if len(tables) == 0 {
		tables = allTables
	}
	if format != "txt" && format != "csv" {
		return fmt.Sprintf("unknown format %q, txt and csv expected", format)
	}
	for _, table := range tables {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		switch table {
		case "events":
			lines = append(lines, s.tableEvents(format)...)
		case "process_madvise":
			lines = append(lines, s.tableProcessMadvise(format)...)
		case "move_pages":
			lines = append(lines, s.tableMovePages(format)...)
		case "move_pages_errors":
			lines = append(lines, s.tableMovePagesErrors(format)...)
		case "memory_scans":
			lines = append(lines, s.tableMemoryScans(format)...)
		default:
			lines = append(lines, fmt.Sprintf("unknown table %q, available: \"%s\"", table, strings.Join(allTables, "\", \"")))
		}
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
