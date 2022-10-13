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

package stp

import (
	"fmt"
	"io/ioutil"
	"path"
	"regexp"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
)

// config captures our runtime configurable parameters.
type config struct {
	// Pools defines our set of pools in use.
	Pools pools `json:"pools,omitempty"`
	// ConfDirPath is the filesystem path to the legacy configuration directry structure.
	ConfDirPath string
	// ConfFilePath is the filesystem path to the legacy configuration file.
	ConfFilePath string
	// LabelNode controls whether backwards-compatible CMK node label is created.
	LabelNode bool
	// TaintNode controls whether backwards-compatible CMK node taint is created.
	TaintNode bool
}

type pools map[string]poolConfig

type cpuList struct {
	Socket     uint64
	Cpuset     string // TODO: might want to use cpuset from kubelet
	containers map[string]struct{}
}

// STP policy runtime configuration with their defaults.
var conf = defaultConfig().(*config)

// defaultConfig returns a new conf instance, all initialized to defaults.
func defaultConfig() interface{} {
	return &config{
		Pools:       make(pools),
		ConfDirPath: "/etc/cmk",
	}
}

func (c *cpuList) addContainer(id string) {
	if c.containers == nil {
		c.containers = make(map[string]struct{})
	}
	c.containers[id] = struct{}{}
}

func (c *cpuList) removeContainer(id string) {
	if c.containers == nil {
		return
	}
	delete(c.containers, id)
}

func (c *cpuList) getContainers() []string {
	if c.containers == nil {
		return []string{}
	}

	ret := make([]string, len(c.containers))
	i := 0
	for k := range c.containers {
		ret[i] = k
		i++
	}
	return ret
}

type poolConfig struct {
	Exclusive bool `json:"exclusive"`
	// Per-socket cpu lists
	CPULists []*cpuList `json:"cpuLists"`
}

func (p *poolConfig) cpuSet() string {
	cpuset := ""
	delim := ""
	for _, cl := range p.CPULists {
		cpuset += delim + cl.Cpuset
		delim = ","
	}
	return cpuset
}

var (
	cpusetValidationRe = regexp.MustCompile(`^(([\d]+)|([\d]+-[\d]+))(,(([\d]+)|([\d]+-[\d]+)))*$`)
)

func parseConfData(raw []byte) (pools, error) {
	conf := &struct {
		Pools pools
	}{}

	err := yaml.Unmarshal(raw, &conf)
	if err != nil {
		return nil, stpError("Failed to parse config file: %v", err)
	}
	return conf.Pools, nil
}

func readConfFile(filepath string) (pools, error) {
	// Read config data
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, stpError("Failed to read config file: %v", err)
	}

	return parseConfData(data)
}

func readConfDir(confDir string) (pools, error) {
	conf := pools{}

	// List pools in the pools configuration directory
	poolsDir := path.Join(confDir, "pools")
	pools, err := ioutil.ReadDir(poolsDir)
	if err != nil {
		return nil, stpError("Failed to list pools config directory %s: %v", poolsDir, err)
	}

	// Read pool configurations
	for _, pool := range pools {
		poolConf, err := readPoolConfDir(path.Join(poolsDir, pool.Name()))
		if err != nil {
			return nil, stpError("Failed to read pool çonfiguration: %v", err)
		}
		conf[pool.Name()] = poolConf
	}

	return conf, nil
}

// Read configuration of one pool from original CMK configuration directory tree
func readPoolConfDir(poolDir string) (poolConfig, error) {
	conf := poolConfig{Exclusive: false, CPULists: []*cpuList{}}

	// Read pool's exclusivity flag
	exclusive, err := ioutil.ReadFile(path.Join(poolDir, "exclusive"))
	if err != nil {
		return conf, fmt.Errorf("Failed to read pool exclusive setting in %s: %v", poolDir, err)
	}
	if len(exclusive) == 1 && exclusive[0] == '1' {
		conf.Exclusive = true
	}

	// Read socket configurations (per-socket cpu lists)
	files, err := ioutil.ReadDir(poolDir)
	if err != nil {
		return conf, fmt.Errorf("Failed to list pool config directory %s: %v", poolDir, err)
	}
	for _, file := range files {
		if !file.IsDir() {
			// Skip non-directory files (e.g. 'exclusive' file)
			continue
		}

		socketPath := path.Join(poolDir, file.Name())
		socketCPULists, err := readSocketConfDir(socketPath)
		if err != nil {
			return conf, fmt.Errorf("Failed to list pool socket config: %s", err)
		}

		conf.CPULists = append(conf.CPULists, socketCPULists...)
	}
	return conf, nil
}

// Read configuration (cpu lists) of a socket of one pool in original CMK
// configuration directory tree
func readSocketConfDir(socketDir string) ([]*cpuList, error) {
	// Get socket number from the name of the directory
	socketNum, err := strconv.ParseUint(path.Base(socketDir), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("Invalid socket id %s: %v", socketDir, err)
	}

	// Socket directory contains a set of subdirectories, one per cpu list
	cpuListDirs, err := ioutil.ReadDir(socketDir)
	if err != nil {
		return nil, fmt.Errorf("Failed to list socket directory %s: %v", socketDir, err)
	}

	conf := make([]*cpuList, len(cpuListDirs))

	for i, cpuListDir := range cpuListDirs {
		// Validate that the cpulist conforms to cpuset formatting
		if err := validateCPUList(cpuListDir.Name()); err != nil {
			return nil, fmt.Errorf("Invalid cpu list in %s: %v", socketDir, err)
		}
		conf[i] = &cpuList{Socket: socketNum,
			Cpuset:     cpuListDir.Name(),
			containers: map[string]struct{}{}}
	}
	return conf, nil
}

func validateCPUList(name string) error {
	// NOTE: Naive implementation, we only check that it "looks right", we don't
	// check that the actual numbers make sense, i.e. that numbers are in
	// ascending order
	if !cpusetValidationRe.MatchString(name) {
		return fmt.Errorf("%q does not look like a cpuset", name)
	}
	return nil
}

// Convert cpu list configuration directory name into a cpuList
func parseCPUListName(name string) ([]uint, error) {
	// The name should be a list of cpu ids (positive integers) separated by commas
	cpuListMembers := strings.Split(name, ",")

	cpus := make([]uint, len(cpuListMembers))

	// Convert cpu ids to a list of integers
	for i, cpuStr := range cpuListMembers {
		cpu, err := strconv.ParseUint(cpuStr, 10, 32)
		if err != nil {
			return cpus, fmt.Errorf("Invalid cpu id in %s: %v", name, err)
		}
		cpus[i] = uint(cpu)
	}
	return cpus, nil
}

const (
	// ConfigDescription describes our configuration fragment.
	ConfigDescription = PolicyDescription // XXX TODO
)

func (c *config) Describe() string {
	return PolicyDescription
}

func (c *config) Reset() {
	*c = config{
		Pools:       make(pools),
		ConfDirPath: "/etc/cmk",
	}
}

func (c *config) Validate() error {
	// XXX TODO
	log.Warn("*** Implement semantic validation for %q, or remove this.", ConfigDescription)
	return nil
}

// Register us for command line option processing and configuration management.
func init() {
	pkgcfg.Register(PolicyPath, conf)
}
