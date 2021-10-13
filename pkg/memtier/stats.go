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
	fmt.Printf("DEBUG STORE: %v\n", stat)
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
	return fmt.Sprintf("%v", sm)
}
