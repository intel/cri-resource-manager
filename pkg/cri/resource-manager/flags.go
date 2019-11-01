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

// Options captures our command line or runtime configurable parameters.
type options struct {
	ImageSocket   string `json:",omitempty"`
	RuntimeSocket string `json:",omitempty"`
	RelaySocket   string `json:",omitempty"`
	RelayDir      string `json:",omitempty"`
	AgentSocket   string `json:",omitempty"`
	ConfigSocket  string `json:",omitempty"`
	ResctrlPath   string `json:",omitempty"`
	NoRdt         bool
}

// conf captures our runtime configurable parameters.
type conf struct {
	// NoRdt disables RDT resource management.
	NoRdt bool
}

// Relay command line options and runtime configuration with their defaults.
var opt = defaultOptions().(*options)
var cfg = defaultConfig().(*conf)

// configNotify propagates runtime configurable changes to our options.
func (o *options) configNotify(event config.Event, source config.Source) error {
	o.NoRdt = cfg.NoRdt
	return nil
}

// defaultOptions returns a new options instance, all initialized to defaults.
func defaultOptions() interface{} {
	return &options{
		ImageSocket:   client.DontConnect,
		RuntimeSocket: sockets.DockerShim,
		RelaySocket:   sockets.ResourceManagerRelay,
		RelayDir:      "/var/libb/cri-resmgr",
		AgentSocket:   sockets.ResourceManagerAgent,
		ConfigSocket:  sockets.ResourceManagerConfig,
		ResctrlPath:   "",
		NoRdt:         defaultConfig().(*conf).NoRdt,
	}
}

// defaultConfig returns a new conf instance, all initialized to defaults.
func defaultConfig() interface{} {
	return &conf{
		NoRdt: false,
	}
}

// Register us for command line option processing and configuration handling.
func init() {
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
	flag.BoolVar(&opt.NoRdt, "no-rdt", false,
		"Disable RDT resource management")
	flag.StringVar(&opt.ResctrlPath, "resctrl-path", "",
		"Path of the resctrl filesystem mountpoint")

	config.Register("resource-manager", "Resource Management", cfg, defaultConfig,
		config.WithNotify(opt.configNotify))
}
