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

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/intel/cri-resource-manager/pkg/utils"
	"google.golang.org/grpc"
	api "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type fakeCriServer struct {
	t            *testing.T
	socket       string
	grpcServer   *grpc.Server
	fakeHandlers map[string]interface{}
}

func newFakeCriServer(t *testing.T, socket string, fakeHandlers map[string]interface{}) *fakeCriServer {
	t.Helper()

	if !filepath.IsAbs(socket) {
		t.Fatalf("invalid socket %q, absolute path expected", socket)
	}

	if err := os.MkdirAll(filepath.Dir(socket), 0700); err != nil {
		t.Fatalf("failed to create directory for socket %q: %v", socket, err)
	}

	srv := &fakeCriServer{
		t:            t,
		socket:       socket,
		grpcServer:   grpc.NewServer(),
		fakeHandlers: fakeHandlers,
	}

	api.RegisterRuntimeServiceServer(srv.grpcServer, srv)
	api.RegisterImageServiceServer(srv.grpcServer, srv)

	lis, err := net.Listen("unix", socket)
	if err != nil {
		if ls, err := utils.IsListeningSocket(socket); ls || err != nil {
			t.Fatalf("failed to create fake server: socket %s already exists", socket)
		}
		os.Remove(socket)
		lis, err = net.Listen("unix", socket)
		if err != nil {
			t.Fatalf("failed to create fake server on socket %q: %v", socket, err)
		}
	}

	go func() {
		if err := srv.grpcServer.Serve(lis); err != nil {
			fmt.Printf("unable to start gRPC server: %+v\n", err)
		}
	}()

	if err := utils.WaitForServer(socket, time.Second); err != nil {
		t.Fatalf("starting fake CRI server failed: %v", err)
	}

	return srv
}

func (s *fakeCriServer) stop() {
	s.t.Helper()
	s.grpcServer.Stop()
	os.Remove(s.socket)
}

func (s *fakeCriServer) callHandler(ctx context.Context, request interface{}, defaultHandler interface{}) (interface{}, error) {
	var err error

	pc, _, _, _ := runtime.Caller(1)
	nameFull := runtime.FuncForPC(pc).Name()
	nameEnd := filepath.Ext(nameFull)
	name := strings.TrimPrefix(nameEnd, ".")

	handler, found := s.fakeHandlers[name]
	if !found {
		if defaultHandler == nil {
			method := reflect.ValueOf(s).MethodByName(name)
			returnType := method.Type().Out(0)
			return reflect.New(returnType).Elem().Interface(), fmt.Errorf("%s() not implemented", name)
		}

		handler = defaultHandler
	}

	in := make([]reflect.Value, 3)
	in[0] = reflect.ValueOf(s)
	in[1] = reflect.ValueOf(ctx)
	in[2] = reflect.ValueOf(request)
	out := reflect.ValueOf(handler).Call(in)

	if !out[1].IsNil() {
		err = out[1].Interface().(error)
	}

	return out[0].Interface(), err
}

// Implementation of api.RuntimeServiceServer

func (s *fakeCriServer) Version(ctx context.Context, req *api.VersionRequest) (*api.VersionResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.VersionResponse), err
}

func (s *fakeCriServer) RunPodSandbox(ctx context.Context, req *api.RunPodSandboxRequest) (*api.RunPodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.RunPodSandboxResponse), err
}

func (s *fakeCriServer) StopPodSandbox(ctx context.Context, req *api.StopPodSandboxRequest) (*api.StopPodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.StopPodSandboxResponse), err
}

func (s *fakeCriServer) RemovePodSandbox(ctx context.Context, req *api.RemovePodSandboxRequest) (*api.RemovePodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.RemovePodSandboxResponse), err
}

func (s *fakeCriServer) PodSandboxStatus(ctx context.Context, req *api.PodSandboxStatusRequest) (*api.PodSandboxStatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.PodSandboxStatusResponse), err
}

func (s *fakeCriServer) ListPodSandbox(ctx context.Context, req *api.ListPodSandboxRequest) (*api.ListPodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, func(*fakeCriServer, context.Context, *api.ListPodSandboxRequest) (*api.ListPodSandboxResponse, error) {
		return &api.ListPodSandboxResponse{}, nil
	})
	return response.(*api.ListPodSandboxResponse), err
}

