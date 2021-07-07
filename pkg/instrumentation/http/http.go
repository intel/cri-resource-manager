// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
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

package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// httpServer is used in log messages.
	httpServer = "HTTP server"
)

// Our logger instance.
var log = logger.NewLogger("http")

// ServeMux is our HTTP request multiplexer with removable handlers.
type ServeMux struct {
	sync.RWMutex
	handlers map[string]http.Handler
	mux      *http.ServeMux
}

// NewServeMux create a new HTTP request multiplexer.
func NewServeMux() *ServeMux {
	return &ServeMux{
		handlers: make(map[string]http.Handler),
		mux:      http.NewServeMux(),
	}
}

// Handle registers a handler for the given pattern.
func (mux *ServeMux) Handle(pattern string, handler http.Handler) {
	mux.Lock()
	defer mux.Unlock()

	log.Debugf("registering handler for %q...", pattern)

	if _, ok := mux.handlers[pattern]; ok {
		log.Errorf("can't register duplicate HTTP handler for %q", pattern)
		return
	}

	mux.handlers[pattern] = handler
	mux.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function for the given pattern.
func (mux *ServeMux) HandleFunc(pattern string, fn func(http.ResponseWriter, *http.Request)) {
	mux.Lock()
	defer mux.Unlock()

	log.Debugf("registering handler function for %q...", pattern)

	if _, ok := mux.handlers[pattern]; ok {
		log.Errorf("can't register duplicate HTTP handler function for '%s'", pattern)
		return
	}

	handler := http.HandlerFunc(fn)

	mux.handlers[pattern] = handler
	mux.mux.Handle(pattern, handler)
}

// Unregister unregister any handlers for the given pattern.
func (mux *ServeMux) Unregister(pattern string) (http.Handler, bool) {
	mux.Lock()
	defer mux.Unlock()

	h, ok := mux.handlers[pattern]
	if !ok {
		return nil, false
	}

	log.Debugf("unregistering handler for %q...", pattern)

	delete(mux.handlers, pattern)
	mux.mux = http.NewServeMux()
	for pattern, handler := range mux.handlers {
		mux.mux.Handle(pattern, handler)
	}

	return h, true
}

// ServeHTTP serves a HTTP request.
func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux.RLock()
	defer mux.RUnlock()
	log.Debugf("serving %s...", r.URL)
	mux.mux.ServeHTTP(w, r)
}

// Server is our HTTP server, with support for unregistering handlers.
type Server struct {
	sync.RWMutex
	server *http.Server
	mux    *ServeMux
}

// NewServer creates a new server instance.
func NewServer() *Server {
	return &Server{
		mux: NewServeMux(),
	}
}

// GetMux returns the mux for this server.
func (s *Server) GetMux() *ServeMux {
	return s.mux
}

// GetAddress returns the current server HTTP endpoint/address.
func (s *Server) GetAddress() string {
	if s.server == nil {
		return ""
	}
	return s.server.Addr
}

// Start sets up the server to listen and serve on the given address.
func (s *Server) Start(addr string) error {
	if addr == "" {
		log.Infof("%s is disabled", httpServer)
		return nil
	}

	log.Infof("starting %s...", httpServer)

	s.Lock()
	defer s.Unlock()

	s.server = &http.Server{Addr: addr, Handler: s}
	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return httpError("can't listen on HTTP TCP address '%s': %v",
			s.server.Addr, err)
	}

	// update address if port was autobound
	if ln.Addr().String() != s.server.Addr {
		s.server.Addr = ln.Addr().String()
	}

	go s.server.Serve(ln)

	return nil
}

// Stop Close()'s the server immediately.
func (s *Server) Stop() {
	log.Infof("stopping %s...", httpServer)

	s.Lock()
	defer s.Unlock()

	if s.server == nil {
		return
	}

	s.server.Close()
	s.server = nil
}

// Shutdown shuts down the server gracefully.
func (s *Server) Shutdown(wait bool) {
	var sync chan struct{}

	log.Infof("shutting down %s...", httpServer)

	s.Lock()
	defer s.Unlock()

	if s.server == nil {
		return
	}

	if wait {
		sync = make(chan struct{})
		s.server.RegisterOnShutdown(func() {
			close(sync)
		})
	}
	s.server.Shutdown(context.Background())
	_ = <-sync

	s.server = nil
}

// Reconfigure reconfigures the server.
func (s *Server) Reconfigure(addr string) error {
	log.Infof("reconfiguring %s...", httpServer)

	if s.GetAddress() != addr {
		return s.Restart(addr)
	}
	return nil
}

// Restart restarts it on the given address.
func (s *Server) Restart(addr string) error {
	log.Infof("restarting %s...", httpServer)

	s.Stop()
	return s.Start(addr)
}

// ServeHTTP servers the given HTTP request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.RLock()
	defer s.RUnlock()
	s.mux.ServeHTTP(w, r)
}

// httpError returns a formatted instrumentation/http-specific error.
func httpError(format string, args ...interface{}) error {
	return fmt.Errorf("instrumentation/http: "+format, args...)
}
