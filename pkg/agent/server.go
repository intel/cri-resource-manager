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

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	core_v1 "k8s.io/api/core/v1"
	k8sclient "k8s.io/client-go/kubernetes"

	v1 "github.com/intel/cri-resource-manager/pkg/agent/api/v1"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/sockets"
	"github.com/intel/cri-resource-manager/pkg/log"
)

// agentServer is the interface for our gRPC server.
type agentServer interface {
	Start(string) error
	Stop()
}

// server implements agentServer.
type server struct {
	log.Logger
	cli       *k8sclient.Clientset // client for accessing k8s api
	server    *grpc.Server         // gRPC server instance
	getConfig getConfigFn          // Getter function for current config
}

// newAgentServer creates new agentServer instance.
func newAgentServer(cli *k8sclient.Clientset, getFn getConfigFn) (agentServer, error) {
	s := &server{
		Logger:    log.NewLogger("server"),
		cli:       cli,
		getConfig: getFn,
	}

	return s, nil
}

// Start runs server instance.
func (s *server) Start(socket string) error {
	// Make sure we have a directory for the socket.
	if err := os.MkdirAll(filepath.Dir(socket), sockets.DirPermissions); err != nil {
		return agentError("failed to create directory for socket %s: %v", socket, err)
	}

	// Remove any leftover sockets.
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return agentError("failed to unlink socket file: %s", err)
	}

	// Create server listening for local unix domain socket
	lis, err := net.Listen("unix", socket)
	if err != nil {
		return agentError("failed to listen to socket: %v", err)
	}

	serverOpts := []grpc.ServerOption{}
	s.server = grpc.NewServer(serverOpts...)
	gs := &grpcServer{
		Logger:    s.Logger,
		cli:       s.cli,
		getConfig: s.getConfig,
	}
	v1.RegisterAgentServer(s.server, gs)

	s.Infof("starting gRPC server at socket %s", socket)
	go func() {
		defer lis.Close()
		err := s.server.Serve(lis)
		if err != nil {
			s.Fatalf("grpc server died: %v", err)
		}
	}()
	return nil
}

// Stop agentServer instance
func (s *server) Stop() {
	s.server.Stop()
}

// grpcServer implements v1.AgentServer
type grpcServer struct {
	log.Logger
	cli       *k8sclient.Clientset
	getConfig getConfigFn
}

// GetNode gets K8s node object.
func (g *grpcServer) GetNode(ctx context.Context, req *v1.GetNodeRequest) (*v1.GetNodeReply, error) {
	g.Debugf("received GetNodeRequest: %v", req)
	rpl := &v1.GetNodeReply{}

	node, err := getNodeObject(g.cli)
	if err != nil {
		return rpl, agentError("failed to get node object: %v", err)
	}
	serialized, err := json.Marshal(node)
	if err != nil {
		return rpl, agentError("failed to serialized node object: %v", err)
	}
	rpl.Node = string(serialized)

	return rpl, nil
}

// PatchNode patches the K8s node object.
func (g *grpcServer) PatchNode(ctx context.Context, req *v1.PatchNodeRequest) (*v1.PatchNodeReply, error) {
	g.Debugf("received PatchNodeRequest: %v", req)
	rpl := &v1.PatchNodeReply{}

	// Apply patches
	if len(req.Patches) > 0 {
		err := patchNode(g.cli, req.Patches)
		if err != nil {
			return rpl, agentError("failed to patch node object: %v", err)
		}
	}

	return rpl, nil
}

// UpdateNodeCapacity updates capacity in Node status
func (g *grpcServer) UpdateNodeCapacity(ctx context.Context, req *v1.UpdateNodeCapacityRequest) (*v1.UpdateNodeCapacityReply, error) {
	g.Debugf("received UpdateNodeCapacityRequest: %v", req)

	rpl := &v1.UpdateNodeCapacityReply{}

	capacity, sep := "", ""
	for name, count := range req.Capacities {
		if isNativeResource(name) {
			err := agentError("refusing to update capacity of native resource '%s'", name)
			return rpl, err
		}

		if !strings.Contains(name, ".") || !strings.Contains(name, "/") {
			err := agentError("invalid resource '%s' in capacity update", name)
			return rpl, err
		}

		capacity += sep + fmt.Sprintf(`"%s": "%s"`, name, count)
		sep = ", "
	}

	err := patchNodeStatus(g.cli, map[string]string{"capacity": "{" + capacity + "}"})

	return rpl, err
}

// HealthCheck checks if the agent is in healthy state
func (g *grpcServer) HealthCheck(ctx context.Context, req *v1.HealthCheckRequest) (*v1.HealthCheckReply, error) {
	g.Debugf("received HealthCheckRequest: %v", req)
	return &v1.HealthCheckReply{}, nil
}

func isNativeResource(name string) bool {
	switch {
	case name == string(core_v1.ResourceCPU), name == string(core_v1.ResourceMemory):
		return true
	case strings.HasPrefix(name, core_v1.ResourceHugePagesPrefix):
		return true
	default:
		return false
	}
}

// GetConfig gets the cri-resmgr configuration
func (g *grpcServer) GetConfig(ctx context.Context, req *v1.GetConfigRequest) (*v1.GetConfigReply, error) {
	g.Debugf("received GetConfigRequest: %v", req)
	rpl := &v1.GetConfigReply{
		NodeName: nodeName,
		Config:   resmgrConfig{},
	}

	if g.getConfig != nil {
		rpl.Config = g.getConfig()
	} else {
		g.Warnf("no getter method configured, returning empty config!")
	}
	return rpl, nil
}
