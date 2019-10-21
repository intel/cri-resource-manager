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
	"strings"
	"sync"
	"time"

	config "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/relay"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/agent"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	resmgrcfg "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	control "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/resource-control"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/rdt"
)

// ResourceManager is the interface we expose for controlling the CRI resource manager.
type ResourceManager interface {
	// Start starts the resource manager.
	Start() error
	// Stop stops the resource manager.
	Stop()
	// SetConfig dynamically updates the resource manager  configuration
	SetConfig(*resmgrcfg.RawConfig) error
}

// resmgr is the implementation of ResourceManager.
type resmgr struct {
	logger.Logger
	sync.Mutex
	relay        relay.Relay      // our CRI relay
	cache        cache.Cache      // cached state
	policy       policy.Policy    // resource manager policy
	configServer resmgrcfg.Server // configuration management server
	rdt          control.CriRdt   // RDT enforcement
}

// NewResourceManager creates a new ResourceManager instance.
func NewResourceManager() (ResourceManager, error) {
	var err error

	m := &resmgr{
		Logger: logger.NewLogger("resource-manager"),
	}

	// Set-up connection to cri-resmgr agent
	agent, err := agent.NewAgentInterface(opt.AgentSocket)
	if err != nil {
		m.Warn("failed to connect to cri-resmgr agent: %v", err)
	}

	// Get configuration
	conf, err := agent.GetConfig(1 * time.Second)
	if err != nil {
		m.Error("failed to retrieve configuration")
	}

	ropts := relay.Options{
		RelaySocket:   opt.RelaySocket,
		ImageSocket:   opt.ImageSocket,
		RuntimeSocket: opt.RuntimeSocket,
	}
	if m.relay, err = relay.NewRelay(ropts); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	copts := cache.Options{
		CacheDir: opt.RelayDir,
		Policy:   policy.ActivePolicy(),
	}
	if m.cache, err = cache.NewCache(copts); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if conf == nil || len(conf.Data) == 0 {
		m.Warn("failed to fetch configuration, using last cached data")
		conf = m.cache.GetConfig()
	}
	if conf == nil {
		conf = &resmgrcfg.RawConfig{}
	}

	if !opt.NoRdt {
		if err = m.setupRdt(conf.Get("rdt")); err != nil {
			return nil, resmgrError("failed to create resource manager: %v", err)
		}
	}

	policyOpts := &policy.Options{
		ResmgrConfig: conf,
		AgentCli:     agent,
		Rdt:          m.rdt,
	}
	if m.policy, err = policy.NewPolicy(policyOpts); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if err = m.relay.Setup(); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if err = m.setupPolicyHooks(); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if m.configServer, err = resmgrcfg.NewConfigServer(m.SetConfig); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	return m, nil
}

// Start starts the resource manager.
func (m *resmgr) Start() error {
	m.Info("starting resource manager...")

	m.Lock()
	defer m.Unlock()

	if err := m.configServer.Start(opt.ConfigSocket); err != nil {
		return resmgrError("failed to start config-server: %v", err)
	}

	if err := m.startPolicy(); err != nil {
		return err
	}

	if err := m.relay.Start(); err != nil {
		return resmgrError("failed to start relay: %v", err)
	}

	m.Info("up and running...")
	return nil
}

// Stop stops the resource manager.
func (m *resmgr) Stop() {
	m.relay.Client().Close()
	m.relay.Server().Stop()
}

// Set new resource manager configuration.
func (m *resmgr) SetConfig(conf *resmgrcfg.RawConfig) error {
	m.Info("received configuration update")

	m.Lock()
	defer m.Unlock()

	data, key := m.extractNodeConfig(conf, conf.NodeName)
	if key == "" {
		m.Error("failed to extract node (%s) or default configuration", conf.NodeName)
		return resmgrError("failed to extract node (%s) or default configuration", conf.NodeName)
	}

	if m.DebugEnabled() {
		for _, line := range strings.Split(data, "\n") {
			m.Debug("config '%s': %s", key, line)
		}
	}

	if err := config.ParseYAMLData([]byte(data), config.External); err != nil {
		m.Error("failed to activate updated configuration: %v", err)
		return resmgrError("failed to activate updated configuration: %v", err)
	}

	//m.cache.SetConfig(conf)

	m.Info("updated configuration activated")
	return nil
}

// extractNodeConfig picks configuration for this node from the raw resmgrcfg.
func (m *resmgr) extractNodeConfig(rawConfig *resmgrcfg.RawConfig, node string) (string, string) {
	keys := []string{node, "default"}

	for _, key := range keys {
		if cfg, ok := rawConfig.Data[key]; ok {
			return cfg, key
		}
	}

	return "", ""
}

func (m *resmgr) setupRdt(conf string) error {
	var err error

	path := opt.ResctrlPath
	if path == "" {
		// Try to find the resctrl mount point
		path, err = rdt.ResctrlMountPath()
		if err != nil {
			m.Info("unable to find the resctrl mount point, RDT enforcement is disabled")
			return nil
		}
		m.Info("using auto-discovered resctrl-path %q", path)
	}
	m.rdt, err = control.NewCriRdt(path, conf)
	return err
}
