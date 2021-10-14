package memtier

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type MoverTask struct {
	pages  *Pages
	to     []Node
	offset int // index to the first page that is still to be moved
}

type MoverConfig struct {
	// process/container mem access prices per NUMA node
	Interval  int // in ms
	Bandwidth int // in MB/s
}

type Mover struct {
	mutex         sync.Mutex
	tasks         []*MoverTask
	config        *MoverConfig
	toTaskHandler chan taskHandlerCmd
	// channel for new tasks and (re)configuring
}

type taskHandlerCmd int

type taskStatus int

const (
	thContinue taskHandlerCmd = iota
	thQuit
	thPause

	tsContinue taskStatus = iota
	tsDone
	tsNoPagesOnSources
	tsNoDestinations
	tsBlocked
	tsError
)

func NewMover() *Mover {
	return &Mover{
		toTaskHandler: nil,
	}
}

func NewMoverTask(pages *Pages, toNode Node) *MoverTask {
	return &MoverTask{
		pages: pages,
		to:    []Node{toNode},
	}
}

func (m *Mover) SetConfigJson(configJson string) error {
	var config MoverConfig
	if err := json.Unmarshal([]byte(configJson), &config); err != nil {
		return err
	}
	m.SetConfig(&config)
	return nil
}

func (m *Mover) SetConfig(config *MoverConfig) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.config = config
}

func (m *Mover) Start() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.toTaskHandler == nil {
		m.toTaskHandler = make(chan taskHandlerCmd)
		go m.taskHandler()
	}
}

func (m *Mover) Pause() {
	if m.toTaskHandler != nil {
		m.toTaskHandler <- thPause
	}
}

func (m *Mover) Continue() {
	if m.toTaskHandler != nil {
		m.toTaskHandler <- thContinue
	}
}

func (m *Mover) AddTask(task *MoverTask) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.tasks = append(m.tasks, task)
	m.toTaskHandler <- thContinue
}

func (m *Mover) taskHandler() {
	for {
		fmt.Printf("taskHandler: waiting for a task\n")
		// blocking channel read when there are no tasks
		cmd := <-m.toTaskHandler
		switch cmd {
		case thQuit:
			return
		case thPause:
			break
		}
	busyloop:
		for {
			// handle tasks
			task := m.popTask()
			if task == nil {
				// no more tasks, back to blocking reads
				break
			}
			if ts := m.handleTask(task); ts == tsContinue {
				fmt.Printf("taskHandler: will rehandle the task\n")
				m.mutex.Lock()
				m.tasks = append(m.tasks, task)
				m.mutex.Unlock()
			}
			// non-blocking channel read when there are tasks
			select {
			case cmd := <-m.toTaskHandler:
				switch cmd {
				case thQuit:
					return
				case thPause:
					break busyloop
				}
			default:
				time.Sleep(time.Duration(m.config.Interval) * time.Millisecond)
			}
		}
	}
}

func (m *Mover) handleTask(task *MoverTask) taskStatus {
	pp := task.pages
	if task.offset > 0 {
		pp = pp.Offset(task.offset)
	}
	pageCountAfterOffset := len(pp.Pages())
	if pageCountAfterOffset == 0 {
		return tsDone
	}
	if task.to == nil || len(task.to) == 0 {
		return tsNoDestinations
	}
	// select destination memory node, now go with the first one
	toNode := task.to[0]
	// bandwidth is MB/s => bandwith * 1024 is kB/s
	// constPagesize is 4096 kB/page
	// count is ([kB/s] / [kB/page] = [page/s]) * ([ms] / 1000 [ms/s] == [s]) = [page]
	count := (m.config.Bandwidth * 1024 * 1024 / int(constPagesize)) * m.config.Interval / 1000
	if count == 0 {
		return tsBlocked
	}
	if _, err := pp.MoveTo(toNode, count); err != nil {
		return tsError
	}
	task.offset += count
	if len(task.pages.Offset(count).Pages()) > 0 {
		return tsContinue
	}
	return tsDone
}

func (m *Mover) TaskCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return len(m.tasks)
}

func (m *Mover) popTask() *MoverTask {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	taskCount := len(m.tasks)
	if taskCount == 0 {
		return nil
	}
	task := m.tasks[taskCount-1]
	m.tasks = m.tasks[:taskCount-1]
	return task
}
