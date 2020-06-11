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

package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/kubernetes"
	logger "github.com/intel/cri-resource-manager/pkg/log"
	"github.com/intel/cri-resource-manager/pkg/metricsring"
	"github.com/intel/cri-resource-manager/pkg/topology"
)

const (
	// CRI marks changes that can be applied by the CRI controller.
	CRI = "cri"
	// RDT marks changes that can be applied by the RDT controller.
	RDT = "rdt"
	// BlockIO marks changes that can be applied by the BlockIO controller.
	BlockIO = "blockio"

	// TagAVX512 tags containers that use AVX512 instructions.
	TagAVX512 = "AVX512"

	// RDTClassKey is the pod annotation key for specifying a container RDT class.
	RDTClassKey = "rdtclass" + "." + kubernetes.ResmgrKeyNamespace
	// BlockIOClassKey is the pod annotation key for specifying a container Block I/O class.
	BlockIOClassKey = "blockioclass" + "." + kubernetes.ResmgrKeyNamespace
)

// PodState is the pod state in the runtime.
type PodState int32

const (
	// PodStateReady marks a pod ready.
	PodStateReady = PodState(int32(cri.PodSandboxState_SANDBOX_READY))
	// PodStateNotReady marks a pod as not ready.
	PodStateNotReady = PodState(int32(cri.PodSandboxState_SANDBOX_NOTREADY))
	// PodStateStale marks a pod as removed.
	PodStateStale = PodState(int32(PodStateNotReady) + 1)
)

// PodResourceRequirements are per container resource requirements, annotated by our webhook.
type PodResourceRequirements struct {
	// InitContainers is the resource requirements by init containers.
	InitContainers map[string]v1.ResourceRequirements `json:"initContainers"`
	// Containers is the resource requirements by normal container.
	Containers map[string]v1.ResourceRequirements `json:"containers"`
}

// Pod is the exposed interface from a cached pod.
type Pod interface {
	// GetInitContainers returns the init containers of the pod.
	GetInitContainers() []Container
	// GetContainers returns the (non-init) containers of the pod.
	GetContainers() []Container
	// GetContainer returns the named container of the pod.
	GetContainer(string) (Container, bool)
	// GetId returns the pod id of the pod.
	GetID() string
	// GetUID returns the (kubernetes) unique id of the pod.
	GetUID() string
	// GetName returns the name of the pod.
	GetName() string
	// GetNamespace returns the namespace of the pod.
	GetNamespace() string
	// GetState returns the PodState of the pod.
	GetState() PodState
	// GetQOSClass returns the PodQOSClass of the pod.
	GetQOSClass() v1.PodQOSClass
	// GetLabelKeys returns the keys of all pod labels as a string slice.
	GetLabelKeys() []string
	// GetLabel returns the value of the given label and whether it was found.
	GetLabel(string) (string, bool)
	// GetResmgrLabelKeys returns pod label keys (without the namespace
	// part) in cri-resource-manager namespace.
	GetResmgrLabelKeys() []string
	// GetResmgrLabel returns the value of a pod label from the
	// cri-resource-manager namespace.
	GetResmgrLabel(string) (string, bool)
	// GetAnnotationKeys returns the keys of all pod annotations as a string slice.
	GetAnnotationKeys() []string
	// GetAnnotation returns the value of the given annotation and whether it was found.
	GetAnnotation(key string) (string, bool)
	// GetAnnotationObject decodes the value of the given annotation with the given function.
	GetAnnotationObject(key string, objPtr interface{},
		decode func([]byte, interface{}) error) (bool, error)
	// GetResmgrAnnotationKeys returns pod annotation keys (without the
	// namespace part) in cri-resource-manager namespace as a string slice.
	GetResmgrAnnotationKeys() []string
	// GetAnnotation returns the value of a pod annotation from the
	// cri-resource-manager namespace and whether it was found.
	GetResmgrAnnotation(key string) (string, bool)
	// GetResmgrAnnotationObject decodes the value of the given annotation in the
	// cri-resource-manager namespace.
	GetResmgrAnnotationObject(key string, objPtr interface{},
		decode func([]byte, interface{}) error) (bool, error)
	// GetEffectiveAnnotation returns the effective annotation for a container.
	// For any given key $K and container $C it will look for annotations in
	// this order and return the first one found:
	//     $K/container.$C
	//     $K/pod
	//     $K
	// and return the value of the first key found.
	GetEffectiveAnnotation(key, container string) (string, bool)
	// GetCgroupParentDir returns the pods cgroup parent directory.
	GetCgroupParentDir() string
	// GetPodResourceRequirements returns container resource requirements if the
	// necessary associated annotation put in place by the CRI resource manager
	// webhook was found.
	GetPodResourceRequirements() PodResourceRequirements
	// GetContainerAffinity returns the affinity expressions for the named container.
	GetContainerAffinity(string) []*Affinity
	// ScopeExpression returns an affinity expression for defining this pod as the scope.
	ScopeExpression() *Expression
}

// A cached pod.
type pod struct {
	cache        *cache            // our cache of object
	ID           string            // pod sandbox runtime id
	UID          string            // (k8s) unique id
	Name         string            // pod sandbox name
	Namespace    string            // pod namespace
	State        PodState          // ready/not ready
	QOSClass     v1.PodQOSClass    // pod QoS class
	Labels       map[string]string // pod labels
	Annotations  map[string]string // pod annotations
	CgroupParent string            // cgroup parent directory
	containers   map[string]string // container name to ID map

	Resources *PodResourceRequirements // annotated resource requirements
	Affinity  *podContainerAffinity    // annotated container affinity
}

// ContainerState is the container state in the runtime.
type ContainerState int32

