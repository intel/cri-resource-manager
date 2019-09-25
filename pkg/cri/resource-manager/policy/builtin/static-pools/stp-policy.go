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

package stp

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/agent"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// PolicyName is the symbol used to pull us in as a builtin policy.
	PolicyName = "static-pools"
	// PolicyDescription is a short description of this policy.
	PolicyDescription = "A reimplementation of CMK (CPU Manager for Kubernetes)."
	// StpEnvPool is the name of the env variable for selecting STP pool of a container
	StpEnvPool = "STP_POOL"
	// StpEnvSocketID is the name of the env variable for selecting cpu socket of a container
	StpEnvSocketID = "STP_SOCKET_ID"
	// StpEnvNoAffinity is the name of the env variable for switching off cpuset enforcement
	StpEnvNoAffinity = "STP_NO_AFFINITY"
	// CmkEnvAssigned is the name of the env variable that the original CMK
	// sets to communicate the selected cpuset to the workload. We use the same
	// environment variable for compatibility.
	CmkEnvAssigned = "CMK_CPUS_ASSIGNED"
	// CmkEnvNumCores is the name of the env used in the original CMK to select
	// the number of exclusive CPUs, deprecated here
	CmkEnvNumCores = "CMK_NUM_CORES"
)

type stp struct {
	logger.Logger

	conf  *config         // STP policy configuration
	state cache.Cache     // state cache
	agent agent.Interface // client connection to cri-resmgr agent gRPC server
}

var _ policy.Backend = &stp{}

// Dump json-compatible struct in human-readable form
func stringify(r interface{}) string {
	out, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Sprintf("!!!!!\nUnable to stringify %T: %v\n!!!!!", r, err)
	}
	return string(out)
}

//
// Policy backend implementation
//

// CreateStpPolicy creates a new policy instance.
func CreateStpPolicy(opts *policy.Options) policy.Backend {
	var err error
	stp := &stp{Logger: logger.NewLogger(PolicyName), agent: opts.AgentCli}

	stp.Info("creating policy...")

	// Read STP configuration
	if len(opts.Config) > 0 {
		stp.Info("using policy config from cri-resmgr configuration")
		stp.conf, err = parseConfData([]byte(opts.Config))
		if err != nil {
			stp.Warn("failed to parse config: %v", err)
		}
	} else {
		if len(opt.confDir) > 0 {
			stp.conf, err = readConfDir(opt.confDir)
			if err != nil {
				stp.Warn("failed to read configuration directory: %v", err)
			}
		}
		if len(opt.confFile) > 0 {
			if stp.conf != nil {
				stp.Info("Overriding configuration from -static-pools-conf-dir with -static-pools-conf-file")
			}
			stp.conf, err = readConfFile(opt.confFile)
			if err != nil {
				stp.Warn("failed to read configuration directory: %v", err)
			}
		}
	}
	if stp.conf == nil {
		stp.Fatal("No STP policy configuration loaded")
	}

	stp.Debug("policy configuration:\n%s", stringify(stp.conf))

	return stp
}

// Name returns the name of this policy.
func (stp *stp) Name() string {
	return PolicyName
}

// Description returns the description for this policy.
func (stp *stp) Description() string {
	return PolicyDescription
}

// Start prepares this policy for accepting allocation/release requests.
func (stp *stp) Start(cch cache.Cache, add []cache.Container, del []cache.Container) error {
	var err error

	err = stp.updateNode(*stp.conf)
	if err != nil {
		stp.Fatal("%v", err)
	}

	if err := stp.initializeState(cch); err != nil {
		return err
	}
	stp.Debug("retrieved stp container states from cache:\n%s", stringify(*stp.getContainerRegistry()))

	if err = stp.Sync(add, del); err != nil {
		return err
	}

	stp.Debug("preparing for making decisions...")

	return nil
}

// Sync synchronizes the state of this policy.
func (stp *stp) Sync(add []cache.Container, del []cache.Container) error {
	stp.Debug("synchronizing state...")
	for _, c := range del {
		stp.ReleaseResources(c)
	}
	for _, c := range add {
		stp.AllocateResources(c)
	}

	return nil
}

