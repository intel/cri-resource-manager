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
	"time"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/sockets"
	"github.com/intel/cri-resource-manager/pkg/pidfile"
)

// Options captures our command line parameters.
type options struct {
	ImageSocket           string
	RuntimeSocket         string
	RelaySocket           string
	RelayDir              string
	AllowUntestedRuntimes bool
	AgentSocket           string
	ConfigSocket          string
	PidFile               string
	ResctrlPath           string
	FallbackConfig        string
	ForceConfig           string
	ForceConfigSignal     string
	DisablePolicySwitch   bool
	ResetPolicy           bool
	ResetConfig           bool
	MetricsTimer          time.Duration
	RebalanceTimer        time.Duration
	DisableUI             bool
}

// Relay command line options.
var opt = options{}

const (
	allowUntestedRuntimesFlag = "allow-untested-runtimes"
)

// Register us for command line option processing.
func init() {
	flag.StringVar(&opt.ImageSocket, "image-socket", sockets.Containerd,
		"Unix domain socket path where CRI image service requests should be relayed to.")
	flag.StringVar(&opt.RuntimeSocket, "runtime-socket", sockets.Containerd,
		"Unix domain socket path where CRI runtime service requests should be relayed to.")
	flag.StringVar(&opt.RelaySocket, "relay-socket", sockets.ResourceManagerRelay,
		"Unix domain socket path where the resource manager should serve requests on.")
	flag.StringVar(&opt.RelayDir, "relay-dir", "/var/lib/cri-resmgr",
		"Permanent storage directory path for the resource manager to store its state in.")
	flag.BoolVar(&opt.AllowUntestedRuntimes, allowUntestedRuntimesFlag, false,
		"Allow proxying for untested CRI runtimes. Usually this is not a good idea.")

	flag.StringVar(&opt.AgentSocket, "agent-socket", sockets.ResourceManagerAgent,
		"local socket of the cri-resmgr agent to connect")
	flag.StringVar(&opt.ConfigSocket, "config-socket", sockets.ResourceManagerConfig,
		"Unix domain socket path where the resource manager listens for cri-resmgr-agent")
	flag.StringVar(&opt.PidFile, "pid-file", pidfile.GetPath(),
		"PID file to write daemon PID to")
	flag.StringVar(&opt.FallbackConfig, "fallback-config", "",
		"Fallback configuration to use unless/until one is available from the cache or agent.")
	flag.StringVar(&opt.ForceConfig, "force-config", "",
		"Configuration used to override the one stored in the cache. Does not override the agent.")
	flag.StringVar(&opt.ForceConfigSignal, "force-config-signal", "SIGHUP",
		"Signal used to reload forced configuration.")
	flag.BoolVar(&opt.ResetConfig, "reset-config", false,
		"Remove configuration (from the agent) stored in the cache, then exit.")

	flag.BoolVar(&opt.ResetPolicy, "reset-policy", false,
		"Reset policy data stored in the cache, then exit.")
	flag.BoolVar(&opt.DisablePolicySwitch, "disable-policy-switch", false,
		"Disable switching policies during startup.")

	flag.DurationVar(&opt.MetricsTimer, "metrics-interval", 0,
		"Interval for polling/gathering runtime metrics data. Use 'disable' for disabling.")
	flag.DurationVar(&opt.RebalanceTimer, "rebalance-interval", 0,
		"Minimum interval between two container rebalancing attempts. Use 'disable' for disabling.")

	flag.BoolVar(&opt.DisableUI, "disable-ui", false,
		"Disable serving container placement visualization UIs.")
}