const (
	// ContainerStateCreated marks a container created, not running.
	ContainerStateCreated = ContainerState(int32(cri.ContainerState_CONTAINER_CREATED))
	// ContainerStateRunning marks a container created, running.
	ContainerStateRunning = ContainerState(int32(cri.ContainerState_CONTAINER_RUNNING))
	// ContainerStateExited marks a container exited.
	ContainerStateExited = ContainerState(int32(cri.ContainerState_CONTAINER_EXITED))
	// ContainerStateUnknown marks a container to be in an unknown state.
	ContainerStateUnknown = ContainerState(int32(cri.ContainerState_CONTAINER_UNKNOWN))
	// ContainerStateCreating marks a container as being created.
	ContainerStateCreating = ContainerState(int32(ContainerStateUnknown) + 1)
	// ContainerStateStale marks a container removed.
	ContainerStateStale = ContainerState(int32(ContainerStateUnknown) + 2)
)

// Container is the exposed interface from a cached container.
type Container interface {
	// PrettyName returns the user-friendly <podname>:<containername> for the container.
	PrettyName() string
	// GetPod returns the pod of the container and a boolean indicating if there was one.
	GetPod() (Pod, bool)
	// GetID returns the ID of the container.
	GetID() string
	// GetPodID returns the pod ID of the container.
	GetPodID() string
	// GetCacheID returns the cacheID of the container.
	GetCacheID() string
	// GetName returns the name of the container.
	GetName() string
	// GetNamespace returns the namespace of the container.
	GetNamespace() string
	// UpdateState updates the state of the container.
	UpdateState(ContainerState)
	// GetState returns the ContainerState of the container.
	GetState() ContainerState
	// GetQOSClass returns the QoS class the pod would have if this was its only container.
	GetQOSClass() v1.PodQOSClass
	// GetImage returns the image of the container.
	GetImage() string
	// GetCommand returns the container command.
	GetCommand() []string
	// GetArgs returns the container command arguments.
	GetArgs() []string
	// GetLabelKeys returns the keys of all labels of the container.
	GetLabelKeys() []string
	// GetLabel returns the value of a container label.
	GetLabel(string) (string, bool)
	// GetLabels returns a copy of all container labels.
	GetLabels() map[string]string
	// GetResmgrLabelKeys returns container label keys (without the namespace
	// part) in cri-resource-manager namespace.
	GetResmgrLabelKeys() []string
	// GetResmgrLabel returns the value of a container label from the
	// cri-resource-manager namespace.
	GetResmgrLabel(string) (string, bool)
	// GetAnnotationKeys returns the keys of all annotations of the container.
	GetAnnotationKeys() []string
	// GetAnnotation returns the value of a container annotation.
	GetAnnotation(key string, objPtr interface{}) (string, bool)
	// GetResmgrAnnotationKeys returns container annotation keys (without the
	// namespace part) in cri-resource-manager namespace.
	GetResmgrAnnotationKeys() []string
	// GetAnnotation returns the value of a container annotation from the
	// cri-resource-manager namespace.
	GetResmgrAnnotation(key string, objPtr interface{}) (string, bool)
	// GetEffectiveAnnotation returns the effective annotation for the container from the pod.
	GetEffectiveAnnotation(key string) (string, bool)
	// GetAnnotations returns a copy of all container annotations.
	GetAnnotations() map[string]string
	// GetEnvKeys returns the keys of all container environment variables.
	GetEnvKeys() []string
	// GetEnv returns the value of a container environment variable.
	GetEnv(string) (string, bool)
	// GetMounts returns all the mounts of the container.
	GetMounts() []Mount
	// GetMountByHost returns the container path corresponding to the host path.
	// XXX We should remove this as is might not be unique.
	GetMountByHost(string) *Mount
	// GetmountByContainer returns the host path mounted to a container path.
	GetMountByContainer(string) *Mount
	// GetDevices returns the devices of the container.
	GetDevices() []Device
	// GetDeviceByHost returns the device for a host path.
	GetDeviceByHost(string) *Device
	// GetDeviceByContainer returns the device for a container path.
	GetDeviceByContainer(string) *Device
	// GetResourceRequirements returns the webhook-annotated requirements for ths container.
	GetResourceRequirements() v1.ResourceRequirements
	// GetLinuxResources returns the CRI linux resource request of the container.
	GetLinuxResources() *cri.LinuxContainerResources

	// SetCommand sets the container command.
	SetCommand([]string)
	// SetArgs sets the container command arguments.
	SetArgs([]string)
	// SetLabel sets the value for a container label.
	SetLabel(string, string)
	// DeleteLabel removes a container label.
	DeleteLabel(string)
	// SetAnnotation sets the value for a container annotation.
	SetAnnotation(string, string)
	// DeleteAnnotation removes a container annotation.
	DeleteAnnotation(string)
	// SetEnv sets a container environment variable.
	SetEnv(string, string)
	// UnsetEnv unsets a container environment variable.
	UnsetEnv(string)
	// InsertMount inserts a mount into the container.
	InsertMount(*Mount)
	// DeleteMount removes a mount from the container.
	DeleteMount(string)
	// InsertDevice inserts a device into the container.
	InsertDevice(*Device)
	// DeleteDevice removes a device from the container.
	DeleteDevice(string)

	// Get any attached topology hints.
	GetTopologyHints() topology.Hints

	// GetCPUPeriod gets the CFS CPU period of the container.
	GetCPUPeriod() int64
	// GetCpuQuota gets the CFS CPU quota of the container.
	GetCPUQuota() int64
	// GetCPUShares gets the CFS CPU shares of the container.
	GetCPUShares() int64
	// GetmemoryLimit gets the memory limit in bytes for the container.
	GetMemoryLimit() int64
	// GetOomScoreAdj gets the OOM score adjustment for the container.
	GetOomScoreAdj() int64
	// GetCpusetCPUs gets the cgroup cpuset.cpus of the container.
	GetCpusetCpus() string
	// GetCpusetMems gets the cgroup cpuset.mems of the container.
	GetCpusetMems() string

	// SetLinuxResources sets the Linux-specific resource request of the container.
	SetLinuxResources(*cri.LinuxContainerResources)
	// SetCPUPeriod sets the CFS CPU period of the container.
	SetCPUPeriod(int64)
	// SetCPUQuota sets the CFS CPU quota of the container.
	SetCPUQuota(int64)
	// SetCPUShares sets the CFS CPU shares of the container.
	SetCPUShares(int64)
	// SetmemoryLimit sets the memory limit in bytes for the container.
	SetMemoryLimit(int64)
	// SetOomScoreAdj sets the OOM score adjustment for the container.
	SetOomScoreAdj(int64)
	// SetCpusetCpu sets the cgroup cpuset.cpus of the container.
	SetCpusetCpus(string)
	// SetCpusetMems sets the cgroup cpuset.mems of the container.
	SetCpusetMems(string)

	// GetAffinity returns the annotated affinity expressions for this container.
	GetAffinity() []*Affinity

	// SetRDTClass assigns this container to the given RDT class.
	SetRDTClass(string)
	// GetRDTClass returns the RDT class for this container.
	GetRDTClass() string

	// SetBlockIOClass assigns this container to the given BlockIO class.
	SetBlockIOClass(string)
	// GetBlockIOClass returns the BlockIO class for this container.
	GetBlockIOClass() string

	// SetCRIRequest sets the current pending CRI request of the container.
	SetCRIRequest(req interface{}) error
	// GetCRIRequest returns the current pending CRI request of the container.
	GetCRIRequest() (interface{}, bool)
	// ClearCRIRequest clears and returns the current pending CRI request of the container.
	ClearCRIRequest() (interface{}, bool)

	// GetCRIEnvs returns container environment variables.
	GetCRIEnvs() []*cri.KeyValue
	// GetCRIMounts returns container mounts.
	GetCRIMounts() []*cri.Mount
	// GetCRIDevices returns container devices.
	GetCRIDevices() []*cri.Device

	// GetPending gets the names of the controllers with pending changes.
	GetPending() []string
	// HasPending checks if the container has pending chanhes for the given controller.
	HasPending(string) bool
	// ClearPending clears the pending change marker for the given controller.
	ClearPending(string)

	// GetTag gets the value of the given tag.
	GetTag(string) (string, bool)
	// SetTag sets the value of the given tag and returns its previous value..
	SetTag(string, string) (string, bool)
	// DeleteTag deletes the given tag, returning its deleted value.
	DeleteTag(string) (string, bool)
}