func (s *fakeCriServer) CreateContainer(ctx context.Context, req *api.CreateContainerRequest) (*api.CreateContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.CreateContainerResponse), err
}

func (s *fakeCriServer) StartContainer(ctx context.Context, req *api.StartContainerRequest) (*api.StartContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.StartContainerResponse), err
}

func (s *fakeCriServer) StopContainer(ctx context.Context, req *api.StopContainerRequest) (*api.StopContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.StopContainerResponse), err
}

func (s *fakeCriServer) RemoveContainer(ctx context.Context, req *api.RemoveContainerRequest) (*api.RemoveContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.RemoveContainerResponse), err
}

func (s *fakeCriServer) ListContainers(ctx context.Context, req *api.ListContainersRequest) (*api.ListContainersResponse, error) {
	response, err := s.callHandler(ctx, req, func(*fakeCriServer, context.Context, *api.ListContainersRequest) (*api.ListContainersResponse, error) {
		return &api.ListContainersResponse{}, nil
	})
	return response.(*api.ListContainersResponse), err
}

func (s *fakeCriServer) ContainerStatus(ctx context.Context, req *api.ContainerStatusRequest) (*api.ContainerStatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ContainerStatusResponse), err
}

func (s *fakeCriServer) UpdateContainerResources(ctx context.Context, req *api.UpdateContainerResourcesRequest) (*api.UpdateContainerResourcesResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.UpdateContainerResourcesResponse), err
}

func (s *fakeCriServer) ReopenContainerLog(ctx context.Context, req *api.ReopenContainerLogRequest) (*api.ReopenContainerLogResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ReopenContainerLogResponse), err
}

func (s *fakeCriServer) ExecSync(ctx context.Context, req *api.ExecSyncRequest) (*api.ExecSyncResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ExecSyncResponse), err
}

func (s *fakeCriServer) Exec(ctx context.Context, req *api.ExecRequest) (*api.ExecResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ExecResponse), err
}

func (s *fakeCriServer) Attach(ctx context.Context, req *api.AttachRequest) (*api.AttachResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.AttachResponse), err
}

func (s *fakeCriServer) PortForward(ctx context.Context, req *api.PortForwardRequest) (*api.PortForwardResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.PortForwardResponse), err
}

func (s *fakeCriServer) ContainerStats(ctx context.Context, req *api.ContainerStatsRequest) (*api.ContainerStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ContainerStatsResponse), err
}

func (s *fakeCriServer) ListContainerStats(ctx context.Context, req *api.ListContainerStatsRequest) (*api.ListContainerStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ListContainerStatsResponse), err
}

func (s *fakeCriServer) PodSandboxStats(ctx context.Context, req *api.PodSandboxStatsRequest) (*api.PodSandboxStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.PodSandboxStatsResponse), err
}

func (s *fakeCriServer) ListPodSandboxStats(ctx context.Context, req *api.ListPodSandboxStatsRequest) (*api.ListPodSandboxStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ListPodSandboxStatsResponse), err
}

func (s *fakeCriServer) UpdateRuntimeConfig(ctx context.Context, req *api.UpdateRuntimeConfigRequest) (*api.UpdateRuntimeConfigResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.UpdateRuntimeConfigResponse), err
}

func (s *fakeCriServer) Status(ctx context.Context, req *api.StatusRequest) (*api.StatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.StatusResponse), err
}

// Implementation of api.ImageServiceServer

func (s *fakeCriServer) ListImages(ctx context.Context, req *api.ListImagesRequest) (*api.ListImagesResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ListImagesResponse), err
}

func (s *fakeCriServer) ImageStatus(ctx context.Context, req *api.ImageStatusRequest) (*api.ImageStatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ImageStatusResponse), err
}

func (s *fakeCriServer) PullImage(ctx context.Context, req *api.PullImageRequest) (*api.PullImageResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.PullImageResponse), err
}

func (s *fakeCriServer) RemoveImage(ctx context.Context, req *api.RemoveImageRequest) (*api.RemoveImageResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.RemoveImageResponse), err
}

func (s *fakeCriServer) ImageFsInfo(ctx context.Context, req *api.ImageFsInfoRequest) (*api.ImageFsInfoResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*api.ImageFsInfoResponse), err
}
