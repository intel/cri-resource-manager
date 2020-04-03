// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package introspect

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	xhttp "github.com/intel/cri-resource-manager/pkg/instrumentation/http"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/topology"
)

// Pod describes a single pod and its containers.
type Pod struct {
	ID         string                // pod CRI ID
	UID        string                // pod kubernetes ID
	Name       string                // pod name
	Containers map[string]*Container // containers of this pod
}

// Container describes a single container.
type Container struct {
	ID            string        // container CRI ID
	Name          string        // container name
	Command       []string      // command
	Args          []string      // and its arguments
	CPURequest    int64         // CPU requested in milli-CPU (guaranteed amount)
	CPULimit      int64         // CPU limit in milli-CPU (maximum allowed CPU)
	MemoryRequest int64         // memory requested in bytes
	MemoryLimit   int64         // memory limit in bytes (maximum allowed memory)
	Hints         TopologyHints // topology/allocation hints
}

// TopologyHints contain a set of allocation hints for a container.
type TopologyHints topology.Hints

// Assignment describes resource assignments for a single container.
type Assignment struct {
	ContainerID   string // ID of container for this assignment
	SharedCPUs    string // shared CPUs
	CPUShare      int    // CPU share/weight for SharedCPUs
	ExclusiveCPUs string // exclusive CPUs
	Memory        string // memory controllers
	Pool          string // pool container is assigned to
}

// Pool describes a single (resource) pool.
type Pool struct {
	Name     string   // pool name
	CPUs     string   // CPUs in this pool
	Memory   string   // memory controllers (NUMA nodes) for this pool
	Parent   string   // parent pool
	Children []string // child pools
}

// Socket describes a single physical CPU socket in the system.
type Socket struct {
	ID   int    // CPU ID
	CPUs string // CPUs in this socket
}

// Node describes a single NUMA node in the system.
type Node struct {
	ID   int    // node ID
	CPUs string // CPUs with locality for this NUMA node.
}

// System describes the underlying HW/system.
type System struct {
	Sockets    map[int]*Socket // physical sockets in the system
	Nodes      map[int]*Node   // NUMA nodes in the system
	Isolated   string          // kernel-isolated CPUs
	Offlined   string          // CPUs offline
	RDTClasses []string        // RDT classes
	Policy     string          // active policy
}

// State is the current introspected state of the resource manager.
type State struct {
	Pools       map[string]*Pool       // pools
	Pods        map[string]*Pod        // pods and containers
	Assignments map[string]*Assignment // resource assignments
	System      *System                // info about hardware/system
	Error       string
}

// our logger instance
var log = logger.NewLogger("instrospect")

// Server is our server for external introspection.
type Server struct {
	sync.RWMutex                 // need to protect against concurrent introspection/update
	mux          *xhttp.ServeMux // our HTTP request multiplexer
	state        *State          // introspection data
	data         string          // state as a JSON string
	ready        bool
}

// Setup prepares the given HTTP request multiplexer for serving introspection.
func Setup(mux *xhttp.ServeMux, state *State) (*Server, error) {
	s := &Server{mux: mux}
	if err := s.set(state); err != nil {
		return nil, err
	}
	mux.HandleFunc("/introspect", s.serve)
	return s, nil
}

// Set sets the current state for introspection.
func (s *Server) Set(state *State) error {
	s.Lock()
	defer s.Unlock()
	return s.set(state)
}

// Start enables serving HTTP requests.
func (s *Server) Start() {
	log.Info("starting introspection server...")
	s.ready = true
}

// Stop stops serving further HTTP requests.
func (s *Server) Stop() {
	log.Info("stopping introspection server...")
	s.ready = false
}

// set sets the given state and encodes it as a JSON string.
func (s *Server) set(state *State) error {
	log.Debug("updating introspection data...")
	s.state = state
	data, err := json.Marshal(s.state)
	if err != nil {
		err = introspectError("failed to marshal state for introspection: %v", err)
		s.state = &State{Error: fmt.Sprintf("%v", err)}
		data, _ = json.Marshal(s.state)
	}

	s.data = string(data)
	return err
}

// serve serves a single HTTP request.
func (s *Server) serve(w http.ResponseWriter, req *http.Request) {
	if !s.ready {
		return
	}
	log.Debug("serving introspection data...")
	s.RLock()
	fmt.Fprintf(w, "%s\r\n", s.data)
	s.RUnlock()
}

// introspectError creates an introspection-specific error.
func introspectError(format string, args ...interface{}) error {
	return fmt.Errorf("introspection: "+format, args...)
}
