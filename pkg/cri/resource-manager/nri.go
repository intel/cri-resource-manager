// Copyright 2021 Intel Corporation. All Rights Reserved.
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
	"fmt"
	"strings"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	"github.com/containerd/nri/pkg/api"
	stub "github.com/containerd/nri/pkg/stub"
)

type nriPlugin struct {
	stub   stub.Stub
	resmgr *resmgr
}

func newNRIPlugin(resmgr *resmgr) (*nriPlugin, error) {
	p := &nriPlugin{
		resmgr: resmgr,
	}

	p.resmgr.Info("creating NRI plugin...")

	return p, nil
}

func (p *nriPlugin) createStub() error {
	var (
		opts = []stub.Option{
			stub.WithPluginName("resource-manager"),
			stub.WithPluginIdx("90"),
			stub.WithOnClose(p.onClose),
		}
		err error
	)

	p.resmgr.Info("creating NRI plugin stub...")

	if p.stub, err = stub.New(p, opts...); err != nil {
		return errors.Wrap(err, "failed to create NRI plugin stub")
	}

	return nil
}

func (p *nriPlugin) start() error {
	if p == nil {
		return nil
	}

	p.resmgr.Info("starting NRI plugin...")

	if err := p.createStub(); err != nil {
		return err
	}

	if err := p.stub.Start(context.Background()); err != nil {
		return errors.Wrap(err, "failed to start NRI plugin")
	}

	return nil
}

func (p *nriPlugin) stop() {
	if p == nil {
		return
	}
	p.resmgr.Info("stopping NRI plugin...")
	p.stub.Stop()
}

func (p *nriPlugin) restart() error {
	return p.start()
}

func (p *nriPlugin) onClose() {
	p.resmgr.Warn("connection to NRI/runtime lost, trying to reconnect...")
	p.restart()
}

func (p *nriPlugin) Configure(cfg, runtime, version string) (stub.EventMask, error) {
	p.dump("NRI-Configure", "data", cfg)
	return api.MustParseEventMask(
		"RunPodSandbox,StopPodSandbox,RemovePodSandbox",
		"CreateContainer,StartContainer,UpdateContainer,StopContainer,RemoveContainer",
	), nil
}

func (p *nriPlugin) syncWithNRI(pods []*api.PodSandbox, containers []*api.Container) ([]cache.Container, []cache.Container, error) {
	m := p.resmgr

	if m.policy.Bypassed() {
		return nil, nil, nil
	}

	m.Info("synchronizing cache state with NRI/CRI runtime...")

	add, del := []cache.Container{}, []cache.Container{}
	status := map[string]*cache.PodStatus{}

	_, _, deleted := m.cache.RefreshPods(pods, status)
	for _, c := range deleted {
		m.Info("discovered stale container %s...", c.GetID())
		del = append(del, c)
	}

	added, deleted := m.cache.RefreshContainers(containers)
	for _, c := range added {
		if c.GetState() != cache.ContainerStateRunning {
			m.Info("ignoring discovered container %s (in state %v)...",
				c.GetID(), c.GetState())
			continue
		}
		m.Info("discovered out-of-sync running container %s...", c.GetID())
		add = append(add, c)
	}
	for _, c := range deleted {
		m.Info("discovered stale container %s...", c.GetID())
		del = append(del, c)
	}

	return add, del, nil
}

func (p *nriPlugin) Synchronize(pods []*api.PodSandbox, containers []*api.Container) ([]*api.ContainerUpdate, error) {
	m := p.resmgr

	p.dump("NRI-Synchronize", "pods", pods, "containers", containers)

	add, del, err := p.syncWithNRI(pods, containers)
	if err != nil {
		p.resmgr.Error("failed to synchronize with NRI: %v", err)
		return nil, err
	}

	if err := m.policy.Start(add, del); err != nil {
		return nil, errors.Wrapf(err,
			"failed to start policy %s", policy.ActivePolicy())
	}

	return p.getPendingUpdates(nil), nil
}

func (p *nriPlugin) RunPodSandbox(pod *api.PodSandbox) error {
	p.dump("NRI-RunPodSandbox", "pod", pod)

	m := p.resmgr

	m.Lock()
	defer m.Unlock()

	m.cache.InsertPod(pod.Id, pod, nil)
	return nil
}

func (p *nriPlugin) StopPodSandbox(pod *api.PodSandbox) error {
	p.dump("NRI-StopPodSandbox", "pod", pod)
	return nil
}

func (p *nriPlugin) RemovePodSandbox(pod *api.PodSandbox) error {
	p.dump("NRI-RemovePodSandbox", "pod", pod)

	m := p.resmgr

	m.Lock()
	defer m.Unlock()

	m.cache.DeletePod(pod.Id)
	return nil
}

