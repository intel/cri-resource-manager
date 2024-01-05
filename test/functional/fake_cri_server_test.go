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
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	fakeKubeAPIVersion    = "0.1.0"
	fakeRuntimeName       = "fake-CRI-runtime"
	fakeRuntimeVersion    = "v0.0.0"
	fakeRuntimeAPIVersion = "v1"
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

	criv1.RegisterRuntimeServiceServer(srv.grpcServer, srv)
	criv1.RegisterImageServiceServer(srv.grpcServer, srv)

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

// Implementation of criv1.RuntimeServiceServer

func (s *fakeCriServer) Version(ctx context.Context, req *criv1.VersionRequest) (*criv1.VersionResponse, error) {
	response, err := s.callHandler(ctx, req,
		func(*fakeCriServer, context.Context, *criv1.VersionRequest) (*criv1.VersionResponse, error) {
			return &criv1.VersionResponse{
				Version:           fakeKubeAPIVersion,
				RuntimeName:       fakeRuntimeName,
				RuntimeVersion:    fakeRuntimeVersion,
				RuntimeApiVersion: fakeRuntimeAPIVersion,
			}, nil
		},
	)
	return response.(*criv1.VersionResponse), err
}

func (s *fakeCriServer) RunPodSandbox(ctx context.Context, req *criv1.RunPodSandboxRequest) (*criv1.RunPodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.RunPodSandboxResponse), err
}

func (s *fakeCriServer) StopPodSandbox(ctx context.Context, req *criv1.StopPodSandboxRequest) (*criv1.StopPodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.StopPodSandboxResponse), err
}

func (s *fakeCriServer) RemovePodSandbox(ctx context.Context, req *criv1.RemovePodSandboxRequest) (*criv1.RemovePodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.RemovePodSandboxResponse), err
}

func (s *fakeCriServer) PodSandboxStatus(ctx context.Context, req *criv1.PodSandboxStatusRequest) (*criv1.PodSandboxStatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.PodSandboxStatusResponse), err
}

func (s *fakeCriServer) ListPodSandbox(ctx context.Context, req *criv1.ListPodSandboxRequest) (*criv1.ListPodSandboxResponse, error) {
	response, err := s.callHandler(ctx, req, func(*fakeCriServer, context.Context, *criv1.ListPodSandboxRequest) (*criv1.ListPodSandboxResponse, error) {
		return &criv1.ListPodSandboxResponse{}, nil
	})
	return response.(*criv1.ListPodSandboxResponse), err
}

func (s *fakeCriServer) CreateContainer(ctx context.Context, req *criv1.CreateContainerRequest) (*criv1.CreateContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.CreateContainerResponse), err
}

func (s *fakeCriServer) StartContainer(ctx context.Context, req *criv1.StartContainerRequest) (*criv1.StartContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.StartContainerResponse), err
}

func (s *fakeCriServer) StopContainer(ctx context.Context, req *criv1.StopContainerRequest) (*criv1.StopContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.StopContainerResponse), err
}

func (s *fakeCriServer) RemoveContainer(ctx context.Context, req *criv1.RemoveContainerRequest) (*criv1.RemoveContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.RemoveContainerResponse), err
}

func (s *fakeCriServer) ListContainers(ctx context.Context, req *criv1.ListContainersRequest) (*criv1.ListContainersResponse, error) {
	response, err := s.callHandler(ctx, req, func(*fakeCriServer, context.Context, *criv1.ListContainersRequest) (*criv1.ListContainersResponse, error) {
		return &criv1.ListContainersResponse{}, nil
	})
	return response.(*criv1.ListContainersResponse), err
}

func (s *fakeCriServer) ContainerStatus(ctx context.Context, req *criv1.ContainerStatusRequest) (*criv1.ContainerStatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ContainerStatusResponse), err
}

