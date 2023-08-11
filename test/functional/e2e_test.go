// Copyright 2020 Intel Corporation. All Rights Reserved.
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
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"

	resmgr "github.com/intel/cri-resource-manager/pkg/cri/resource-manager"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/dump"
	"google.golang.org/grpc"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	testDir = "/tmp/cri-rm-test"
)

func init() {
	rate := logger.Rate{Limit: logger.Every(1 * time.Minute)}
	logger.SetGrpcLogger("grpc", &rate)

	if err := os.MkdirAll(testDir, 0700); err != nil {
		fmt.Printf("unable to create %q: %+v\n", testDir, err)
	}
}

type testEnv struct {
	t           *testing.T
	handlers    map[string]interface{}
	client      criv1.RuntimeServiceClient
	forceConfig string
	mgr         resmgr.ResourceManager
	cache       cache.Cache
}

func (env *testEnv) Run(name string, testFunction func(context.Context, *testEnv)) {
	t := env.t
	overriddenCriHandlers := env.handlers

	t.Helper()
	t.Run(name, func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(testDir, "requests-")
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
		if err := flag.Set("allow-untested-runtimes", "true"); err != nil {
			t.Fatalf("unable to allow untested runtimes: %v", err)
		}

		if env.forceConfig != "" {
			path := filepath.Join(tmpDir, "forcedconfig.cfg")
			if err := os.WriteFile(path, []byte(env.forceConfig), 0644); err != nil {
				t.Fatalf("failed to create configuration file %s: %v", path, err)
			}
			if err := flag.Set("force-config", path); err != nil {
				t.Fatalf("unable to set force-config")
			}
		}

		flag.Parse()

		fakeCri := newFakeCriServer(t, filepath.Join(tmpDir, "fakecri.sock"), overriddenCriHandlers)
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

		client := criv1.NewRuntimeServiceClient(conn)

		env.client = client
		env.mgr = resMgr
		env.cache = resMgr.GetCache()

		testFunction(ctx, env)

		// until pkg/log fixes gets merged: wait until pkg/dump is done with
		// logging before we run next test (and consequently do a reconfig)
		dump.Sync()
	})
}

