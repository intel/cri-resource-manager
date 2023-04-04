/*
Copyright 2020 Intel Corporation

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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"

	agent_v1 "github.com/intel/cri-resource-manager/pkg/agent/api/v1"
	v1 "github.com/intel/cri-resource-manager/pkg/agent/api/v1"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/sockets"
	"github.com/intel/cri-resource-manager/pkg/log"
)

func main() {
	socket := flag.String("agent-socket", sockets.ResourceManagerAgent, "Unix domain socket where agent is serving")
	query := flag.String("query", "", fmt.Sprintf("query to send, use %q to query status of last config push to resmgr", v1.ConfigStatus))

	// Disable logger buffering and make sure that everything has been flushed
	// when program exits
	log.Flush()
	defer log.Flush()

	flag.Parse()

	// Try to connect to agent
	dialOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithDialer(func(sock string, timeout time.Duration) (net.Conn, error) {
			return net.Dial("unix", sock)
		}),
	}
	conn, err := grpc.Dial(*socket, dialOpts...)
	if err != nil {
		log.Fatal("failed to connect to agent: %v", err)
	}
	cli := agent_v1.NewAgentClient(conn)

	// Do health check
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	rpl, err := cli.HealthCheck(ctx, &agent_v1.HealthCheckRequest{
		Query: *query,
	})
	if err != nil {
		log.Fatal("%v", err)
	}
	if rpl.Error != "" {
		log.Fatal("health check negative: %s", rpl.Error)
	}
	log.Info("Health check OK")
}
