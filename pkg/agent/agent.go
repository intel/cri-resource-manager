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
	"fmt"

	"github.com/intel/cri-resource-manager/pkg/log"
	k8sclient "k8s.io/client-go/kubernetes"
)

// Get cri-resmgr config
type getConfigFn func() resmgrConfig

// resmgrConfig represents cri-resmgr configuration
type resmgrConfig map[string]string

// ResourceManagerAgent is the interface exposed for the CRI Resource Manager Congig Agent
type ResourceManagerAgent interface {
	Run() error
}

// agent implements ResourceManagerAgent
type agent struct {
	log.Logger                      // Our logging interface
	cli        *k8sclient.Clientset // K8s client
	server     agentServer          // gRPC server listening for requests from cri-resource-manager
	watcher    k8sWatcher           // Watcher monitoring events in K8s cluster
	updater    configUpdater        // Client sending config updates to cri-resource-manager
}

// NewResourceManager creates a new instance of ResourceManagerAgent
func NewResourceManagerAgent() (ResourceManagerAgent, error) {
	var err error

	a := &agent{
		Logger: log.NewLogger("resource-manager-agent"),
	}

	if a.cli, err = a.getK8sClient(opts.kubeconfig); err != nil {
		return nil, agentError("failed to get k8s client: %v", err)
	}

	if a.watcher, err = newK8sWatcher(a.cli); err != nil {
		return nil, agentError("failed to initialize watcher instance: %v", err)
	}

	if a.server, err = newAgentServer(a.cli, a.watcher.GetConfig); err != nil {
		return nil, agentError("failed to initialize gRPC server")
	}

	if a.updater, err = newConfigUpdater(opts.resmgrSocket); err != nil {
		return nil, agentError("failed to initialize config updater instance: %v", err)
	}

	return a, nil
}

// Start starts the resource manager.
func (a *agent) Run() error {
	a.Info("starting CRI Resource Manager Agent")

	if err := a.server.Start(opts.agentSocket); err != nil {
		return agentError("failed to start gRPC server: %v", err)
	}

	if err := a.watcher.Start(); err != nil {
		return agentError("failed to start watcher: %v", err)
	}

	if err := a.updater.Start(); err != nil {
		return agentError("failed to start config updater: %v", err)
	}

	for {
		config := <-a.watcher.ConfigChan()
		a.updater.Update(&config)
	}

	return nil
}

func agentError(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
