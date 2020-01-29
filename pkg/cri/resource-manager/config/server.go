/*
Copyright 2019 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config/api/v1"
	"github.com/intel/cri-resource-manager/pkg/log"
)

// SetConfigCb is a callback function for SetConfig request
type SetConfigCb func(*RawConfig) error

// Server is the interface for our gRPC server.
type Server interface {
	Start(string) error
	Stop()
}

// server implements Server.
type server struct {
	log.Logger
	sync.Mutex               // lock for gRPC server against concurrent per-request goroutines.
	server      *grpc.Server // gRPC server instance
	setConfigCb SetConfigCb
}

// NewConfigServer creates new Server instance.
func NewConfigServer(cb SetConfigCb) (Server, error) {
	s := &server{
		Logger:      log.NewLogger("config-server"),
		setConfigCb: cb,
	}
	return s, nil
}

// Start runs server instance.
func (s *server) Start(socket string) error {
	// Remove socket file if it exists
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return serverError("failed to unlink socket file: %s", err)
	}

	// Create server listening for local unix domain socket
	lis, err := net.Listen("unix", socket)
	if err != nil {
		return serverError("failed to listen to socket: %v", err)
	}

	serverOpts := []grpc.ServerOption{}
	s.server = grpc.NewServer(serverOpts...)
	v1.RegisterConfigServer(s.server, s)

	s.Info("starting config-server at socket %s...", socket)
	go func() {
		defer lis.Close()
		err := s.server.Serve(lis)
		if err != nil {
			s.Fatal("config-server died: %v", err)
		}
	}()
	return nil

}

// Stop Server instance
func (s *server) Stop() {
	if s.server != nil {
		s.server.Stop()
		s.server = nil
	}
}

// GetNode gets K8s node object.
func (s *server) SetConfig(ctx context.Context, req *v1.SetConfigRequest) (*v1.SetConfigReply, error) {
	s.Lock()
	defer s.Unlock()

	s.Debug("REQUEST: %s", req)

	reply := &v1.SetConfigReply{}
	err := s.setConfigCb(&RawConfig{NodeName: req.NodeName, Data: req.Config})
	if err != nil {
		reply.Error = fmt.Sprintf("failed to apply configuration: %v", err)
	}

	return reply, nil
}

func serverError(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