func (p *nriPlugin) CreateContainer(pod *api.PodSandbox, container *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
	p.dump("NRI-CreateContainer", "pod", pod, "container", container)

	m := p.resmgr

	m.Lock()
	defer m.Unlock()

	c, err := m.cache.InsertContainer(container)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to cache container")
	}

	if err := m.policy.AllocateResources(c); err != nil {
		return nil, nil, errors.Wrap(err, "failed to allocate resources")
	}

	c.InsertMount(&cache.Mount{
		Container:   "/.cri-resmgr",
		Host:        m.cache.ContainerDirectory(c.GetCacheID()),
		Readonly:    true,
		Propagation: cache.MountHostToContainer,
	})
	m.policy.ExportResourceData(c)

	adjust := p.getPendingCreate(container)
	updates := p.getPendingUpdates(container)

	p.dump("<= CreateContainer", "adjustments", adjust, "updates", updates)

	return adjust, updates, nil
}

func (p *nriPlugin) StartContainer(pod *api.PodSandbox, container *api.Container) error {
	p.dump("NRI-StartContainer", "pod", pod, "container", container)

	m := p.resmgr

	m.Lock()
	defer m.Unlock()

	c, ok := m.cache.LookupContainer(container.Id)
	if ok {
		c.UpdateState(cache.ContainerStateRunning)
	}

	// test timeouts
	//time.Sleep(3 * time.Second)

	return nil
}

func (p *nriPlugin) UpdateContainer(pod *api.PodSandbox, container *api.Container) ([]*api.ContainerUpdate, error) {
	p.dump("NRI-UpdateContainer", "pod", pod, "container", container)
	return nil, nil
}

func (p *nriPlugin) StopContainer(pod *api.PodSandbox, container *api.Container) ([]*api.ContainerUpdate, error) {
	p.dump("NRI-StopContainer", "pod", pod, "container", container)

	m := p.resmgr

	m.Lock()
	defer m.Unlock()

	c, ok := m.cache.LookupContainer(container.Id)
	if !ok {
		return nil, nil
	}

	if err := m.policy.ReleaseResources(c); err != nil {
		return nil, errors.Wrap(err, "failed to release resources")
	}

	c.UpdateState(cache.ContainerStateExited)

	updates := p.getPendingUpdates(container)

	return updates, nil
}

func (p *nriPlugin) RemoveContainer(pod *api.PodSandbox, container *api.Container) error {
	p.dump("NRI-RemoveContainer", "pod", pod, "container", container)

	m := p.resmgr

	m.Lock()
	defer m.Unlock()

	m.cache.DeleteContainer(container.Id)
	return nil
}

func (p *nriPlugin) getPendingCreate(container *api.Container) *api.ContainerAdjustment {
	m := p.resmgr
	c, ok := m.cache.LookupContainer(container.GetId())
	if !ok {
		return nil
	}

	for _, ctrl := range c.GetPending() {
		c.ClearPending(ctrl)
	}

	a := &api.ContainerAdjustment{}
	p.adjustDevices(a, container, c)
	p.adjustResources(a, container, c)
	p.adjustAnnotations(a, container, c)
	p.adjustEnv(a, container, c)
	p.adjustMounts(a, container, c)

	return a
}

func (p *nriPlugin) getPendingUpdates(creating *api.Container) []*api.ContainerUpdate {
	m := p.resmgr
	updates := []*api.ContainerUpdate{}
	for _, container := range m.cache.GetPendingContainers() {
		id := container.GetID()
		if creating != nil && creating.GetId() == id {
			continue
		}

		u := &api.ContainerUpdate{
			ContainerId: id,
		}
		p.updateResources(u, container)
		updates = append(updates, u)

		for _, ctrl := range container.GetPending() {
			container.ClearPending(ctrl)
		}
	}

	return updates
}

func toNRILinuxResources(container cache.Container) *api.LinuxResources {
	cr := container.GetLinuxResources()
	if cr == nil {
		return nil
	}

	r := &api.LinuxResources{
		Cpu: &api.LinuxCPU{
			Period: api.UInt64(cr.CpuPeriod),
			Quota:  api.Int64(cr.CpuQuota),
			Shares: api.UInt64(cr.CpuShares),
			Cpus:   cr.CpusetCpus,
			Mems:   cr.CpusetMems,
		},
		Memory: &api.LinuxMemory{
			Limit: api.Int64(cr.MemoryLimitInBytes),
		},
	}
	for _, l := range r.HugepageLimits {
		r.HugepageLimits = append(r.HugepageLimits, &api.HugepageLimit{
			PageSize: l.PageSize,
			Limit:    l.Limit,
		})
	}
	if bioc := container.GetBlockIOClass(); bioc != "" {
		r.BlockioClass = api.String(bioc)
	}
	if rdtc := container.GetRDTClass(); rdtc != "" {
		r.RdtClass = api.String(rdtc)
	}

	return r
}

func (p *nriPlugin) adjustDevices(a *api.ContainerAdjustment, c *api.Container, cc cache.Container) {
	// Notes: we don't alter devices... but it should perhaps be checked.
}