// A cached container.
type container struct {
	cache         *cache             // our cache of objects
	ID            string             // container runtime id
	PodID         string             // associate pods runtime id
	CacheID       string             // our cache id
	Name          string             // container name
	Namespace     string             // container namespace
	State         ContainerState     // created/running/exited/unknown
	QOSClass      v1.PodQOSClass     // QoS class, if the container had one
	Image         string             // containers image
	Command       []string           // command to run in container
	Args          []string           // arguments for command
	Labels        map[string]string  // container labels
	Annotations   map[string]string  // container annotations
	Env           map[string]string  // environment variables
	Mounts        map[string]*Mount  // mounts
	Devices       map[string]*Device // devices
	TopologyHints topology.Hints     // Set of topology hints for all containers within Pod
	Tags          map[string]string  // container tags (local dynamic labels)

	Resources v1.ResourceRequirements      // container resources (from webhook annotation)
	LinuxReq  *cri.LinuxContainerResources // used to estimate Resources if we lack annotations
	req       *interface{}                 // pending CRI request

	RDTClass     string              // RDT class this container is assigned to.
	BlockIOClass string              // Block I/O class this container is assigned to.
	pending      map[string]struct{} // controllers with pending changes for this container

	prettyName string // cached PrettyName()
}

// MountType is a propagation type.
type MountType int32

const (
	// MountPrivate is a private container mount.
	MountPrivate MountType = MountType(cri.MountPropagation_PROPAGATION_PRIVATE)
	// MountHostToContainer is a host-to-container mount.
	MountHostToContainer MountType = MountType(cri.MountPropagation_PROPAGATION_HOST_TO_CONTAINER)
	// MountBidirectional is a bidirectional mount.
	MountBidirectional MountType = MountType(cri.MountPropagation_PROPAGATION_BIDIRECTIONAL)
)

// Mount is a filesystem entry mounted inside a container.
type Mount struct {
	// Container is the path inside the container.
	Container string
	// Host is the path on the host.
	Host string
	// Readonly specifies if the mount is read-only or read-write.
	Readonly bool
	// Relabels denotes SELinux relabeling.
	Relabel bool
	// Propagation identifies the mount propagation type.
	Propagation MountType
}

// Device is a device exposed to a container.
type Device struct {
	// Container is the device path inside the container.
	Container string
	// Host is the device path on the host side.
	Host string
	// Permissions specify the device permissions for the container.
	Permissions string
}

//
// Cachable is an interface opaque cachable data must implement.
type Cachable interface {
	// Set value (via a pointer receiver) to the object.
	Set(value interface{})
	// Get the object that should be cached.
	Get() interface{}
}

