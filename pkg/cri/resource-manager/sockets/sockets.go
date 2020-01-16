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

package sockets

const (
	// Containerd is the CRI socket containerd listens on.
	Containerd = "/var/run/containerd/containerd.sock"
	// ResourceManagerRelay is the CRI socket the resource manager listens on.
	ResourceManagerRelay = "/var/run/cri-resmgr/cri-resmgr.sock"
	// ResourceManagerAgent is the socket the resource manager node agent listens on.
	ResourceManagerAgent = "/var/run/cri-resmgr/cri-resmgr-agent.sock"
	// ResourceManagerConfig for resource manager configuration notifications.
	ResourceManagerConfig = "/var/run/cri-resmgr/cri-resmgr-config.sock"
)
