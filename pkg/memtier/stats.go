package memtier

import (
	"fmt"
	"strings"
)

type StatsStorage struct {
	stored []interface{}
}

type StatsMoved struct {
	pid            int
	sysRet         uint
	firstPageAddr  uintptr
	destNode       int
	reqCount       int
	destNodeCount  int
	otherNodeCount int
	errCount       int
}

var stats *StatsStorage = newStats()

func newStats() *StatsStorage {
	return &StatsStorage{
		stored: []interface{}{},
	}
}

func Stats() *StatsStorage {
	return stats
}

func (s *StatsStorage) Store(stat interface{}) {
	s.stored = append(s.stored, stat)
}

func (s *StatsStorage) Summarize() string {
	movesNodePages := map[int]int64{}
	for _, entry := range s.stored {
		switch v := entry.(type) {
		case StatsMoved:
			movesNodePages[v.destNode] += int64(v.destNodeCount)
		}
	}
	entries := []string{}
	for node, pageCount := range movesNodePages {
		entries = append(entries, fmt.Sprintf("moved to %d: %d MB", node, (pageCount*constPagesize)/(1024*1024)))
	}
	return strings.Join(entries, "\n")
}

func (s *StatsStorage) Dump() string {
	entries := []string{}
	for _, entry := range s.stored {
		switch v := entry.(type) {
		case StatsMoved:
			entries = append(entries, v.String())
		}
	}
	return strings.Join(entries, "\n")
}

func (sm *StatsMoved) String() string {
	return fmt.Sprintf("move %d %d %d %x => %d %d %d %d",
		// inputs
		sm.pid, sm.destNode, sm.reqCount, sm.firstPageAddr,
		// results
		sm.sysRet, sm.destNodeCount, sm.otherNodeCount, sm.errCount)
}