func TestListPodSandbox(t *testing.T) {
	tcases := []struct {
		name         string
		pods         []*criv1.PodSandbox
		expectedPods int
	}{
		{
			name: "empty",
		},
		{
			name:         "list one pod",
			pods:         []*criv1.PodSandbox{{}},
			expectedPods: 1,
		},
	}
	for _, tc := range tcases {
		criHandlers := map[string]interface{}{
			"ListPodSandbox": func(*fakeCriServer, context.Context, *criv1.ListPodSandboxRequest) (*criv1.ListPodSandboxResponse, error) {
				return &criv1.ListPodSandboxResponse{
					Items: tc.pods,
				}, nil
			},
		}
		env := &testEnv{
			t:        t,
			handlers: criHandlers,
		}
		env.Run(tc.name, func(ctx context.Context, env *testEnv) {
			t := env.t
			client := env.client
			resp, err := client.ListPodSandbox(ctx, &criv1.ListPodSandboxRequest{})
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
		containers         []*criv1.Container
		expectedContainers int
	}{
		{
			name: "empty",
		},
		{
			name:               "list one container",
			containers:         []*criv1.Container{{}},
			expectedContainers: 1,
		},
	}
	for _, tc := range tcases {
		criHandlers := map[string]interface{}{
			"ListContainers": func(*fakeCriServer, context.Context, *criv1.ListContainersRequest) (*criv1.ListContainersResponse, error) {
				return &criv1.ListContainersResponse{
					Containers: tc.containers,
				}, nil
			},
		}
		env := &testEnv{
			t:        t,
			handlers: criHandlers,
		}
		env.Run(tc.name, func(ctx context.Context, env *testEnv) {
			t := env.t
			client := env.client
			resp, err := client.ListContainers(ctx, &criv1.ListContainersRequest{})
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

func TestLingeringPodCleanup(t *testing.T) {
	cfg := `
policy:
  Active: topology-aware
  ReservedResources:
    CPU: 750m
`
	tcases := []struct {
		name         string
		reqs         []*criv1.RunPodSandboxRequest
		expectedPods int
	}{
		{
			name: "create Pod #1",
			reqs: []*criv1.RunPodSandboxRequest{
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
			},
			expectedPods: 1,
		},
		{
			name: "create Pods #1 and #2",
			reqs: []*criv1.RunPodSandboxRequest{
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
				createPodRequest("Pod#2", "UID#2", "", nil, nil, ""),
			},
			expectedPods: 2,
		},
		{
			name: "create Pods #1, #2, and #3",
			reqs: []*criv1.RunPodSandboxRequest{
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
				createPodRequest("Pod#2", "UID#2", "", nil, nil, ""),
				createPodRequest("Pod#3", "UID#3", "", nil, nil, ""),
			},
			expectedPods: 3,
		},
		{
			name: "create Pods #1, #2, #3, #4, '1, '2, '3",
			reqs: []*criv1.RunPodSandboxRequest{
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
				createPodRequest("Pod#2", "UID#2", "", nil, nil, ""),
				createPodRequest("Pod#3", "UID#3", "", nil, nil, ""),
				createPodRequest("Pod#4", "UID#4", "", nil, nil, ""),
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
				createPodRequest("Pod#2", "UID'2", "", nil, nil, ""),
				createPodRequest("Pod#3", "UID'3", "", nil, nil, ""),
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
				createPodRequest("Pod#2", "UID#2", "", nil, nil, ""),
				createPodRequest("Pod#3", "UID#3", "", nil, nil, ""),
				createPodRequest("Pod#1", "UID'1", "", nil, nil, ""),
				createPodRequest("Pod#2", "UID'2", "", nil, nil, ""),
				createPodRequest("Pod#3", "UID'3", "", nil, nil, ""),
				createPodRequest("Pod#4", "UID#4", "", nil, nil, ""),
			},
			expectedPods: 7,
		},
	}

	numPods := 0
	for _, tc := range tcases {
		criHandlers := map[string]interface{}{
			"RunPodSandbox": func(*fakeCriServer, context.Context, *criv1.RunPodSandboxRequest) (*criv1.RunPodSandboxResponse, error) {
				numPods++
				return &criv1.RunPodSandboxResponse{
					PodSandboxId: fmt.Sprintf("Pod#%d", numPods),
				}, nil
			},
		}
		env := &testEnv{
			t:           t,
			handlers:    criHandlers,
			forceConfig: cfg,
		}
		env.Run(tc.name, func(ctx context.Context, env *testEnv) {
			t := env.t
			client := env.client
			cache := env.cache
			for _, req := range tc.reqs {
				_, err := client.RunPodSandbox(ctx, req)
				if err != nil {
					t.Errorf("failed to create pod %+v: %v", req, err)
				}
			}
			pods := cache.GetPods()
			if len(pods) != tc.expectedPods {
				t.Errorf("expected %d pods in cache, got %d (%v)", tc.expectedPods, len(pods), pods)
			}
		})
	}
}

func TestLingeringContainerCleanup(t *testing.T) {
	cfg := `
policy:
  Active: topology-aware
  ReservedResources:
    CPU: 750m
`
	type pod struct {
		UID string
		ID  string
		req *criv1.RunPodSandboxRequest
	}

	type container struct {
		pod    string
		name   string
		expect int
		req    *criv1.CreateContainerRequest
		ID     string
	}

	tcases := []struct {
		name       string
		pods       []*criv1.RunPodSandboxRequest
		containers []*container
	}{
		{
			name: "create containers per one pod",
			pods: []*criv1.RunPodSandboxRequest{
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
			},
			containers: []*container{
				{pod: "UID#1", name: "Container#1", expect: 1},
				{pod: "UID#1", name: "Container#2", expect: 2},
			},
		},
		{
			name: "create lingering containers per one pod",
			pods: []*criv1.RunPodSandboxRequest{
				createPodRequest("Pod#1", "UID#1", "", nil, nil, ""),
			},
			containers: []*container{
				{pod: "UID#1", name: "Container#1", expect: 1},
				{pod: "UID#1", name: "Container#2", expect: 2},
				{pod: "UID#1", name: "Container#3", expect: 3},
				{pod: "UID#1", name: "Container#3", expect: 3},
				{pod: "UID#1", name: "Container#2", expect: 3},
				{pod: "UID#1", name: "Container#1", expect: 3},
			},
		},
	}

	numPods := 0
	numContainers := 0
	for _, tc := range tcases {
		criHandlers := map[string]interface{}{
			"RunPodSandbox": func(*fakeCriServer, context.Context, *criv1.RunPodSandboxRequest) (*criv1.RunPodSandboxResponse, error) {
				numPods++
				return &criv1.RunPodSandboxResponse{
					PodSandboxId: fmt.Sprintf("Pod#%d", numPods),
				}, nil
			},
			"CreateContainer": func(*fakeCriServer, context.Context, *criv1.CreateContainerRequest) (*criv1.CreateContainerResponse, error) {
				numContainers++
				return &criv1.CreateContainerResponse{
					ContainerId: fmt.Sprintf("Container#%d", numContainers),
				}, nil
			},
		}
		env := &testEnv{
			t:           t,
			handlers:    criHandlers,
			forceConfig: cfg,
		}
		env.Run(tc.name, func(ctx context.Context, env *testEnv) {
			t := env.t
			client := env.client
			cache := env.cache
			pods := map[string]*pod{}

			for _, req := range tc.pods {
				rpl, err := client.RunPodSandbox(ctx, req)
				if err != nil {
					t.Errorf("failed to create pod %+v: %v", req, err)
				} else {
					id := rpl.PodSandboxId
					uid := req.Config.Metadata.Uid
					pods[uid] = &pod{
						UID: uid,
						ID:  id,
						req: req,
					}
				}
			}

			for _, c := range tc.containers {
				pod, ok := pods[c.pod]
				if !ok {
					t.Errorf("failed to find pod by UID %s", c.pod)
					continue
				}

				c.req = createContainerRequest(pod.ID, c.name, pod.req)
				rpl, err := client.CreateContainer(ctx, c.req)
				if err != nil {
					t.Errorf("failed to create container %+v: %v", c.req, err)
				} else {
					c.ID = rpl.ContainerId
					cached := cache.GetContainers()
					if len(cached) != c.expect {
						t.Errorf("pod %s, container %s: expected %d containers in cache, got %d",
							c.pod, c.name, c.expect, len(cached))
					}
				}
			}
		})
	}
}

func createPodRequest(name, uid, namespace string,
	labels, annotations map[string]string,
	cgroupParent string) *criv1.RunPodSandboxRequest {
	if namespace == "" {
		namespace = "default"
	}
	if labels == nil {
		labels = map[string]string{}
	}
	labels[kubetypes.KubernetesPodUIDLabel] = uid
	return &criv1.RunPodSandboxRequest{
		Config: &criv1.PodSandboxConfig{
			Metadata: &criv1.PodSandboxMetadata{
				Name:      name,
				Uid:       uid,
				Namespace: namespace,
			},
			Labels:      labels,
			Annotations: annotations,
			Linux: &criv1.LinuxPodSandboxConfig{
				CgroupParent: cgroupParent,
			},
		},
	}
}

func createContainerRequest(podID, name string,
	podReq *criv1.RunPodSandboxRequest) *criv1.CreateContainerRequest {
	return &criv1.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &criv1.ContainerConfig{
			Metadata: &criv1.ContainerMetadata{
				Name: name,
			},
			Linux: &criv1.LinuxContainerConfig{},
		},
		SandboxConfig: podReq.Config,
	}
}
