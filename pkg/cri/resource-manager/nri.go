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
	"io"
	"net"
	"strings"

	"github.com/pkg/errors"

	nri "github.com/containerd/nri/api/plugin/vproto"
	//cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/containerd/ttrpc"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy"

	"sigs.k8s.io/yaml"
)

type nriPlugin struct {
	listener net.Listener
	server   *ttrpc.Server
	resmgr   *resmgr
}

func newNRIPlugin(resmgr *resmgr) (*nriPlugin, error) {
	conn, err := net.Dial("unix", "/var/run/nri.sock")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to NRI socket")
	}

	server, err := ttrpc.NewServer()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ttrpc server")
	}

	p := &nriPlugin{
		resmgr:   resmgr,
		listener: newConnListener(conn),
		server:   server,
	}

	nri.RegisterPluginService(p.server, p)

	return p, nil
}

func (p *nriPlugin) start() error {
	if p == nil {
		return nil
	}

	go func() {
		p.server.Serve(context.Background(), p.listener)
	}()

	return nil
}

func (p *nriPlugin) stop() error {
	return p.server.Close()
}

func (p *nriPlugin) syncWithNRI(ctx context.Context, msg *nri.SynchronizeRequest) ([]cache.Container, []cache.Container, error) {
	m := p.resmgr

	if m.policy.Bypassed() {
		return nil, nil, nil
	}

	m.Info("synchronizing cache state with NRI/CRI runtime...")

	add, del := []cache.Container{}, []cache.Container{}
	status := map[string]*cache.PodStatus{}

	_, _, deleted := m.cache.RefreshPods(msg.Pods, status)
	for _, c := range deleted {
		m.Info("discovered stale container %s...", c.GetID())
		del = append(del, c)
	}

	added, deleted := m.cache.RefreshContainers(msg.Containers)
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

func (p *nriPlugin) Configure(ctx context.Context, req *nri.ConfigureRequest) (*nri.ConfigureResponse, error) {
	p.dump(req)
	return &nri.ConfigureResponse{
		Id: "zz-resource-manager",
	}, nil
}

func (p *nriPlugin) Synchronize(ctx context.Context, req *nri.SynchronizeRequest) (*nri.SynchronizeResponse, error) {
	p.dump(req)

	m := p.resmgr

	add, del, err := p.syncWithNRI(context.Background(), req)
	if err != nil {
		p.resmgr.Error("failed to synchronize with NRI: %v", err)
		return &nri.SynchronizeResponse{}, nil // err
	}

	if err := m.policy.Start(add, del); err != nil {
		return &nri.SynchronizeResponse{}, errors.Wrapf(err,
			"failed to start policy %s", policy.ActivePolicy())
	}

	return &nri.SynchronizeResponse{
		Updates: p.getPendingUpdates(""),
	}, nil
}

func (p *nriPlugin) Shutdown(ctx context.Context, req *nri.ShutdownRequest) (*nri.ShutdownResponse, error) {
	p.dump(req)
	return &nri.ShutdownResponse{}, nil
}

func (p *nriPlugin) RunPodSandbox(ctx context.Context, req *nri.RunPodSandboxRequest) (*nri.RunPodSandboxResponse, error) {
	m := p.resmgr

	p.dump(req)

	m.Lock()
	defer m.Unlock()

	m.cache.InsertPod(req.Pod.Id, req.Pod, nil)

	return &nri.RunPodSandboxResponse{}, nil
}

func (p *nriPlugin) StopPodSandbox(ctx context.Context, req *nri.StopPodSandboxRequest) (*nri.StopPodSandboxResponse, error) {
	p.dump(req)
	return &nri.StopPodSandboxResponse{}, nil
}

func (p *nriPlugin) RemovePodSandbox(ctx context.Context, req *nri.RemovePodSandboxRequest) (*nri.RemovePodSandboxResponse, error) {
	m := p.resmgr

	p.dump(req)

	m.Lock()
	defer m.Unlock()

	m.cache.DeletePod(req.Pod.Id)

	return &nri.RemovePodSandboxResponse{}, nil
}

func (p *nriPlugin) CreateContainer(ctx context.Context, req *nri.CreateContainerRequest) (*nri.CreateContainerResponse, error) {
	m := p.resmgr

	p.dump(req)

	m.Lock()
	defer m.Unlock()

	container, err := m.cache.InsertContainer(req.Container)
	if err != nil {
		return &nri.CreateContainerResponse{}, errors.Wrap(err, "failed to cache container")
	}

	if err := m.policy.AllocateResources(container); err != nil {
		return &nri.CreateContainerResponse{}, errors.Wrap(err, "failed to allocate resources")
	}

	return &nri.CreateContainerResponse{
		Create:  p.getPendingCreate(req.Container.Id),
		Updates: p.getPendingUpdates(req.Container.Id),
	}, nil
}

func (p *nriPlugin) StartContainer(ctx context.Context, req *nri.StartContainerRequest) (*nri.StartContainerResponse, error) {
	p.dump(req)
	return &nri.StartContainerResponse{}, nil
}

func (p *nriPlugin) UpdateContainer(ctx context.Context, req *nri.UpdateContainerRequest) (*nri.UpdateContainerResponse, error) {
	p.dump(req)
	return &nri.UpdateContainerResponse{}, nil
}

func (p *nriPlugin) StopContainer(ctx context.Context, req *nri.StopContainerRequest) (*nri.StopContainerResponse, error) {
	m := p.resmgr

	p.dump(req)

	m.Lock()
	defer m.Unlock()

	container, ok := m.cache.LookupContainer(req.Container.Id)
	if !ok {
		return &nri.StopContainerResponse{}, nil
	}

	if err := m.policy.ReleaseResources(container); err != nil {
		return &nri.StopContainerResponse{}, errors.Wrap(err, "failed to release resources")
	}

	container.UpdateState(cache.ContainerStateExited)

	updates := p.getPendingUpdates(req.Container.Id)

	return &nri.StopContainerResponse{
		Updates: updates,
	}, nil
}

func (p *nriPlugin) RemoveContainer(ctx context.Context, req *nri.RemoveContainerRequest) (*nri.RemoveContainerResponse, error) {
	m := p.resmgr

	m.Lock()
	defer m.Unlock()

	p.dump(req)

	m.cache.DeleteContainer(req.Container.Id)

	return &nri.RemoveContainerResponse{}, nil
}

func (p *nriPlugin) getPendingCreate(id string) *nri.ContainerCreateUpdate {
	m := p.resmgr
	container, ok := m.cache.LookupContainer(id)
	if !ok {
		return &nri.ContainerCreateUpdate{}
	}

	envs := []*nri.KeyValue{}
	for _, k := range container.GetEnvKeys() {
		v, _ := container.GetEnv(k)
		envs = append(envs, &nri.KeyValue{
			Key:   k,
			Value: v,
		})
	}

	mounts := []*nri.Mount{}
	for _, mnt := range container.GetMounts() {
		var propagation nri.MountPropagation
		switch mnt.Propagation {
		case cache.MountPrivate:
			propagation = nri.MountPropagation_PROPAGATION_PRIVATE
		case cache.MountHostToContainer:
			propagation = nri.MountPropagation_PROPAGATION_HOST_TO_CONTAINER
		case cache.MountBidirectional:
			propagation = nri.MountPropagation_PROPAGATION_BIDIRECTIONAL
		}
		mounts = append(mounts, &nri.Mount{
			ContainerPath:  mnt.Container,
			HostPath:       mnt.Host,
			Readonly:       mnt.Readonly,
			SelinuxRelabel: mnt.Relabel,
			Propagation:    propagation,
		})
	}

	devices := []*nri.Device{}
	for _, d := range container.GetDevices() {
		devices = append(devices, &nri.Device{
			ContainerPath: d.Container,
			HostPath:      d.Host,
			Permissions:   d.Permissions,
		})
	}

	for _, ctrl := range container.GetPending() {
		container.ClearPending(ctrl)
	}

	return &nri.ContainerCreateUpdate{
		LinuxResources: &nri.LinuxContainerResources{
			CpuPeriod:  container.GetCPUPeriod(),
			CpuQuota:   container.GetCPUQuota(),
			CpuShares:  container.GetCPUShares(),
			CpusetCpus: container.GetCpusetCpus(),
			CpusetMems: container.GetCpusetMems(),
		},
		Labels:      container.GetLabels(),
		Annotations: container.GetAnnotations(),
		Envs:        envs,
		Mounts:      mounts,
		Devices:     devices,
	}
}

func (p *nriPlugin) getPendingUpdates(id string) []*nri.ContainerUpdate {
	m := p.resmgr
	updates := []*nri.ContainerUpdate{}
	for _, container := range m.cache.GetPendingContainers() {
		containerID := container.GetID()
		if containerID == id {
			continue
		}
		updates = append(updates, &nri.ContainerUpdate{
			ContainerId: containerID,
			LinuxResources: &nri.LinuxContainerResources{
				CpuPeriod:  container.GetCPUPeriod(),
				CpuQuota:   container.GetCPUQuota(),
				CpuShares:  container.GetCPUShares(),
				CpusetCpus: container.GetCpusetCpus(),
				CpusetMems: container.GetCpusetMems(),
			},
		})
		for _, ctrl := range container.GetPending() {
			container.ClearPending(ctrl)
		}
	}
	return updates
}

func (p *nriPlugin) dump(obj interface{}) {
	msg, err := yaml.Marshal(obj)
	if err != nil {
		return
	}
	prefix := fmt.Sprintf("%T", obj)
	if split := strings.Split(prefix, "."); len(split) > 1 {
		prefix = split[len(split)-1]
	}
	p.resmgr.InfoBlock("NRI-"+strings.TrimSuffix(prefix, "Request"), "%s", msg)
}

// socketListener implements net.Listener for an already connected socket.
type socketListener struct {
	next chan net.Conn
	conn net.Conn
}

// newConnListener returns a listener for the given socket connection.
func newConnListener(conn net.Conn) *socketListener {
	next := make(chan net.Conn, 1)
	next <- conn

	return &socketListener{
		next: next,
		conn: conn,
	}
}

// Accept implements net.Listener.Accept() for a socketListener.
func (sl *socketListener) Accept() (net.Conn, error) {
	conn := <-sl.next
	if conn == nil {
		return nil, io.EOF
	}

	return conn, nil
}

// Close implements net.Listener.Close() for a socketListener.
func (sl *socketListener) Close() error {
	sl.conn.Close()
	close(sl.next)
	return nil
}

// Addr implements net.Listener.Addr() for a socketListener.
func (sl *socketListener) Addr() net.Addr {
	return sl.conn.LocalAddr()
}