//
// Cache is the primary interface exposed for tracking pods and containers.
//
// Cache tracks pods and containers in the runtime, mostly by processing CRI
// requests and responses which the cache is fed as these are being procesed.
// Cache also saves its state upon changes to secondary storage and restores
// itself upon startup.
type Cache interface {
	// InsertPod inserts a pod into the cache, using a runtime request or reply.
	InsertPod(id string, msg interface{}) Pod
	// DeletePod deletes a pod from the cache.
	DeletePod(id string) Pod
	// LookupPod looks up a pod in the cache.
	LookupPod(id string) (Pod, bool)
	// InsertContainer inserts a container into the cache, using a runtime request or reply.
	InsertContainer(msg interface{}) (Container, error)
	// UpdateContainerID updates a containers runtime id.
	UpdateContainerID(cacheID string, msg interface{}) (Container, error)
	// DeleteContainer deletes a container from the cache.
	DeleteContainer(id string) Container
	// LookupContainer looks up a container in the cache.
	LookupContainer(id string) (Container, bool)
	// LookupContainerByCgroup looks up a container for the given cgroup path.
	LookupContainerByCgroup(path string) (Container, bool)

	// GetPendingContainers returs all containers with pending changes.
	GetPendingContainers() []Container

	// GetPods returns all the pods known to the cache.
	GetPods() []Pod
	// GetContainers returns all the containers known to the cache.
	GetContainers() []Container

	// GetContainerCacheIds returns the cache ids of all containers.
	GetContainerCacheIds() []string
	// GetContaineIds return the ids of all containers.
	GetContainerIds() []string
	GetMetrics() *Metricsdata
	SetMetrics(m *Metricsdata)

	// FilterScope returns the containers selected by the scope expression.
	FilterScope(*Expression) []Container
	// EvaluateAffinity evaluates the given affinity against all known in-scope containers
	EvaluateAffinity(*Affinity) map[string]int32
	// AddImplicitAffinities adds a set of implicit affinities (added to all containers).
	AddImplicitAffinities(map[string]*ImplicitAffinity) error

	// GetActivePolicy returns the name of the active policy stored in the cache.
	GetActivePolicy() string
	// SetActivePolicy updates the name of the active policy stored in the cache.
	SetActivePolicy(string) error

	// ResetActivePolicy clears the active policy any any policy-specific data from the cache.
	ResetActivePolicy() error

	// SetPolicyEntry sets the policy entry for a key.
	SetPolicyEntry(string, interface{})
	// GetPolicyEntry gets the policy entry for a key.
	GetPolicyEntry(string, interface{}) bool

	// SetConfig caches the given configuration.
	SetConfig(*config.RawConfig) error
	// GetConfig returns the current/cached configuration.
	GetConfig() *config.RawConfig

	// Save requests a cache save.
	Save() error

	// Refresh requests purging old entries and creating new ones.
	Refresh(rpl interface{}) ([]Pod, []Pod, []Container, []Container)

	// Get the container (data) directory for a container.
	ContainerDirectory(string) string
	// OpenFile opens the names container data file, creating it if necessary.
	OpenFile(string, string, os.FileMode) (*os.File, error)
	// WriteFile writes a container data file, creating it if necessary.
	WriteFile(string, string, os.FileMode, []byte) error
}

const (
	// CacheVersion is the running version of the cache.
	CacheVersion = "1"
)

// permissions describe preferred/expected ownership and permissions for a file or directory.
type permissions struct {
	prefer os.FileMode // permissions to create file/directory with
	reject os.FileMode // bits that cause rejection to use an existing entry
}

type MetricsEntry map[string]metricsring.SampleBuffer

type Metricsdata struct {
	Containers map[string]MetricsEntry
}

// permissions to create with/check against
var (
	cacheDirPerm  = &permissions{prefer: 0710, reject: 0022}
	cacheFilePerm = &permissions{prefer: 0644, reject: 0022}
	dataDirPerm   = &permissions{prefer: 0755, reject: 0022}
	dataFilePerm  = &permissions{prefer: 0644, reject: 0022}
)

// Our cache of objects.
type cache struct {
	sync.Mutex    `json:"-"` // we're lockable
	logger.Logger `json:"-"` // cache logger instance
	filePath      string     // where to store to/load from
	dataDir       string     // container data directory

	Pods       map[string]*pod       // known/cached pods
	Containers map[string]*container // known/cache containers
	Metrics    *Metricsdata          // known/cached metrics for containers
	NextID     uint64                // next container cache id to use

	Cfg        *config.RawConfig      // cached/current configuration
	PolicyName string                 // name of the active policy
	policyData map[string]interface{} // opaque policy data
	PolicyJSON map[string]string      // ditto in raw, unmarshaled form

	pending map[string]struct{} // cache IDs of containers with pending changes

	implicit map[string]*ImplicitAffinity // implicit affinities
}

// Make sure cache implements Cache.
var _ Cache = &cache{}

// Options contains the configurable cache options.
type Options struct {
	// CacheDir is the directory the cache should save its state in.
	CacheDir string
}

// NewCache instantiates a new cache. Load it from the given path if it exists.
func NewCache(options Options) (Cache, error) {
	cch := &cache{
		filePath:   filepath.Join(options.CacheDir, "cache"),
		dataDir:    filepath.Join(options.CacheDir, "containers"),
		Logger:     logger.NewLogger("cache"),
		Pods:       make(map[string]*pod),
		Containers: make(map[string]*container),
		Metrics:    &Metricsdata{Containers: make(map[string]MetricsEntry)},
		NextID:     1,
		policyData: make(map[string]interface{}),
		PolicyJSON: make(map[string]string),
		implicit:   make(map[string]*ImplicitAffinity),
	}

	if _, err := cch.checkPerm("cache", cch.filePath, false, cacheFilePerm); err != nil {
		return nil, cacheError("refusing to use existing cache file: %v", err)
	}
	if err := cch.mkdirAll("cache", options.CacheDir, cacheDirPerm); err != nil {
		return nil, err
	}
	if err := cch.mkdirAll("container", cch.dataDir, dataDirPerm); err != nil {
		return nil, err
	}
	if err := cch.Load(); err != nil {
		return nil, err
	}

	return cch, nil
}

func (cch *cache) GetMetrics() *Metricsdata {
	return cch.Metrics
}

