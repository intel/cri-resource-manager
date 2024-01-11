// Copyright 2019 Intel Corporation. All Rights Reserved.
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

package relay

import (
	"fmt"
	"os"
	"sync"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/server"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// DisableService is used to mark a socket/service to not be connected.
	DisableService = client.DontConnect
	// DefaultImageSocket uses the runtime socket for the image servie, too.
	DefaultImageSocket = "default"
)

// Options contains the configurable options of our CRI relay.
type Options struct {
	// RelaySocket is the socket path for the CRI relay services.
	RelaySocket string
	// ImageSocket is the socket path for the (real) CRI image services.
	ImageSocket string
	// RuntimeSocket is the socket path for the (real) CRI runtime services.
	RuntimeSocket string
	// QualifyReqFn produces context for disambiguating a CRI request/reply.
	QualifyReqFn func(interface{}) string
}

// Relay is the interface we expose for controlling our CRI relay.
type Relay interface {
	// Setup prepares the relay to start processing CRI requests.
	Setup() error
	// Start starts the relay.
	Start() error
	// Stop stops the relay.
	Stop()
	// Client returns the relays client interface.
	Client() client.Client
	// Server returns the relays server interface.
	Server() server.Server
}

// relay is the implementation of Relay.
type relay struct {
	logger.Logger
	sync.Mutex
	options Options       // relay options
	client  client.Client // relay CRI client
	server  server.Server // relay CRI server

	evtClient criv1.RuntimeService_GetContainerEventsClient
	evtChans  map[*criv1.GetEventsRequest]chan *criv1.ContainerEventResponse
}

// NewRelay creates a new relay instance.
func NewRelay(options Options) (Relay, error) {
	var err error

	r := &relay{
		Logger:   logger.NewLogger("cri/relay"),
		options:  options,
		evtChans: map[*criv1.GetEventsRequest]chan *criv1.ContainerEventResponse{},
	}

	imageSocket := r.options.ImageSocket
	if imageSocket == DefaultImageSocket {
		imageSocket = r.options.RuntimeSocket
	}

	cltopts := client.Options{
		ImageSocket:   imageSocket,
		RuntimeSocket: r.options.RuntimeSocket,
		DialNotify:    r.dialNotify,
	}
	if r.client, err = client.NewClient(cltopts); err != nil {
		return nil, relayError("failed to create relay client: %v", err)
	}

	srvopts := server.Options{
		Socket:       r.options.RelaySocket,
		User:         -1,
		Group:        -1,
		Mode:         0660,
		QualifyReqFn: r.options.QualifyReqFn,
	}
	if r.server, err = server.NewServer(srvopts); err != nil {
		return nil, relayError("failed to create relay server: %v", err)
	}

	return r, nil
}

// Setup prepares the relay to start processing requests.
func (r *relay) Setup() error {
	if err := r.client.Connect(client.ConnectOptions{Wait: true}); err != nil {
		return relayError("client connection failed: %v", err)
	}

	if r.options.ImageSocket != DisableService {
		if err := r.server.RegisterImageService(r); err != nil {
			return relayError("failed to register image service: %v", err)
		}
	}

	if r.options.RuntimeSocket != DisableService {
		if err := r.server.RegisterRuntimeService(r); err != nil {
			return relayError("failed to register runtime service: %v", err)
		}
	}

	return nil
}

// Start starts the relays request processing goroutine.
func (r *relay) Start() error {
	if err := r.server.Start(); err != nil {
		return relayError("failed to start relay: %v", err)
	}

	return nil
}

// Stop stops the relay.
func (r *relay) Stop() {
	r.client.Close()
	r.server.Stop()
}

// Client returns the relays Client interface.
func (r *relay) Client() client.Client {
	return r.client
}

// Server returns the relays Server interface.
func (r *relay) Server() server.Server {
	return r.server
}

func (r *relay) dialNotify(socket string, uid int, gid int, mode os.FileMode, err error) {
	if err != nil {
		r.Error("failed to determine permissions/ownership of client socket %q: %v",
			socket, err)
		return
	}

	// Notes:
	//   Kubelet has separate configuration/command line options for the container
	//   runtime's Image and Runtime Services. Hence, in principle it is possible
	//   that we run with two separate sockets for those. However, we always expose
	//   both services over our single relay socket. Since we cannot set two set of
	//   ownerships and permissions on a single socket, if this situation arises in
	//   practice we choose to go with the runtime socket's properties.
	if r.options.ImageSocket != r.options.RuntimeSocket {
		if socket != r.options.RuntimeSocket && r.options.RuntimeSocket != client.DontConnect {
			r.Warn("ignoring ownership/permissions of dedicated CR Image Service socket...")
			return
		}
	}

	if err := r.server.Chown(uid, gid); err != nil {
		r.Error("server socket ownership change request failed: %v", err)
	} else {
		if err := r.server.Chmod(mode); err != nil {
			r.Error("server socket permissions change request failed: %v", err)
		}
	}
}

// relayError creates a formatted relay-specific error.
func relayError(format string, args ...interface{}) error {
	return fmt.Errorf("cri/relay: "+format, args...)
}