// AllocateResources is a resource allocation request for this policy.
func (stp *stp) AllocateResources(c cache.Container) error {
	containerID := c.GetCacheID()
	stp.Debug("allocating resources for container %s...", containerID)

	cs := stpContainerStatus{Socket: -1}

	// Default pool name
	poolName := "shared"

	// Get resource requests
	stp.Debug("RESOURCE REQUESTS: %s", c.GetResourceRequirements().Requests)
	requestedCPUs, ok := c.GetResourceRequirements().Requests[exclusiveCoreResourceName]
	if ok {
		nCPUs, _ := requestedCPUs.AsInt64()
		cs.NExclusiveCPUs = nCPUs
	}

	// Parse container command line. Backwards compatibility for old CMK
	// workloads
	cmkArgs := stp.parseContainerCmdline(c.GetCommand(), c.GetArgs())
	if cmkArgs != nil {
		poolName = cmkArgs.Pool
		cs.Socket = cmkArgs.SocketID
		cs.NoAffinity = cmkArgs.NoAffinity

		// Overwrite container commandline
		c.SetCommand(cmkArgs.Command)
		c.SetArgs([]string{})

		stp.Debug("parsed options from container command line: %v", cmkArgs)
	}

	// Get STP options from container env
	envVal, ok := c.GetEnv(StpEnvSocketID)
	if ok {
		socketID, err := strconv.ParseInt(envVal, 10, 32)
		if err != nil {
			stp.Warn("unable to parse socket id from %q: %v", StpEnvSocketID, err)
		} else {
			cs.Socket = socketID
		}
	}
	envVal, ok = c.GetEnv(StpEnvPool)
	if ok {
		poolName = envVal
	}
	_, ok = c.GetEnv(StpEnvNoAffinity)
	if ok {
		// We do not care about the value of the env variable here
		cs.NoAffinity = true
	}

	// Force socket to -1 if pool is not "socket aware"
	if poolName == "infra" {
		cs.Socket = -1
	}

	// Get pool configuration
	if _, ok := stp.conf.Pools[poolName]; !ok {
		return stpError("non-existent pool %q", poolName)
	}
	cs.Pool = poolName

	// Allocate (CPU) resources for the container
	err := stp.allocateStpResources(c, cs)
	if err != nil {
		return err
	}

	return nil
}

// ReleaseResources is a resource release request for this policy.
func (stp *stp) ReleaseResources(c cache.Container) error {
	stp.Debug("releasing resources of container %s...", c.PrettyName())
	stp.releaseStpResources(c.GetCacheID())
	return nil
}

// UpdateResources is a resource allocation update request for this policy.
func (stp *stp) UpdateResources(c cache.Container) error {
	stp.Debug("updating resource allocations of container %s...", c.PrettyName())
	return nil
}

// ExportResourceData provides resource data to export for the container.
func (stp *stp) ExportResourceData(c cache.Container, syntax policy.DataSyntax) []byte {
	return nil
}

func (stp *stp) PostStart(cch cache.Container) error {
	return nil
}

// SetConfig sets the policy backend configuration
func (stp *stp) SetConfig(conf string) error {
	// Unserialize
	newConf, err := parseConfData([]byte(conf))
	if err != nil {
		return stpError("failed to parse config: %v", err)
	}

	if err = stp.verifyConfig(newConf); err != nil {
		return err
	}

	stp.Info("config updated successfully")
	stp.conf = newConf
	stp.Debug("new policy configuration:\n%s", stringify(stp.conf))
	return nil
}

//
// Helper functions for STP policy backend
//

func stpError(format string, args ...interface{}) error {
	return fmt.Errorf(PolicyName+": "+format, args...)
}

func (stp *stp) initializeState(state cache.Cache) error {
	stp.state = state

	ccr := stp.getContainerRegistry()

	for id := range *ccr {
		// Remove orphaned containers
		if _, ok := stp.state.LookupContainer(id); !ok {
			stp.Info("removing orphaned container %s from policy cache", id)
			stp.releaseStpResources(id)
		}
	}

	return stp.verifyConfig(stp.conf)
}

// Verify configuration against the existing set of containers
func (stp *stp) verifyConfig(conf *config) error {
	//  Sanity check for config
	if conf == nil || conf.Pools == nil {
		return stpError("invalid config, no pools configured")
	}

	// Loop through all existing containers
	ccr := stp.getContainerRegistry()
	for id, cs := range *ccr {
		// Check that pool for container exists
		pool, ok := conf.Pools[cs.Pool]
		if !ok {
			return stpError("invalid stp configuration: pool %q for container %q not found", cs.Pool, id)
		}

		// Check that pool exclusivity is compatible with container configuration
		if pool.Exclusive && cs.NExclusiveCPUs < 1 {
			return stpError("invalid stp configuration: container %q with no exclusive CPUs set to run in exclusive pool %q", id, cs.Pool)
		} else if !pool.Exclusive && cs.NExclusiveCPUs > 0 {
			return stpError("invalid stp configuration: container %q with exclusive CPUs set to run in non-exclusive pool %q", id, cs.Pool)
		}

		// Check that cpu lists (cpuset) of container can be satisfied by the pool
		// NOTE: we do not try to do any migration to possibly free cpu lists
		// if the originally allocated cpu lists are not available
		// TODO: for non-exclusive pools it might be feasible just to alter the
		// cpuset (i.e. reconcile new cpu list using the existing pool/socket
		// spec for container) in case cpu lists do not match exactly
		for _, cCpuset := range cs.Cpusets {
			for i, pClist := range pool.CPULists {
				if cCpuset == pClist.Cpuset {
					pool.CPULists[i].addContainer(id)
					break
				}
				if i == len(pool.CPULists)-1 {
					return stpError("invalid stp configuration: cpu list %q configured for container %q not found in pool %q", cCpuset, id, cs.Pool)
				}
			}
		}
	}

	return nil
}

