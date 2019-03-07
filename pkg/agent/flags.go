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
	"flag"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/sockets"
)

type options struct {
	kubeconfig    string
	agentSocket   string
	resmgrSocket  string
	configNs      string
	configMapName string
}

var opts = options{}

func init() {
	flag.StringVar(&opts.agentSocket, "agent-socket", sockets.ResourceManagerAgent, "Socket for incoming requests from cri-resmgr")
	flag.StringVar(&opts.resmgrSocket, "cri-resmgr-socket", sockets.ResourceManagerConfig, "cri-resmgr socket to connect to")
	flag.StringVar(&opts.kubeconfig, "kubeconfig", "", "Kubeconfig to use, empty string implies in-cluster config (i.e. running inside a Pod)")
	flag.StringVar(&opts.configNs, "config-ns", "kube-system", "Kubernetes namespace where to look for config")
	flag.StringVar(&opts.configMapName, "configmap-name", "cri-resmgr-config", "Name of the K8s ConfigMap to watch")
}
