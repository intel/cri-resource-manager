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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	core_v1 "k8s.io/api/core/v1"

	agent_v1 "github.com/intel/cri-resource-manager/pkg/agent/api/v1"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config"
)

const (
	SocketDisabled = "disabled"
)

// Interface describe interfaces of cri-resource-manager agent
type Interface interface {
	IsDisabled() bool

	GetNode(time.Duration) (core_v1.Node, error)
	PatchNode([]*agent_v1.JsonPatch, time.Duration) error
	UpdateNodeCapacity(map[string]string, time.Duration) error
	GetConfig(time.Duration) (*config.RawConfig, error)

	GetLabels(time.Duration) (map[string]string, error)
	SetLabels(map[string]string, time.Duration) error
	RemoveLabels([]string, time.Duration) error

	GetAnnotations(time.Duration) (map[string]string, error)
	SetAnnotations(map[string]string, time.Duration) error
	RemoveAnnotations([]string, time.Duration) error

	GetTaints(time.Duration) ([]core_v1.Taint, error)
	SetTaints([]core_v1.Taint, time.Duration) error
	RemoveTaints([]core_v1.Taint, time.Duration) error

	FindTaintIndex([]core_v1.Taint, *core_v1.Taint) (int, bool)
}

// agentInterface implements Interface
type agentInterface struct {
	socket string
	cli    agent_v1.AgentClient
}

// NewAgentInterface connects to cri-resource-manager-agent gRPC server
// and return a new Interface
func NewAgentInterface(socket string) (Interface, error) {
	a := &agentInterface{
		socket: socket,
	}

	if a.IsDisabled() {
		return a, nil
	}

	dialOpts := []grpc.DialOption{
		//		grpc.WithBlock(),
		//		grpc.WithTimeout(10 * time.Second),
		grpc.WithInsecure(),
		//		grpc.FailOnNonTempDialError(true),
		grpc.WithDialer(func(sock string, timeout time.Duration) (net.Conn, error) {
			return net.Dial("unix", sock)
		}),
	}
	conn, err := grpc.Dial(socket, dialOpts...)
	if err != nil {
		return nil, agentError("failed to connect to cri-resmgr agent: %v", err)
	}
	a.cli = agent_v1.NewAgentClient(conn)

	return a, nil
}

// IsDisabled returns true if the agent interface is disabled.
func (a *agentInterface) IsDisabled() bool {
	return a.socket == SocketDisabled || a.socket == ""
}

func (a *agentInterface) GetNode(timeout time.Duration) (core_v1.Node, error) {
	if a.IsDisabled() {
		return core_v1.Node{}, agentError("agent interface is disabled")
	}

	ctx, cancel, callOpts := prepareCall(timeout)
	defer cancel()

	req := &agent_v1.GetNodeRequest{}

	node := core_v1.Node{}
	rsp, err := a.cli.GetNode(ctx, req, callOpts...)
	if err != nil {
		return node, agentError("failed to get node object: %v", err)
	}

	if err = json.Unmarshal([]byte(rsp.Node), &node); err != nil {
		return node, agentError("invalid response, failed to unmarshal v1.Node: %v", err)
	}

	return node, nil
}

func (a *agentInterface) PatchNode(patches []*agent_v1.JsonPatch, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	ctx, cancel, callOpts := prepareCall(timeout)
	defer cancel()

	req := &agent_v1.PatchNodeRequest{
		Patches: patches,
	}

	_, err := a.cli.PatchNode(ctx, req, callOpts...)
	if err != nil {
		return agentError("failed to patch node object: %v", err)
	}
	return nil
}

func (a *agentInterface) UpdateNodeCapacity(caps map[string]string, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	ctx, cancel, callOpts := prepareCall(timeout)
	defer cancel()

	req := &agent_v1.UpdateNodeCapacityRequest{
		Capacities: caps,
	}
	_, err := a.cli.UpdateNodeCapacity(ctx, req, callOpts...)
	if err != nil {
		return agentError("failed to update node capacities: %v", err)
	}
	return nil
}

func (a *agentInterface) GetConfig(timeout time.Duration) (*config.RawConfig, error) {
	if a.IsDisabled() {
		return nil, agentError("agent interface is disabled")
	}

	ctx, cancel, callOpts := prepareCall(timeout)
	defer cancel()

	rpl, err := a.cli.GetConfig(ctx, &agent_v1.GetConfigRequest{}, callOpts...)
	if err != nil {
		return nil, agentError("failed to get config: %v", err)
	}

	return &config.RawConfig{NodeName: rpl.NodeName, Data: rpl.Config}, nil
}

const (
	// PatchAdd specifies an add operation.
	PatchAdd string = "add"
	// PatchRemove specifies an remove operation.
	PatchRemove string = "remove"
	// PatchReplace specifies an replace operation.
	PatchReplace string = "replace"
)

func patchPath(class, key string) string {
	return "/metadata/" + class + "/" + strings.Replace(key, "/", "~1", -1)
}

func labelPatchPath(key string) string {
	return patchPath("labels", key)
}

func annotationPatchPath(key string) string {
	return patchPath("annotations", key)
}

func taintPatchPath(idx int) string {
	return fmt.Sprintf("/spec/taints/%d", idx)
}

func (a *agentInterface) GetLabels(timeout time.Duration) (map[string]string, error) {
	if a.IsDisabled() {
		return nil, agentError("agent interface is disabled")
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return nil, err
	}

	return node.Labels, nil
}