func (cch *cache) SetMetrics(m *Metricsdata) {
	cch.Metrics = m
}

// GetActivePolicy returns the name of the active policy stored in the cache.
func (cch *cache) GetActivePolicy() string {
	return cch.PolicyName
}

// SetActivePolicy updaes the name of the active policy stored in the cache.
func (cch *cache) SetActivePolicy(policy string) error {
	cch.PolicyName = policy
	return cch.Save()
}

// ResetActivePolicy clears the active policy any any policy-specific data from the cache.
func (cch *cache) ResetActivePolicy() error {
	cch.Warn("clearing all data for active policy (%q) from cache...",
		cch.PolicyName)

	cch.PolicyName = ""
	cch.policyData = make(map[string]interface{})
	cch.PolicyJSON = make(map[string]string)

	return cch.Save()
}

// SetConfig caches the given configuration.
func (cch *cache) SetConfig(cfg *config.RawConfig) error {
	old := cch.Cfg
	cch.Cfg = cfg

	if err := cch.Save(); err != nil {
		cch.Cfg = old
		return err
	}

	return nil
}

// GetConfig returns the current/cached configuration.
func (cch *cache) GetConfig() *config.RawConfig {
	return cch.Cfg
}

// Derive cache id using pod uid, or allocate a new unused local cache id.
func (cch *cache) createCacheID(c *container) string {
	if pod, ok := c.cache.LookupPod(c.PodID); ok {
		uid := pod.GetUID()
		if uid != "" {
			return uid + ":" + c.Name
		}
	}

	cch.Warn("can't find unique id for pod %s, assigning local cache id", c.PodID)
	id := "cache:" + strconv.FormatUint(cch.NextID, 16)
	cch.NextID++

	return id
}

// Insert a pod into the cache.
func (cch *cache) InsertPod(id string, msg interface{}) Pod {
	var err error

	p := &pod{cache: cch, ID: id}

	switch msg.(type) {
	case *cri.RunPodSandboxRequest:
		err = p.fromRunRequest(msg.(*cri.RunPodSandboxRequest))
	case *cri.PodSandbox:
		err = p.fromListResponse(msg.(*cri.PodSandbox))
	default:
		err = fmt.Errorf("cannot create pod from message %T", msg)
	}

	if err != nil {
		cch.Error("failed to insert pod %s: %v", id, err)
		return nil
	}

	cch.Pods[p.ID] = p

	cch.Save()

	return p
}

// Delete a pod from the cache.
func (cch *cache) DeletePod(id string) Pod {
	p, ok := cch.Pods[id]
	if !ok {
		return nil
	}

	cch.Debug("removing pod %s", p.ID)
	delete(cch.Pods, id)

	cch.Save()

	return p
}

// Look up a pod in the cache.
func (cch *cache) LookupPod(id string) (Pod, bool) {
	p, ok := cch.Pods[id]
	return p, ok
}

// Insert a container into the cache.
func (cch *cache) InsertContainer(msg interface{}) (Container, error) {
	var err error

	c := &container{
		cache: cch,
	}

	switch msg.(type) {
	case *cri.CreateContainerRequest:
		err = c.fromCreateRequest(msg.(*cri.CreateContainerRequest))
	case *cri.Container:
		err = c.fromListResponse(msg.(*cri.Container))
	default:
		err = fmt.Errorf("cannot create container from message %T", msg)
	}

	if err != nil {
		return nil, cacheError("failed to insert container %s: %v", c.CacheID, err)
	}

	c.CacheID = cch.createCacheID(c)

	cch.Containers[c.CacheID] = c
	if c.ID != "" {
		cch.Containers[c.ID] = c
	}

	cch.createContainerDirectory(c.CacheID)

	cch.Save()

	return c, nil
}

// UpdateContainerID updates a containers runtime id.
func (cch *cache) UpdateContainerID(cacheID string, msg interface{}) (Container, error) {
	c, ok := cch.Containers[cacheID]
	if !ok {
		return nil, cacheError("failed to update container id, container %s not found", cacheID)
	}

	switch msg.(type) {
	case *cri.CreateContainerResponse:
		c.ID = msg.(*cri.CreateContainerResponse).ContainerId
	default:
		return nil, cacheError("can't update container id from message %T", msg)
	}

	cch.Containers[c.ID] = c

	cch.Save()

	return c, nil
}

// Delete a pod from the cache.
func (cch *cache) DeleteContainer(id string) Container {
	c, ok := cch.Containers[id]
	if !ok {
		return nil
	}

	cch.Debug("removing container %s/%s", c.ID, c.CacheID)
	cch.removeContainerDirectory(c.CacheID)
	delete(cch.Containers, c.ID)
	delete(cch.Containers, c.CacheID)
	delete(cch.Metrics.Containers, c.ID)

	cch.Save()

	return c
}

// Look up a pod in the cache.
func (cch *cache) LookupContainer(id string) (Container, bool) {
	c, ok := cch.Containers[id]
	return c, ok
}

// LookupContainerByCgroup looks up the container for the given cgroup path.
func (cch *cache) LookupContainerByCgroup(path string) (Container, bool) {
	cch.Debug("resolving %s to a container...", path)

	for id, c := range cch.Containers {
		if id != c.CacheID {
			continue
		}

		parent := ""
		if pod, ok := c.GetPod(); ok {
			parent = pod.GetCgroupParentDir()
		}
		if parent == "" {
			continue
		}

		if !strings.HasPrefix(path, parent+"/") {
			continue
		}

		if strings.Index(path, c.GetID()) != -1 {
			return c, true
		}
	}

	return nil, false
}

