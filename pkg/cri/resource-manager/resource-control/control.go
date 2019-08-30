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

package resourcecontrol

import (
	"fmt"
	"strconv"

	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/rdt"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

// CriRdt is the RDT control interface for resource-manager
type CriRdt interface {
	rdt.Control

	SetContainerClass(cache.Container, string) error
}

type criRdt struct {
	rdt.Control
}

// NewCriRdt creates a new CriRdt instance
func NewCriRdt(resctrlpath string, config string) (CriRdt, error) {
	var err error
	c := &criRdt{}
	c.Control, err = rdt.NewControl(resctrlpath, config)
	return c, err

}

// SetContainerClass assigns all processes of a container into am RDT class
func (c *criRdt) SetContainerClass(container cache.Container, class string) error {
	cID := container.GetID()
	pod, ok := container.GetPod()
	if !ok {
		return controlError("Pod of container %q not found", cID)
	}
	cgroupParent := pod.GetCgroupParentDir()

	pids, err := utils.GetProcessInContainer(cgroupParent, cID)
	if err != nil {
		return controlError("failed to get PIDs of container %s: %v", cID, err)
	}

	for _, pid := range pids {
		if err = c.SetProcessClass(class, strconv.Itoa(pid)); err != nil {
			return err
		}
	}
	return nil
}

func controlError(format string, args ...interface{}) error {
	return fmt.Errorf("resource-control: "+format, args...)
}