type cmkLegacyArgs struct {
	Pool       string
	SocketID   int64
	Command    []string
	NoAffinity bool
}

// parseContainerCmdline tries to parse the pool name and socket id parameters
// from container command line
func (stp *stp) parseContainerCmdline(cmd, args []string) *cmkLegacyArgs {
	// NOTE: This is naive implementation and not foolproof. E.g. args could be
	// defined throught env variables
	cmdLine := append(cmd, args...)
	stp.Debug("Parsing container command line %v\n", cmdLine)

	cmkArgs := parseCmkCmdline(cmdLine)

	// If we didn't find cmk arguments, try to parse each argument separately
	// in case cmk was invoked like 'bash -c "cmk isolate ..."
	// NOTE: We do somewhat naive strings.Fields() here, there is room for
	// improvement by usage go-shellquote or similar
	if cmkArgs == nil {
		for _, arg := range cmdLine {
			cmkArgs = parseCmkCmdline(strings.Fields(arg))
			if cmkArgs != nil {
				break
			}
		}
	}
	return cmkArgs
}

func parseCmkCmdline(args []string) *cmkLegacyArgs {
	parsedArgs := cmkLegacyArgs{}

	// Create parser
	cmkCmd := flag.NewFlagSet("cmk-legacy", flag.ContinueOnError)
	cmkCmd.SetOutput(ioutil.Discard)
	cmkCmd.StringVar(&parsedArgs.Pool, "pool", "", "pool to use")
	cmkCmd.Int64Var(&parsedArgs.SocketID, "socket-id", -1, "socket id to use")
	cmkCmd.BoolVar(&parsedArgs.NoAffinity, "no-affinity", false, "Do not set cpu affinity before forking the child command")
	// Args that we're not really interested in
	_ = cmkCmd.String("conf-dir", "", "CMK configuration directory")

	if len(args) > 1 && args[1] == "isolate" {
		err := cmkCmd.Parse(args[2:])
		// Parse out (i.e. ignore) all unknown args
		for err != nil {
			err = cmkCmd.Parse(cmkCmd.Args())
		}
		// Pool needs to be defined
		if parsedArgs.Pool != "" {
			parsedArgs.Command = cmkCmd.Args()
			return &parsedArgs
		}
	}
	return nil
}

func (stp *stp) allocateStpResources(c cache.Container, cs stpContainerStatus) error {
	var CPULists [](*cpuList)

	// Get pool configuration for this container
	pool, ok := stp.conf.Pools[cs.Pool]
	if !ok {
		return stpError("BUG: pool %q not found", cs.Pool)
	}

	availableCPULists := getAvailableCPULists(cs.Socket, &pool)

	if pool.Exclusive {
		if cs.NExclusiveCPUs < 1 {
			return stpError("exclusive pool specified but the number of exclusive CPUs requested is 0")
		}

		// Check the possible deprecated CMK_NUM_CORES setting. Print a warning
		// if this does not match what was requested through extended resources
		envNumCores, ok := c.GetEnv(CmkEnvNumCores)
		if ok {
			iNumCores, err := strconv.ParseInt(envNumCores, 10, 64)
			if err != nil || iNumCores != cs.NExclusiveCPUs {
				stp.Warn("Ignoring deprecated env variable setting, %s=%q does "+
					"not match the number of cores (%d) from resource request",
					CmkEnvNumCores, envNumCores, cs.NExclusiveCPUs)
			}
		}

		if int64(len(availableCPULists)) < cs.NExclusiveCPUs {
			if cs.Socket < 0 {
				return stpError("not enough free cpu lists in pool %q", cs.Pool)
			}
			return stpError("not enough free cpu lists in pool %q with socket id %d", cs.Pool, cs.Socket)
		}

		CPULists = availableCPULists[0:cs.NExclusiveCPUs]

	} else {
		/* NOTE (from CMK): This allocation algorithm is probably an
		oversimplification, however for known use cases the non-exclusive
		pools should never have more than one cpu list anyhow.
		If that ceases to hold in the future, we could explore population
		or load-based spreading. Keeping it simple for now. */
		if len(availableCPULists) == 0 {
			return stpError("no available cpu lists in pool %q with socket id %d", cs.Pool, cs.Socket)
		}

		i := rand.Int31n(int32((len(availableCPULists))))
		CPULists = availableCPULists[i : i+1]
	}

	containerID := c.GetCacheID()
	cpuset := ""
	sep := ""
	for _, cl := range CPULists {
		cl.addContainer(containerID)
		cpuset += sep + cl.Cpuset
		sep = ","
		cs.Cpusets = append(cs.Cpusets, cpuset)
	}

	// Commit our changes
	containers := stp.getContainerRegistry()
	(*containers)[containerID] = cs
	stp.setContainerRegistry(containers)

	if cs.NoAffinity {
		stp.Info("not setting cpuset for container  %q as --no-affinity was specified", containerID)
	} else {
		stp.Info("setting cpuset of container %q to %q", containerID, cpuset)
		c.SetCpusetCpus(cpuset)
	}

	c.SetEnv(CmkEnvAssigned, cpuset)

	return nil
}