// Refresh the cache from an (assumed to be unfiltered) pod or container list response.
func (cch *cache) Refresh(rpl interface{}) ([]Pod, []Pod, []Container, []Container) {
	switch rpl.(type) {
	case *cri.ListPodSandboxResponse:
		add, del, containers := cch.RefreshPods(rpl.(*cri.ListPodSandboxResponse))
		return add, del, nil, containers

	case *cri.ListContainersResponse:
		add, del := cch.RefreshContainers(rpl.(*cri.ListContainersResponse))
		return nil, nil, add, del
	}

	cch.Error("can't refresh cache using a %T message", rpl)
	return nil, nil, nil, nil
}

// Refresh pods, purging stale and inserting new ones using a pod sandbox list response.
func (cch *cache) RefreshPods(msg *cri.ListPodSandboxResponse) ([]Pod, []Pod, []Container) {
	valid := make(map[string]struct{})

	add := []Pod{}
	del := []Pod{}
	containers := []Container{}

	for _, item := range msg.Items {
		valid[item.Id] = struct{}{}
		if _, ok := cch.Pods[item.Id]; !ok {
			cch.Debug("inserting discovered pod %s...", item.Id)
			pod := cch.InsertPod(item.Id, item)
			add = append(add, pod)
		}
	}

	for _, pod := range cch.Pods {
		if _, ok := valid[pod.ID]; !ok {
			cch.Debug("purging stale pod %s...", pod.ID)
			pod.State = PodStateStale
			del = append(del, cch.DeletePod(pod.ID))
		}
	}

	for id, c := range cch.Containers {
		if _, ok := valid[c.PodID]; !ok {
			cch.Debug("purging container %s of stale pod %s...", c.CacheID, c.PodID)
			cch.DeleteContainer(c.CacheID)
			c.State = ContainerStateStale
			if id == c.CacheID {
				containers = append(containers, c)
			}
		}
	}

	return add, del, containers
}

// Refresh pods, purging stale and inserting new ones using a pod sandbox list response.
func (cch *cache) RefreshContainers(msg *cri.ListContainersResponse) ([]Container, []Container) {
	valid := make(map[string]struct{})

	add := []Container{}
	del := []Container{}

	for _, c := range msg.Containers {
		if ContainerState(c.State) == ContainerStateExited {
			continue
		}

		valid[c.Id] = struct{}{}
		if _, ok := cch.Containers[c.Id]; !ok {
			cch.Debug("inserting discovered container %s...", c.Id)
			inserted, err := cch.InsertContainer(c)
			if err != nil {
				cch.Error("failed to insert discovered container %s to cache: %v",
					c.Id, err)
			} else {
				add = append(add, inserted)
			}
		}
	}

	for id, c := range cch.Containers {
		if _, ok := valid[c.ID]; !ok {
			cch.Debug("purging stale container %s (state: %v)...", c.CacheID, c.GetState())
			cch.DeleteContainer(c.CacheID)
			c.State = ContainerStateStale
			if id == c.CacheID {
				del = append(del, c)
			}
		}
	}

	return add, del
}

// Mark a container as having pending changes.
func (cch *cache) markPending(c *container) {
	if cch.pending == nil {
		cch.pending = make(map[string]struct{})
	}
	cch.pending[c.CacheID] = struct{}{}
}

// Get all containers with pending changes.
func (cch *cache) GetPendingContainers() []Container {
	pending := make([]Container, 0, len(cch.pending))
	for id := range cch.pending {
		c, ok := cch.LookupContainer(id)
		if ok {
			pending = append(pending, c)
		}
	}
	return pending
}

// clear the pending state of the given container.
func (cch *cache) clearPending(c *container) {
	delete(cch.pending, c.CacheID)
}

// Get the cache ids of all cached containers.
func (cch *cache) GetContainerCacheIds() []string {
	ids := make([]string, len(cch.Containers))

	idx := 0
	for id, c := range cch.Containers {
		if id != c.CacheID {
			continue
		}
		ids[idx] = c.CacheID
		idx++
	}

	return ids[0:idx]
}

// Get the ids of all cached containers.
func (cch *cache) GetContainerIds() []string {
	ids := make([]string, len(cch.Containers))

	idx := 0
	for id, c := range cch.Containers {
		if id == c.CacheID {
			continue
		}
		ids[idx] = c.ID
		idx++
	}

	return ids[0:idx]
}

// GetPods returns all pods present in the cache.
func (cch *cache) GetPods() []Pod {
	pods := make([]Pod, 0, len(cch.Pods))
	for _, pod := range cch.Pods {
		pods = append(pods, pod)
	}
	return pods
}

// GetContainers returns all the containers present in the cache.
func (cch *cache) GetContainers() []Container {
	containers := make([]Container, 0, len(cch.Containers)/2)
	for id, container := range cch.Containers {
		if id != container.CacheID {
			continue
		}
		containers = append(containers, container)
	}
	return containers
}

// Set the policy entry for a key.
func (cch *cache) SetPolicyEntry(key string, obj interface{}) {
	cch.policyData[key] = obj

	if cch.DebugEnabled() {
		if data, err := marshalEntry(obj); err != nil {
			cch.Error("marshalling of policy entry '%s' failed: %v", key, err)
		} else {
			cch.Debug("policy entry '%s' set to '%s'", key, string(data))
		}
	}
}

// Get the policy entry for a key.
func (cch *cache) GetPolicyEntry(key string, ptr interface{}) bool {

	//
	// Notes:
	//     We try to serve requests from the demarshaled cache (policyData).
	//     If that fails (may be a first access since load) we look for the
	//     entry in the unmarshaled cache (PolicyJSON), demarshal, and cache
	//     the entry if found.
	//     Note the quirk: in the latter case we first directly unmarshal to
	//     the pointer provided by the caller, only then Get() and cache the
	//     result.
	//

	obj, ok := cch.policyData[key]
	if !ok {
		entry, ok := cch.PolicyJSON[key]
		if !ok {
			return false
		}

		// first access to key since startup
		if err := unmarshalEntry([]byte(entry), ptr); err != nil {
			cch.Fatal("failed to unmarshal '%s' policy entry for key '%s' (%T): %v",
				cch.PolicyName, key, ptr, err)
		}

		if err := cch.cacheEntry(key, ptr); err != nil {
			cch.Fatal("failed to cache '%s' policy entry for key '%s': %v",
				cch.PolicyName, key, err)
		}
	} else {
		// subsequent accesses to key
		if err := cch.setEntry(key, ptr, obj); err != nil {
			cch.Fatal("failed use cached entry for key '%s' of policy '%s': %v",
				key, cch.PolicyName, err)
		}
	}

	return true
}