func (a *agentInterface) SetLabels(labels map[string]string, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	if len(labels) == 0 {
		return nil
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return err
	}

	patches := []*agent_v1.JsonPatch{}
	for key, val := range labels {
		patch := &agent_v1.JsonPatch{
			Path: labelPatchPath(key),
			// Value is supposed to be in marshalled JSON format. Thus, we need
			// to add quotes so that it will be interpreted as a string.
			Value: "\"" + val + "\"",
		}
		if _, ok := node.Labels[key]; ok {
			patch.Op = PatchReplace
		} else {
			patch.Op = PatchAdd
		}
		patches = append(patches, patch)
	}

	return a.PatchNode(patches, timeout)
}

func (a *agentInterface) RemoveLabels(keys []string, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	if len(keys) == 0 {
		return nil
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return err
	}

	patches := []*agent_v1.JsonPatch{}
	for _, key := range keys {
		if _, ok := node.Labels[key]; !ok {
			continue
		}
		patch := &agent_v1.JsonPatch{
			Op:   PatchRemove,
			Path: labelPatchPath(key),
		}
		patches = append(patches, patch)
	}
	if len(patches) == 0 {
		return nil
	}

	return a.PatchNode(patches, timeout)
}

func (a *agentInterface) GetAnnotations(timeout time.Duration) (map[string]string, error) {
	if a.IsDisabled() {
		return nil, agentError("agent interface is disabled")
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return nil, err
	}
	return node.Annotations, nil
}

func (a *agentInterface) SetAnnotations(annotations map[string]string, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	if len(annotations) == 0 {
		return nil
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return err
	}

	patches := []*agent_v1.JsonPatch{}
	for key, val := range annotations {
		patch := &agent_v1.JsonPatch{
			Path:  annotationPatchPath(key),
			Value: val,
		}
		if _, ok := node.Annotations[key]; ok {
			patch.Op = PatchReplace
		} else {
			patch.Op = PatchAdd
		}
		patches = append(patches, patch)
	}

	return a.PatchNode(patches, timeout)
}

func (a *agentInterface) RemoveAnnotations(keys []string, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	if len(keys) == 0 {
		return nil
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return err
	}

	patches := []*agent_v1.JsonPatch{}
	for _, key := range keys {
		if _, ok := node.Annotations[key]; !ok {
			continue
		}

		patch := &agent_v1.JsonPatch{
			Op:   PatchRemove,
			Path: annotationPatchPath(key),
		}
		patches = append(patches, patch)
	}
	if len(patches) == 0 {
		return nil
	}

	return a.PatchNode(patches, timeout)
}

func (a *agentInterface) GetTaints(timeout time.Duration) ([]core_v1.Taint, error) {
	if a.IsDisabled() {
		return nil, agentError("agent interface is disabled")
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return nil, err
	}
	return node.Spec.Taints, nil
}

func (a *agentInterface) SetTaints(taints []core_v1.Taint, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	if len(taints) == 0 {
		return nil
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return err
	}

	patches := []*agent_v1.JsonPatch{}
	if node.Spec.Taints == nil {
		patch := &agent_v1.JsonPatch{
			Op:    PatchAdd,
			Path:  "/spec/taints",
			Value: "[]"}
		patches = append(patches, patch)
	}

	for _, t := range taints {
		value, err := json.Marshal(t)
		if err != nil {
			return agentError("BUG: failed to marshal taint %v: %v", t, err)
		}
		idx, found := findTaintIndex(node.Spec.Taints, &t)
		patch := &agent_v1.JsonPatch{Value: string(value)}
		patch.Path = taintPatchPath(idx)
		if !found {
			patch.Op = PatchAdd
		} else {
			patch.Op = PatchReplace
		}
		patches = append(patches, patch)
	}

	return a.PatchNode(patches, timeout)
}

func (a *agentInterface) RemoveTaints(taints []core_v1.Taint, timeout time.Duration) error {
	if a.IsDisabled() {
		return agentError("agent interface is disabled")
	}

	if len(taints) == 0 {
		return nil
	}

	node, err := a.GetNode(timeout)
	if err != nil {
		return err
	}
	if node.Spec.Taints == nil {
		return nil
	}

	patches := []*agent_v1.JsonPatch{}
	for _, t := range taints {
		idx, found := findTaintIndex(node.Spec.Taints, &t)
		if found {
			patch := &agent_v1.JsonPatch{
				Op:   "remove",
				Path: taintPatchPath(idx),
			}
			patches = append(patches, patch)
		}
	}
	if len(patches) == 0 {
		return nil
	}

	return a.PatchNode(patches, timeout)
}

func findTaintIndex(taints []core_v1.Taint, taint *core_v1.Taint) (int, bool) {
	for idx, t := range taints {
		if t.Key == taint.Key && t.Value == taint.Value && t.Effect == taint.Effect {
			return idx, true
		}
	}
	return 0, false
}

func (a *agentInterface) FindTaintIndex(taints []core_v1.Taint, taint *core_v1.Taint) (int, bool) {
	return findTaintIndex(taints, taint)
}

func agentError(format string, args ...interface{}) error {
	return fmt.Errorf("agent-client: "+format, args...)
}

func prepareCall(timeout time.Duration) (context.Context, context.CancelFunc, []grpc.CallOption) {
	callOpts := []grpc.CallOption{grpc.FailFast(false)}
	ctx := context.Background()
	cancel := func() {}
	if timeout >= 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	}

	return ctx, cancel, callOpts
}