// getAvailableCPULists Constructa a list of available cpu lists that satisfy
// the possible socket constraint
func getAvailableCPULists(socket int64, pool *poolConfig) [](*cpuList) {
	availableCPULists := make([](*cpuList), 0, len(pool.CPULists))
	for _, c := range pool.CPULists {
		if socket < 0 || socket == int64(c.Socket) {
			if pool.Exclusive && len(c.getContainers()) > 0 {
				continue
			}
			availableCPULists = append(availableCPULists, c)
		}
	}
	return availableCPULists
}

func (stp *stp) releaseStpResources(containerID string) error {
	ccr := *stp.getContainerRegistry()
	if cs, ok := ccr[containerID]; ok {
		pool, ok := stp.conf.Pools[cs.Pool]
		if !ok {
			return stpError("BUG: pool %q for container %q not found", cs.Pool, containerID)
		}
		for _, clist := range pool.CPULists {
			clist.removeContainer(containerID)
		}
		delete(ccr, containerID)

		// Commit our changes to stp cache
		stp.setContainerRegistry(&ccr)
	}

	return nil
}

//
// Handling of cached data
//

const (
	cacheKeyContainerRegistry = "ContainerRegistry"
)

type stpContainerStatus struct {
	Pool           string   // pool configuration
	Socket         int64    // physical socket id
	NExclusiveCPUs int64    // number of exclusive cpus
	Cpusets        []string // cpusets (cpu lists) assigned to this container
	NoAffinity     bool     // disable cpuset enforcing
}

// stpContainerCache contains STP-specific data of containers
type stpContainerCache map[string]stpContainerStatus

// Set the value of cached cachableContainerRegistry object
func (c *stpContainerCache) Set(value interface{}) {
	switch value.(type) {
	case stpContainerCache:
		*c = value.(stpContainerCache)
	case *stpContainerCache:
		cp := value.(*stpContainerCache)
		*c = *cp
	}
}

// Get the cached cachableContainerRegistry object
func (c *stpContainerCache) Get() interface{} {
	return *c
}

// getContainerRegistry gets the current state of our container registry
func (stp *stp) getContainerRegistry() *stpContainerCache {
	ccr := &stpContainerCache{}

	if !stp.state.GetPolicyEntry(cacheKeyContainerRegistry, ccr) {
		stp.Error("no cached container registry found")
	}

	return ccr
}

// setContainerRegistry caches the state of our container registry
func (stp *stp) setContainerRegistry(ccr *stpContainerCache) {
	stp.state.SetPolicyEntry(cacheKeyContainerRegistry, cache.Cachable(ccr))
}

//
// Automatically register us as a policy implementation.
//

// Implementation is the implementation we register with the policy module.
type Implementation func(*policy.Options) policy.Backend

// Name returns the name of this policy implementation.
func (i Implementation) Name() string {
	return PolicyName
}

// Description returns the desccription of this policy implementation.
func (i Implementation) Description() string {
	return PolicyDescription
}

// CreateFn returns the functions used to instantiate this policy.
func (i Implementation) CreateFn() policy.CreateFn {
	return policy.CreateFn(i)
}

var _ policy.Implementation = Implementation(nil)

func init() {
	policy.Register(Implementation(CreateStpPolicy))

	rand.Seed(time.Now().Unix())
}