func (s *fakeCriServer) UpdateContainerResources(ctx context.Context, req *criv1.UpdateContainerResourcesRequest) (*criv1.UpdateContainerResourcesResponse, error) {
	response, err := s.callHandler(ctx, req,
		func(*fakeCriServer, context.Context, *criv1.UpdateContainerResourcesRequest) (*criv1.UpdateContainerResourcesResponse, error) {
			return &criv1.UpdateContainerResourcesResponse{}, nil
		},
	)
	return response.(*criv1.UpdateContainerResourcesResponse), err
}

func (s *fakeCriServer) ReopenContainerLog(ctx context.Context, req *criv1.ReopenContainerLogRequest) (*criv1.ReopenContainerLogResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ReopenContainerLogResponse), err
}

func (s *fakeCriServer) ExecSync(ctx context.Context, req *criv1.ExecSyncRequest) (*criv1.ExecSyncResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ExecSyncResponse), err
}

func (s *fakeCriServer) Exec(ctx context.Context, req *criv1.ExecRequest) (*criv1.ExecResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ExecResponse), err
}

func (s *fakeCriServer) Attach(ctx context.Context, req *criv1.AttachRequest) (*criv1.AttachResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.AttachResponse), err
}

func (s *fakeCriServer) PortForward(ctx context.Context, req *criv1.PortForwardRequest) (*criv1.PortForwardResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.PortForwardResponse), err
}

func (s *fakeCriServer) ContainerStats(ctx context.Context, req *criv1.ContainerStatsRequest) (*criv1.ContainerStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ContainerStatsResponse), err
}

func (s *fakeCriServer) ListContainerStats(ctx context.Context, req *criv1.ListContainerStatsRequest) (*criv1.ListContainerStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ListContainerStatsResponse), err
}

func (s *fakeCriServer) PodSandboxStats(ctx context.Context, req *criv1.PodSandboxStatsRequest) (*criv1.PodSandboxStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.PodSandboxStatsResponse), err
}

func (s *fakeCriServer) ListPodSandboxStats(ctx context.Context, req *criv1.ListPodSandboxStatsRequest) (*criv1.ListPodSandboxStatsResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ListPodSandboxStatsResponse), err
}

func (s *fakeCriServer) UpdateRuntimeConfig(ctx context.Context, req *criv1.UpdateRuntimeConfigRequest) (*criv1.UpdateRuntimeConfigResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.UpdateRuntimeConfigResponse), err
}

func (s *fakeCriServer) Status(ctx context.Context, req *criv1.StatusRequest) (*criv1.StatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.StatusResponse), err
}

func (s *fakeCriServer) CheckpointContainer(ctx context.Context, req *criv1.CheckpointContainerRequest) (*criv1.CheckpointContainerResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.CheckpointContainerResponse), err
}

func (s *fakeCriServer) GetContainerEvents(_ *criv1.GetEventsRequest, _ criv1.RuntimeService_GetContainerEventsServer) error {
	return nil
}

// Implementation of criv1.ImageServiceServer

func (s *fakeCriServer) ListImages(ctx context.Context, req *criv1.ListImagesRequest) (*criv1.ListImagesResponse, error) {
	response, err := s.callHandler(ctx, req,
		func(*fakeCriServer, context.Context, *criv1.ListImagesRequest) (*criv1.ListImagesResponse, error) {
			return &criv1.ListImagesResponse{}, nil
		},
	)
	return response.(*criv1.ListImagesResponse), err
}

func (s *fakeCriServer) ImageStatus(ctx context.Context, req *criv1.ImageStatusRequest) (*criv1.ImageStatusResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ImageStatusResponse), err
}

func (s *fakeCriServer) PullImage(ctx context.Context, req *criv1.PullImageRequest) (*criv1.PullImageResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.PullImageResponse), err
}

func (s *fakeCriServer) RemoveImage(ctx context.Context, req *criv1.RemoveImageRequest) (*criv1.RemoveImageResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.RemoveImageResponse), err
}

func (s *fakeCriServer) ImageFsInfo(ctx context.Context, req *criv1.ImageFsInfoRequest) (*criv1.ImageFsInfoResponse, error) {
	response, err := s.callHandler(ctx, req, nil)
	return response.(*criv1.ImageFsInfoResponse), err
}
