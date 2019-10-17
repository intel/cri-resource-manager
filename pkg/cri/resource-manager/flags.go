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

package resmgr

import (
	"flag"
	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/sockets"
)

// CRI resource manager/relay options configurable via the command line.
type options struct {
	ImageSocket   string
	RuntimeSocket string
	RelaySocket   string
	RelayDir      string
	AgentSocket   string
	ConfigSocket  string
	NoRdt         bool
	ResctrlPath   string
}

// Relay options with their defaults.
var cfg *config.Module
var opt = options{}

// Register our command-line flags.
func init() {
	// Declare command line-only options first.
	flag.StringVar(&opt.ImageSocket, "image-socket", client.DontConnect,
		"Unix domain socket path where CRI image service requests should be relayed to.")
	flag.StringVar(&opt.RuntimeSocket, "runtime-socket", sockets.DockerShim,
		"Unix domain socket path where CRI runtime service requests should be relayed to.")
	flag.StringVar(&opt.RelaySocket, "relay-socket", sockets.ResourceManagerRelay,
		"Unix domain socket path where the resource manager should serve requests on.")
	flag.StringVar(&opt.RelayDir, "relay-dir", "/var/lib/cri-resmgr",
		"Permanent storage directory path for the resource manager to store its state in.")
	flag.StringVar(&opt.AgentSocket, "agent-socket", sockets.ResourceManagerAgent,
		"local socket of the cri-resmgr agent to connect")
	flag.StringVar(&opt.ConfigSocket, "config-socket", sockets.ResourceManagerConfig,
		"Unix domain socket path where the resource manager listens for cri-resmgr-agent")
	flag.StringVar(&opt.ResctrlPath, "resctrl-path", "",
		"Path of the resctrl filesystem mountpoint")

	// Declare options that we accept from any source.
	cfg = config.GetModule(config.MainModule)
	cfg.BoolVar(&opt.NoRdt, "no-rdt", false,
		"Disable RDT resource management")
}