// Marshal an opaque policy entry, special-casing cpusets and maps of cpusets.
func marshalEntry(obj interface{}) ([]byte, error) {
	switch obj.(type) {
	case cpuset.CPUSet:
		return []byte("\"" + obj.(cpuset.CPUSet).String() + "\""), nil
	case map[string]cpuset.CPUSet:
		dst := make(map[string]string)
		for key, cset := range obj.(map[string]cpuset.CPUSet) {
			dst[key] = cset.String()
		}
		return json.Marshal(dst)

	default:
		return json.Marshal(obj)
	}
}

// Unmarshal an opaque policy entry, special-casing cpusets and maps of cpusets.
func unmarshalEntry(data []byte, ptr interface{}) error {
	switch ptr.(type) {
	case *cpuset.CPUSet:
		cset, err := cpuset.Parse(string(data[1 : len(data)-1]))
		if err != nil {
			return err
		}
		*ptr.(*cpuset.CPUSet) = cset
		return nil

	case *map[string]cpuset.CPUSet:
		src := make(map[string]string)
		if err := json.Unmarshal([]byte(data), &src); err != nil {
			return cacheError("failed to unmarshal map[string]cpuset.CPUSet: %v", err)
		}

		dst := make(map[string]cpuset.CPUSet)
		for key, str := range src {
			cset, err := cpuset.Parse(str)
			if err != nil {
				return cacheError("failed to unmarshal cpuset.CPUSet '%s': %v", str, err)
			}
			dst[key] = cset
		}

		*ptr.(*map[string]cpuset.CPUSet) = dst
		return nil

	default:
		err := json.Unmarshal(data, ptr)
		return err
	}
}

// Cache an unmarshaled opaque policy entry, special-casing some simple/common types.
func (cch *cache) cacheEntry(key string, ptr interface{}) error {
	if cachable, ok := ptr.(Cachable); ok {
		cch.policyData[key] = cachable.Get()
		return nil
	}

	switch ptr.(type) {
	case *cpuset.CPUSet:
		cch.policyData[key] = *ptr.(*cpuset.CPUSet)
	case *map[string]cpuset.CPUSet:
		cch.policyData[key] = *ptr.(*map[string]cpuset.CPUSet)
	case *map[string]string:
		cch.policyData[key] = *ptr.(*map[string]string)

	case *string:
		cch.policyData[key] = *ptr.(*string)
	case *bool:
		cch.policyData[key] = *ptr.(*bool)

	case *int32:
		cch.policyData[key] = *ptr.(*int32)
	case *uint32:
		cch.policyData[key] = *ptr.(*uint32)
	case *int64:
		cch.policyData[key] = *ptr.(*int64)
	case *uint64:
		cch.policyData[key] = *ptr.(*uint64)

	case *int:
		cch.policyData[key] = *ptr.(*int)
	case *uint:
		cch.policyData[key] = *ptr.(*uint)

	default:
		return cacheError("can't handle policy data of type %T", ptr)
	}

	return nil
}

// Serve an unmarshaled opaque policy entry, special-casing some simple/common types.
func (cch *cache) setEntry(key string, ptr, obj interface{}) error {
	if cachable, ok := ptr.(Cachable); ok {
		cachable.Set(obj)
		return nil
	}

	switch ptr.(type) {
	case *cpuset.CPUSet:
		*ptr.(*cpuset.CPUSet) = obj.(cpuset.CPUSet)
	case *map[string]cpuset.CPUSet:
		*ptr.(*map[string]cpuset.CPUSet) = obj.(map[string]cpuset.CPUSet)
	case *map[string]string:
		*ptr.(*map[string]string) = obj.(map[string]string)

	case *string:
		*ptr.(*string) = obj.(string)
	case *bool:
		*ptr.(*bool) = obj.(bool)

	case *int32:
		*ptr.(*int32) = obj.(int32)
	case *uint32:
		*ptr.(*uint32) = obj.(uint32)
	case *int64:
		*ptr.(*int64) = obj.(int64)
	case *uint64:
		*ptr.(*uint64) = obj.(uint64)

	case *int:
		*ptr.(*int) = obj.(int)
	case *uint:
		*ptr.(*uint) = obj.(uint)

	default:
		return cacheError("can't handle policy data of type %T", ptr)
	}

	return nil
}

// checkPerm checks permissions of an already existing file or directory.
func (cch *cache) checkPerm(what, path string, isDir bool, p *permissions) (bool, error) {
	if isDir {
		what += " directory"
	}

	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return true, cacheError("failed to os.Stat() %s %q: %v", what, path, err)
		}
		return false, nil
	}

	// check expected file type
	if isDir {
		if !info.IsDir() {
			return true, cacheError("%s %q exists, but is not a directory", what, path)
		}
	} else {
		if info.Mode()&os.ModeType != 0 {
			return true, cacheError("%s %q exists, but is not a regular file", what, path)
		}
	}

	existing := info.Mode().Perm()
	expected := p.prefer
	rejected := p.reject
	if ((expected | rejected) &^ os.ModePerm) != 0 {
		cch.Panic("internal error: current permissions check only handles permission bits (rwx)")
	}

	// check that we don't have any of the rejectable permission bits set
	if existing&rejected != 0 {
		return true, cacheError("existing %s %q has disallowed permissions set: %v",
			what, path, existing&rejected)
	}

	// warn if permissions are less strict than the preferred defaults
	if (existing | expected) != expected {
		cch.Warn("existing %s %q has less strict permissions %v than expected %v",
			what, path, existing, expected)
	}

	return true, nil
}

