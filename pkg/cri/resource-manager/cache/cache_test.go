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
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
)

var nextFakePodID = 1
var nextFakeContainerID = 1

type fakePod struct {
	name        string
	uid         string
	id          string
	qos         v1.PodQOSClass
	labels      map[string]string
	annotations map[string]string
	podCfg      *cri.PodSandboxConfig
}

type fakeContainer struct {
	fakePod     *fakePod
	name        string
	id          string
	labels      map[string]string
	annotations map[string]string
	resources   cri.LinuxContainerResources
}

func createTmpCache() (Cache, string, error) {
	dir, err := ioutil.TempDir("", "cache-test")
	if err != nil {
		return nil, "", err
	}
	cch, err := NewCache(Options{CacheDir: dir})
	if err != nil {
		return nil, "", err
	}
	return cch, dir, nil
}

func removeTmpCache(dir string) {
	if dir != "" {
		os.RemoveAll(dir)
	}
}

func createFakePod(cch Cache, fp *fakePod) (Pod, error) {
	if string(fp.qos) == "" {
		fp.qos = v1.PodQOSBurstable
	}

	if fp.labels == nil {
		fp.labels = make(map[string]string)
	}
	fp.id = fmt.Sprintf("pod%4.4d", nextFakePodID)
	fp.uid = fmt.Sprintf("poduid%4.4d", nextFakePodID)
	fp.labels[kubetypes.KubernetesPodUIDLabel] = fp.uid
	nextFakePodID++

	req := &cri.RunPodSandboxRequest{
		Config: &cri.PodSandboxConfig{
			Metadata: &cri.PodSandboxMetadata{
				Name:      fp.name,
				Uid:       fp.uid,
				Namespace: "default",
			},
			Labels:      fp.labels,
			Annotations: fp.annotations,
			Linux: &cri.LinuxPodSandboxConfig{
				CgroupParent: "/kubepods.slice/kubepods-" + string(fp.qos) + ".slice/" +
					"kubepods-" + string(fp.qos) + "-pod-" + fp.id + ".slice",
			},
		},
	}
	fp.podCfg = req.Config

	cch.(*cache).Debug("*** => creating Pod: %+v\n", *req)
	p := cch.InsertPod(fp.id, req)
	cch.(*cache).Debug("*** <= created Pod: %+v\n", *p.(*pod))
	return p, nil
}

func createFakeContainer(cch Cache, fc *fakeContainer) (Container, error) {
	if fc.labels == nil {
		fc.labels = make(map[string]string)
	}
	fc.id = fmt.Sprintf("container-id-%4.4d", nextFakeContainerID)
	nextFakeContainerID++

	req := &cri.CreateContainerRequest{
		PodSandboxId: fc.fakePod.id,
		Config: &cri.ContainerConfig{
			Metadata: &cri.ContainerMetadata{
				Name: fc.name,
			},
			Labels:      fc.labels,
			Annotations: fc.annotations,
			Linux: &cri.LinuxContainerConfig{
				Resources: &fc.resources,
			},
		},
		SandboxConfig: fc.fakePod.podCfg,
	}

	cch.(*cache).Debug("*** => creating Container: %+v\n", *req)
	c, err := cch.InsertContainer(req)
	if err != nil {
		return nil, err
	}
	cch.(*cache).Debug("*** <= created Container: %+v\n", *c.(*container))
	update := &cri.CreateContainerResponse{ContainerId: fc.id}
	if _, err := cch.UpdateContainerID(c.GetCacheID(), update); err != nil {
		return nil, err
	}
	return c, nil
}

func TestLookupContainerByCgroup(t *testing.T) {
	fakePods := map[string]*fakePod{
		"pod1": {name: "pod1"},
		"pod2": {name: "pod2"},
		"pod3": {name: "pod3"},
	}

	fakePodContainers := map[string][]*fakeContainer{
		"pod1": {{name: "container1"}, {name: "container2"}, {name: "err-container3"}},
		"pod2": {{name: "err-container4"}, {name: "container5"}, {name: "err-container6"}},
		"pod3": {{name: "container7"}, {name: "container8"}, {name: "container10"}},
	}

	cch, dir, err := createTmpCache()
	if err != nil {
		t.Errorf("failed: %v", err)
	}
	defer removeTmpCache(dir)

	for _, fp := range fakePods {
		_, err := createFakePod(cch, fp)
		if err != nil {
			t.Errorf("failed to create fake pod: %v", err)
		}
	}

	for podName, fcs := range fakePodContainers {
		fp, ok := fakePods[podName]
		if !ok {
			t.Errorf("failed to find fake pod '%s'", podName)
		}
		for _, fc := range fcs {
			fc.fakePod = fp
			if _, err := createFakeContainer(cch, fc); err != nil {
				t.Errorf("failed to create fake container '%s.%s': %v", podName, fc.name, err)
			}
		}
	}

	for _, c := range cch.GetContainers() {
		p, ok := c.GetPod()
		if !ok {
			t.Errorf("failed to find Pod for Container %s", c.PrettyName())
		}
		podCgroupDir := p.GetCgroupParentDir()
		path := podCgroupDir + "/container-" + c.GetID() + ".scope"

		cch.(*cache).Info("=> %s: testing lookup by cgroup path %s...", c.PrettyName(), path)
		chk, ok := cch.LookupContainerByCgroup(path)
		if !ok {
			t.Errorf("failed to look up container %s by cgroup path %s (pod parent cgroup: %s)",
				c.PrettyName(), path, podCgroupDir)
		}
		cch.(*cache).Info("<= %s", chk.PrettyName())

		if strings.HasPrefix(c.GetName(), "err-") {
			path := podCgroupDir + "-another/container-" + c.GetID() + ".scope"

			cch.(*cache).Info("=> %s: testing lookup failure by cgroup path %s...",
				c.PrettyName(), path)
			chk, ok := cch.LookupContainerByCgroup(path)
			if ok {
				t.Errorf("look up of container %s by path %s should have failed, but gave %s",
					c.PrettyName(), path, chk.PrettyName())
			}
			cch.(*cache).Info("<= OK (not found as expected)")
		}

		if chk.GetID() != c.GetID() {
			t.Errorf("found container %s is not the expected %s", chk.GetID(), c.GetID())
		}
	}
}
