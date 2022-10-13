// Copyright 2022 Intel Corporation. All Rights Reserved.
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
	"context"
	"time"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	config "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
)

const (
	agentTimeout = 1 * time.Second
)

// Load initial configuration on startup.
func (m *resmgr) loadInitialConfig() error {
	//
	// We try to load initial configuration from a number of sources:
	//
	//    1. use forced configuration file if we were given one
	//    2. use configuration from agent, if we can fetch it and it applies
	//    3. use last configuration stored in cache, if we have one and it applies
	//    4. use fallback configuration file if we were given one
	//    5. use empty/builtin default configuration, whatever that is...
	//
	// Notes/TODO:
	//   If the agent is already running at this point, the initial configuration is
	//   obtained by polling the agent via GetConfig(). Unlike for the latter updates
	//   which are pushed by the agent, there is currently no way to report problems
	//   about polled configuration back to the agent. If/once the agent will have a
	//   mechanism to propagate configuration errors back to the origin, this might
	//   become a problem that we'll need to solve.
	//

	var (
		ok  bool
		err error
	)

	if ok, err = m.loadForcedConfig(); ok {
		m.Info("loaded configuration from forced config file")
		return nil
	}
	if err != nil {
		return err
	}

	if ok, err = m.loadAgentConfig(); ok {
		m.Info("loaded configuration from agent")
		return nil
	}
	if err != nil {
		m.Warn("failed to load configuration from agent: %v", err)
	}

	if ok, err = m.loadCachedConfig(); ok {
		m.Info("loaded last saved configuration from cache")
		return nil
	}
	if err != nil {
		m.Warn("failed to load saved configuration from cache: %v", err)
	}

	if ok, err = m.loadFallbackConfig(); ok {
		m.Info("loaded configuration from fallback config file")
		return nil
	}
	if err != nil {
		m.Warn("failed to load configuration from fallback config file: %v", err)
	}

	if ok, err = m.loadBuiltinDefaultConfig(); ok {
		m.Info("loaded builtin default configuration")
		return nil
	}
	if err != nil {
		m.Warn("failed to load builtin default configuration: %v", err)
	}

	return resmgrError("could not load any usable configuration")
}

// Update configuration to the given one.
func (m *resmgr) updateConfig(v interface{}) error {
	m.Lock()
	defer m.Unlock()

	old, err := pkgcfg.GetYAML()
	if err != nil {
		return resmgrError("failed to query current configuration: %w", err)
	}

	err = m.activateConfig("updated", v)
	if err != nil {
		err = m.activateConfig("reverted", old)
		if err != nil {
			m.Error("failed to revert to existing configuration: %v", err)
		}

		return err
	}

	return nil
}

// Activate the given configuration configuration.
func (m *resmgr) activateConfig(kind string, v interface{}) error {
	var err error

	switch cfg := v.(type) {
	case *config.RawConfig:
		err = pkgcfg.SetFromConfigMap(cfg.Data)
	case string:
		err = pkgcfg.SetFromFile(cfg)
	case []byte:
		err = pkgcfg.SetYAML(cfg)
	default:
		err = resmgrError("%s configuration: unknown type %T", kind, v)
	}

	if err != nil {
		return resmgrError("%s configuration rejected: %w", kind, err)
	}

	if m.policy != nil && !m.policy.Bypassed() {
		err = m.policy.UpdateConfig()
		if err != nil {
			return resmgrError("failed to activate %s configuration: %w", kind, err)
		}

		err = m.control.UpdateConfig()
		if err != nil {
			return resmgrError("failed to activate %s configuration: %w", kind, err)
		}

		err = m.runPostUpdateHooks(context.Background(), "updateConfig")
		if err != nil {
			return resmgrError("post-update hooks failed for %s configuration: %w", kind, err)
		}
	}

	if cfg, ok := v.(*config.RawConfig); ok {
		m.cache.SetConfig(cfg)
	}

	return nil
}

// Load configuration from a forced configuration file.
func (m *resmgr) loadForcedConfig() (bool, error) {
	if opt.ForceConfig == "" {
		return false, nil
	}

	err := pkgcfg.SetFromFile(opt.ForceConfig)
	if err != nil {
		return false, resmgrError("failed to load %s: %w", opt.ForceConfig, err)
	}

	err = m.setupConfigSignal(opt.ForceConfigSignal)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Load configuration from the agent.
func (m *resmgr) loadAgentConfig() (bool, error) {
	if m.agent.IsDisabled() {
		return false, nil
	}

	conf, err := m.agent.GetConfig(agentTimeout)
	if err != nil {
		return false, err
	}

	err = pkgcfg.SetFromConfigMap(conf.Data)
	if err != nil {
		return false, resmgrError("failed to load configuration from agent: %w", err)
	}

	return true, nil
}

// Load saved configuration from cache.
func (m *resmgr) loadCachedConfig() (bool, error) {
	conf := m.cache.GetConfig()
	if conf == nil {
		return false, nil
	}

	err := pkgcfg.SetFromConfigMap(conf.Data)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Load configuration from fallback file.
func (m *resmgr) loadFallbackConfig() (bool, error) {
	if opt.FallbackConfig == "" {
		return false, nil
	}

	err := pkgcfg.SetFromFile(opt.FallbackConfig)
	if err != nil {
		return false, resmgrError("failed to load fallback configuration %s: %w",
			opt.FallbackConfig, err)
	}

	return true, nil
}

// Load builtin default configuration.
func (m *resmgr) loadBuiltinDefaultConfig() (bool, error) {
	emptyConfig := map[string]string{}

	if err := pkgcfg.SetFromConfigMap(emptyConfig); err != nil {
		return false, resmgrError("failed to load empty builtin default config: %w", err)
	}

	return true, nil
}