// mkdirAll creates a directory, checking permissions if it already exists.
func (cch *cache) mkdirAll(what, path string, p *permissions) error {
	exists, err := cch.checkPerm(what, path, true, p)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := os.MkdirAll(path, p.prefer); err != nil {
		return cacheError("failed to create %s directory %q: %v", what, path, err)
	}

	return nil
}

// snapshot is used to serialize the cache into a saveable/loadable state.
type snapshot struct {
	Version    string
	Pods       map[string]*pod
	Containers map[string]*container
	NextID     uint64
	Cfg        *config.RawConfig
	PolicyName string
	PolicyJSON map[string]string
}

// Snapshot takes a restorable snapshot of the current state of the cache.
func (cch *cache) Snapshot() ([]byte, error) {
	s := snapshot{
		Version:    CacheVersion,
		Pods:       make(map[string]*pod),
		Containers: make(map[string]*container),
		Cfg:        cch.Cfg,
		NextID:     cch.NextID,
		PolicyName: cch.PolicyName,
		PolicyJSON: cch.PolicyJSON,
	}

	for id, p := range cch.Pods {
		s.Pods[id] = p
	}

	for id, c := range cch.Containers {
		if id == c.CacheID {
			s.Containers[c.CacheID] = c
		}
	}

	for key, obj := range cch.policyData {
		data, err := marshalEntry(obj)
		if err != nil {
			return nil, cacheError("failed to marshal policy entry '%s': %v", key, err)
		}

		s.PolicyJSON[key] = string(data)
	}

	data, err := json.Marshal(s)
	if err != nil {
		return nil, cacheError("failed to marshal cache: %v", err)
	}

	return data, nil
}

// Restore restores a previously takes snapshot of the cache.
func (cch *cache) Restore(data []byte) error {
	s := snapshot{
		Pods:       make(map[string]*pod),
		Containers: make(map[string]*container),
		PolicyJSON: make(map[string]string),
	}

	if err := json.Unmarshal(data, &s); err != nil {
		return cacheError("failed to unmarshal snapshot data: %v", err)
	}

	if s.Version != CacheVersion {
		return cacheError("can't restore snapshot, version '%s' != running version %s",
			s.Version, CacheVersion)
	}

	cch.Pods = s.Pods
	cch.Containers = s.Containers
	cch.Cfg = s.Cfg
	cch.NextID = s.NextID
	cch.PolicyJSON = s.PolicyJSON
	cch.PolicyName = s.PolicyName
	cch.policyData = make(map[string]interface{})

	for _, p := range cch.Pods {
		p.cache = cch
		p.containers = make(map[string]string)
	}
	for _, c := range cch.Containers {
		c.cache = cch
		cch.Containers[c.CacheID] = c
		if c.ID != "" {
			cch.Containers[c.ID] = c
		}
	}

	return nil
}

// Save the state of the cache.
func (cch *cache) Save() error {
	cch.Debug("saving cache to file '%s'...", cch.filePath)

	data, err := cch.Snapshot()
	if err != nil {
		return cacheError("failed to save cache: %v", err)
	}

	if err = ioutil.WriteFile(cch.filePath, data, cacheFilePerm.prefer); err != nil {
		return cacheError("failed to write cache to file '%s': %v", cch.filePath, err)
	}

	return nil
}

// Load loads the last saved state of the cache.
func (cch *cache) Load() error {
	cch.Debug("loading cache from file '%s'...", cch.filePath)

	data, err := ioutil.ReadFile(cch.filePath)

	switch {
	case os.IsNotExist(err):
		cch.Debug("no cache file '%s', nothing to restore", cch.filePath)
		return nil
	case len(data) == 0:
		cch.Debug("empty cache file '%s', nothing to restore", cch.filePath)
		return nil
	case err != nil:
		return cacheError("failed to load cache from file '%s': %v", cch.filePath, err)
	}

	return cch.Restore(data)
}

func (cch *cache) ContainerDirectory(id string) string {
	c, ok := cch.Containers[id]
	if !ok {
		return ""
	}
	return filepath.Join(cch.dataDir, strings.Replace(c.CacheID, ":", "-", 1))
}

func (cch *cache) createContainerDirectory(id string) error {
	dir := cch.ContainerDirectory(id)
	if dir == "" {
		return cacheError("failed to determine container directory path for container %s", id)
	}
	return cch.mkdirAll("container directory", dir, dataDirPerm)
}

func (cch *cache) removeContainerDirectory(id string) error {
	dir := cch.ContainerDirectory(id)
	if dir == "" {
		return cacheError("failed to delete directory for container %s", id)
	}
	return os.RemoveAll(dir)
}

func (cch *cache) OpenFile(id string, name string, perm os.FileMode) (*os.File, error) {
	dir := cch.ContainerDirectory(id)
	if dir == "" {
		return nil, cacheError("failed to determine data directory for container %s", id)
	}
	if err := cch.mkdirAll("container directory", dir, dataDirPerm); err != nil {
		return nil, cacheError("container %s: can't create data file %q: %v", id, name, err)
	}

	path := filepath.Join(dir, name)
	if _, err := cch.checkPerm("container", path, false, dataFilePerm); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return nil, cacheError("container %s: can't open data file %q: %v", id, path, err)
	}

	return file, nil
}

func (cch *cache) WriteFile(id string, name string, perm os.FileMode, data []byte) error {
	file, err := cch.OpenFile(id, name, perm)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)

	return err
}