func (p *nriPlugin) adjustResources(a *api.ContainerAdjustment, c *api.Container, cc cache.Container) {
	ccr := cc.GetLinuxResources()
	a.SetLinuxCPUPeriod(ccr.CpuPeriod)
	a.SetLinuxCPUQuota(ccr.CpuQuota)
	a.SetLinuxCPUShares(uint64(ccr.CpuShares))
	a.SetLinuxCPUSetCPUs(ccr.CpusetCpus)
	a.SetLinuxCPUSetMems(ccr.CpusetMems)
	a.SetLinuxMemoryLimit(ccr.MemoryLimitInBytes)
	for _, l := range ccr.HugepageLimits {
		a.AddLinuxHugepageLimit(l.PageSize, l.Limit)
	}
	if bioc := cc.GetBlockIOClass(); bioc != "" {
		// XXX TODO skip setting blockio class for now
		//
		// We need to do class to CLOS resolution here using goresctrl.
		// For that we need to figure out how to properly initialize
		// goresctrl if we are running as a DaemonSet with host sysfs
		// mounted under a non-standard location.

		p.resmgr.Info("*** TODO: should set Block I/O class to %s...", bioc)

		//a.SetLinuxBlockIOClass(bioc)
	}
	// XXX TODO skip setting RDT class for now...
	//
	// We need to do class to CLOS resolution here using goresctrl.
	// For that we need to figure out how to properly initialize
	// goresctrl if we are running as a DaemonSet with host sysfs
	// mounted under a non-standard location.
	if rdtc := cc.GetRDTClass(); rdtc != "" {
		if rdtc == cache.RDTClassPodQoS {
			rdtc = string(cc.GetQOSClass())
		}
		if rdtc != "" {
			p.resmgr.Info("*** TODO: should set RDT class to %s...", rdtc)
		}

		//a.SetLinuxRDTClass(rdtc)
	}
}

func (p *nriPlugin) adjustMounts(a *api.ContainerAdjustment, c *api.Container, cc cache.Container) {
	// Notes: we don't alter mounts, just inject new ones...
nextMount:
	for _, mnt := range cc.GetMounts() {
		for _, m := range c.GetMounts() {
			if m.Destination == mnt.Container {
				continue nextMount
			}
		}

		options := []string{"rbind"}

		switch mnt.Propagation {
		case cache.MountPrivate:
			options = append(options, "rprivate")
		case cache.MountHostToContainer:
			options = append(options, "rslave")
		case cache.MountBidirectional:
			options = append(options, "rshared")
		}
		if mnt.Readonly {
			options = append(options, "ro")
		}
		if mnt.Relabel {
			options = append(options, api.SELinuxRelabel)
		}

		a.AddMount(&api.Mount{
			Destination: mnt.Container,
			Source:      mnt.Host,
			Options:     options,
		})
	}
}

func (p *nriPlugin) adjustEnv(a *api.ContainerAdjustment, c *api.Container, cc cache.Container) {
	old := map[string]string{}
	for _, e := range c.GetEnv() {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) < 2 || kv[0] == "" {
			continue
		}
		old[kv[0]] = kv[1]
	}
	for _, k := range cc.GetEnvKeys() {
		if _, ok := old[k]; ok {
			a.RemoveEnv(k)
		}
		v, _ := cc.GetEnv(k)
		if v != "" {
			a.AddEnv(k, v)
		}
	}
}

func (p *nriPlugin) adjustAnnotations(a *api.ContainerAdjustment, c *api.Container, cc cache.Container) {
	old := c.GetAnnotations()
	for k, v := range cc.GetAnnotations() {
		if ov, ok := old[k]; ok {
			if ov == v {
				continue
			}
			a.RemoveAnnotation(k)
		}
		a.AddAnnotation(k, v)
	}
}

func (p *nriPlugin) updateResources(u *api.ContainerUpdate, c cache.Container) {
	cr := c.GetLinuxResources()
	u.SetLinuxCPUPeriod(cr.CpuPeriod)
	u.SetLinuxCPUQuota(cr.CpuQuota)
	u.SetLinuxCPUShares(uint64(cr.CpuShares))
	u.SetLinuxCPUSetCPUs(cr.CpusetCpus)
	u.SetLinuxCPUSetMems(cr.CpusetMems)
	u.SetLinuxMemoryLimit(cr.MemoryLimitInBytes)
	for _, l := range cr.HugepageLimits {
		u.AddLinuxHugepageLimit(l.PageSize, l.Limit)
	}
	if bioc := c.GetBlockIOClass(); bioc != "" {
		u.SetLinuxBlockIOClass(bioc)
	}
	if rdtc := c.GetRDTClass(); rdtc != "" {
		u.SetLinuxRDTClass(rdtc)
	}
}

func (p *nriPlugin) dump(header string, args ...interface{}) {
	for {
		var (
			prefix string
			msg    []byte
			ok     bool
			err    error
		)

		if len(args) == 0 {
			return
		}
		if len(args) == 1 {
			p.resmgr.Error("invalid call to dump, expecting tag/obj pairs")
			return
		}
		if prefix, ok = args[0].(string); !ok {
			p.resmgr.Error("invalid call to dump, expecting string tags")
			return
		}

		msg, err = yaml.Marshal(args[1])
		if err != nil {
			msg = []byte(fmt.Sprintf("<failed to YAML-marshal object: %v>", err))
		}

		p.resmgr.InfoBlock(header+" ", "%s:", prefix)
		p.resmgr.InfoBlock(header+" ", "%s", msg)

		args = args[2:]
	}
}
