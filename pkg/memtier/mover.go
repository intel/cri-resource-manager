package memtier

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type MoverConfig struct {
	// process/container mem access prices per NUMA node
	IntervalMs int // in ms
	Bandwidth  int // in MB/s
}

const moverDefaults string = "{\"IntervalMs\":10,\"Bandwidth\":100}"

type MoverTask struct {
	pages  *Pages
	to     []Node
	offset int // index to the first page that is still to be moved
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

func (mt *MoverTask) String() string {
	pid := mt.pages.Pid()
	p := mt.pages.Pages()
	nextAddr := "N/A"
	if len(p) > mt.offset {
		nextAddr = fmt.Sprintf("%x", p[mt.offset].Addr())
	}
	return fmt.Sprintf("MoverTask{pid: %d, next: %s, page: %d out of %d, dest: %v}", pid, nextAddr, mt.offset, len(p), mt.to)
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

func (m *Mover) GetConfigJson() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.config == nil {
		return ""
	}
	if configStr, err := json.Marshal(m.config); err == nil {
		return string(configStr)
	}
	return ""
}

func (m *Mover) Start() error {
	if m.config == nil {
		if err := m.SetConfigJson(moverDefaults); err != nil {
			return fmt.Errorf("start failed on default configuration error: %w", err)
		}
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.toTaskHandler == nil {
		m.toTaskHandler = make(chan taskHandlerCmd, 8)
		go m.taskHandler()
	}
	return nil
}

func (m *Mover) Stop() {
	if m.toTaskHandler != nil {
		m.toTaskHandler <- thQuit
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

func (m *Mover) Tasks() []*MoverTask {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	tasks := make([]*MoverTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		taskCopy := *task
		tasks = append(tasks, &taskCopy)
	}
	return tasks
}

func (m *Mover) AddTask(task *MoverTask) {
	m.mutex.Lock()
	m.tasks = append(m.tasks, task)
	// m.mutex must be unlocked before using the channel,
	// otherwise taskHandler may never read the channel
	// because it is waiting for the lock.
	m.mutex.Unlock()
	m.toTaskHandler <- thContinue
}

func (m *Mover) RemoveTask(taskId int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if taskId < 0 || taskId >= len(m.tasks) {
		return
	}
	m.tasks = append(m.tasks[0:taskId], m.tasks[taskId+1:]...)
}

func (m *Mover) taskHandler() {
	log.Debugf("Mover: online\n")
	defer func() {
		close(m.toTaskHandler)
		m.toTaskHandler = nil
		log.Debugf("Mover: offline\n")
	}()
	for {
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
				time.Sleep(time.Duration(m.config.IntervalMs) * time.Millisecond)
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
	count := (m.config.Bandwidth * 1024 * 1024 / int(constPagesize)) * m.config.IntervalMs / 1000
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
