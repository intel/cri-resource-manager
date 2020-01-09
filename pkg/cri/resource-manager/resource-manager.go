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
	"sync"
	"time"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/cri/relay"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/agent"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	config "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

// ResourceManager is the interface we expose for controlling the CRI resource manager.
type ResourceManager interface {
	// Start starts the resource manager.
	Start() error
	// Stop stops the resource manager.
	Stop()
	// SetConfig dynamically updates the resource manager  configuration
	SetConfig(*config.RawConfig) error
}

// resmgr is the implementation of ResourceManager.
type resmgr struct {
	logger.Logger
	sync.Mutex
	relay        relay.Relay     // our CRI relay
	cache        cache.Cache     // cached state
	policy       policy.Policy   // resource manager policy
	configServer config.Server   // configuration management server
	control      control.Control // policy controllers/enforcement
	agent        agent.Interface // connection to cri-resmgr agent
	conf         *config.RawConfig
}

// NewResourceManager creates a new ResourceManager instance.
func NewResourceManager() (ResourceManager, error) {
	var err error

	if opt.ForceConfig != "" && opt.FallbackConfig != "" {
		return nil, resmgrError("both fallback (%s) and forced (%s) configuration given",
			opt.FallbackConfig, opt.ForceConfig)
	}

	m := &resmgr{
		Logger: logger.NewLogger("resource-manager"),
	}

	// Set-up connection to cri-resmgr agent
	m.agent, err = agent.NewAgentInterface(opt.AgentSocket)
	if err != nil {
		m.Warn("failed to connect to cri-resmgr agent: %v", err)
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
	}
	if m.cache, err = cache.NewCache(copts); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if err = m.loadInitialConfig(); err != nil {
		return nil, resmgrError("failed to load initial configuration: %v", err)
	}

	if policy.ActivePolicy() != m.cache.GetActivePolicy() {
		if m.cache.GetActivePolicy() != "" {
			return nil, resmgrError("trying to load cache with policy %s for active policy %s",
				m.cache.GetActivePolicy(), policy.ActivePolicy())
		}
		m.cache.SetActivePolicy(policy.ActivePolicy())
	}

	policyOpts := &policy.Options{
		AgentCli: m.agent,
	}
	if m.policy, err = policy.NewPolicy(m.cache, policyOpts); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if err = m.relay.Setup(); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if err = m.setupRequestProcessing(); err != nil {
		return nil, resmgrError("failed to create resource manager: %v", err)
	}

	if m.control, err = control.NewControl(); err != nil {
		return nil, resmgrError("failed to create controller: %v", err)
	}

	if opt.ForceConfig != "" {
		m.Warn("using forced configuration %s, not starting config server...", opt.ForceConfig)
	} else {
		if m.configServer, err = config.NewConfigServer(m.SetConfig); err != nil {
			return nil, resmgrError("failed to create resource manager: %v", err)
		}
	}

	return m, nil
}

// Start starts the resource manager.
func (m *resmgr) Start() error {
	m.Info("starting resource manager...")

	m.Lock()
	defer m.Unlock()

	if opt.ForceConfig == "" {
		if err := m.configServer.Start(opt.ConfigSocket); err != nil {
			return resmgrError("failed to start config-server: %v", err)
		}
	}

	if err := m.activateRequestProcessing(); err != nil {
		return err
	}

	if err := m.control.StartStopControllers(m.cache, m.relay.Client()); err != nil {
		return resmgrError("failed to start controller: %v", err)
	}

	if err := m.relay.Start(); err != nil {
		return resmgrError("failed to start relay: %v", err)
	}

	// Temporary hack to get pkg/rdt properly configured.
	// TODO: remove after pkg/rdt config management is fixed
	if err := m.loadInitialConfig(); err != nil {
		return resmgrError("failed to load initial configuration: %v", err)
	}

	if m.conf != nil {
		m.cache.SetConfig(m.conf)
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
func (m *resmgr) SetConfig(conf *config.RawConfig) error {
	m.Info("received new configuration")

	m.Lock()
	defer m.Unlock()

	// TODO: save current configuration and roll back if some controllers fail to start

	if err := pkgcfg.SetConfig(conf.Data); err != nil {
		m.Error("failed to update configuration: %v", err)
		return resmgrError("configuration rejected: %v", err)
	}

	if err := m.control.StartStopControllers(m.cache, m.relay.Client()); err != nil {
		m.Error("failed to activate configuration: %v", err)
		return resmgrError("failed to fully activate configuration: %v", err)
	}

	m.cache.SetConfig(conf)
	m.Info("configuration successfully updated")

	return nil
}

// loadInitialConfig tries to load initial configuration using the agent, a file, or the cache.
func (m *resmgr) loadInitialConfig() error {
	//
	// Notes/TODO:
	//
	//   If the agent is already up and running when cri-resmgr is being started the
	//   first configuration update will be polled using GetConfig(), unlike latter
	//   updates which are pushed by the agent. Since there is no way to report about
	//   problems with a polled configuration the agent will not know about problems
	//   with the initial one...
	//

	m.Info("loading initial configuration...")

	if opt.ForceConfig != "" {
		m.Info("using forced configuration %s...", opt.ForceConfig)
		return pkgcfg.SetConfigFromFile(opt.ForceConfig)
	}

	m.Info("trying configuration from agent...")
	if conf, err := m.agent.GetConfig(1 * time.Second); err == nil {
		if err = pkgcfg.SetConfig(conf.Data); err == nil {
			m.conf = conf // save to be stored if we manage to start up
			return nil
		}
		m.Error("configuration from agent failed to apply: %v", err)
	}

	m.Info("trying saved configuration from cache...")
	if conf := m.cache.GetConfig(); conf != nil {
		err := pkgcfg.SetConfig(conf.Data)
		if err == nil {
			return nil
		}
		m.Error("configuration from cache failed to apply: %v", err)
	}

	if opt.FallbackConfig != "" {
		m.Info("trying fallback configuration %s...", opt.FallbackConfig)
		return pkgcfg.SetConfigFromFile(opt.FallbackConfig)
	}

	m.Warn("no initial configuration found")
	return nil
}
