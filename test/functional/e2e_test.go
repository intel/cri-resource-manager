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
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	resmgr "github.com/intel/cri-resource-manager/pkg/cri/resource-manager"
	"google.golang.org/grpc"
	api "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	testDir = "/tmp/cri-rm-test"
)

func init() {
	if err := os.MkdirAll(testDir, 0700); err != nil {
		fmt.Printf("unable to create %q: %+v\n", testDir, err)
	}
}

func runTest(t *testing.T, name string, overridenCriHandlers map[string]interface{}, testFunction func(*testing.T, api.RuntimeServiceClient, context.Context)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		tmpDir, err := ioutil.TempDir(testDir, "requests-")
		if err != nil {
			t.Fatalf("unable to create temp directory: %+v", err)
		}
		defer os.RemoveAll(tmpDir)

		if err := flag.Set("runtime-socket", filepath.Join(tmpDir, "fakecri.sock")); err != nil {
			t.Fatalf("unable to set runtime-socket")
		}
		if err := flag.Set("image-socket", filepath.Join(tmpDir, "fakecri.sock")); err != nil {
			t.Fatalf("unable to set image-socket")
		}
		if err := flag.Set("relay-socket", filepath.Join(tmpDir, "relay.sock")); err != nil {
			t.Fatalf("unable to set relay-socket")
		}
		if err := flag.Set("relay-dir", filepath.Join(tmpDir, "relaystorage")); err != nil {
			t.Fatalf("unable to set relay-dir")
		}
		if err := flag.Set("agent-socket", filepath.Join(tmpDir, "agent.sock")); err != nil {
			t.Fatalf("unable to set agent-socket")
		}
		if err := flag.Set("config-socket", filepath.Join(tmpDir, "config.sock")); err != nil {
			t.Fatalf("unable to set config-socket")
		}
		if err := flag.Set("logger-debug", "*"); err != nil {
			t.Fatalf("unable to set logger-debug")
		}
		flag.Parse()

		fakeCri := newFakeCriServer(t, filepath.Join(tmpDir, "fakecri.sock"), overridenCriHandlers)
		defer fakeCri.stop()

		resMgr, err := resmgr.NewResourceManager()
		if err != nil {
			t.Fatalf("unable to create resource manager: %+v", err)
		}
		if err := resMgr.Start(); err != nil {
			t.Fatalf("unable to start resource manager: %+v", err)
		}
		defer resMgr.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, filepath.Join(tmpDir, "relay.sock"), grpc.WithInsecure(), grpc.WithBlock(),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				if deadline, ok := ctx.Deadline(); ok {
					return net.DialTimeout("unix", addr, time.Until(deadline))
				}
				return net.DialTimeout("unix", addr, 0)
			}),
		)
		if err != nil {
			t.Fatalf("unable to connect to relay: %+v", err)
		}
		defer conn.Close()

		client := api.NewRuntimeServiceClient(conn)

		testFunction(t, client, ctx)
	})
}

func TestListPodSandbox(t *testing.T) {
	tcases := []struct {
		name         string
		pods         []*api.PodSandbox
		expectedPods int
	}{
		{
			name: "empty",
		},
		{
			name:         "list one pod",
			pods:         []*api.PodSandbox{{}},
			expectedPods: 1,
		},
	}
	for _, tc := range tcases {
		criHandlers := map[string]interface{}{
			"ListPodSandbox": func(*fakeCriServer, context.Context, *api.ListPodSandboxRequest) (*api.ListPodSandboxResponse, error) {
				return &api.ListPodSandboxResponse{
					Items: tc.pods,
				}, nil
			},
		}
		runTest(t, tc.name, criHandlers, func(t *testing.T, client api.RuntimeServiceClient, ctx context.Context) {
			resp, err := client.ListPodSandbox(ctx, &api.ListPodSandboxRequest{})
			if err != nil {
				t.Errorf("Unexpected error: %+v", err)
				return
			}
			if len(resp.Items) != tc.expectedPods {
				t.Errorf("Expected %d pods, got %d", tc.expectedPods, len(resp.Items))
			}
		})
	}
}

func TestListContainers(t *testing.T) {
	tcases := []struct {
		name               string
		containers         []*api.Container
		expectedContainers int
	}{
		{
			name: "empty",
		},
		{
			name:               "list one container",
			containers:         []*api.Container{{}},
			expectedContainers: 1,
		},
	}
	for _, tc := range tcases {
		criHandlers := map[string]interface{}{
			"ListContainers": func(*fakeCriServer, context.Context, *api.ListContainersRequest) (*api.ListContainersResponse, error) {
				return &api.ListContainersResponse{
					Containers: tc.containers,
				}, nil
			},
		}
		runTest(t, tc.name, criHandlers, func(t *testing.T, client api.RuntimeServiceClient, ctx context.Context) {
			resp, err := client.ListContainers(ctx, &api.ListContainersRequest{})
			if err != nil {
				t.Errorf("Unexpected error: %+v", err)
				return
			}
			if len(resp.Containers) != tc.expectedContainers {
				t.Errorf("Expected %d pods, got %d", tc.expectedContainers, len(resp.Containers))
			}
		})
	}
}
