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

package rdt

var configHelp = `
Resource Manager RDT enforcement controller.

The RDT controller enforces container policy RDT decisions. The controller
takes the assigned RDT class of a container and puts all processes in the
containers' cgroup to that class using the resctrl pseudo-filesystem. If
the container has not assigned RDT class, the RDT controller uses the QOS
class as the RDT class.

The controller can be configured to map the containers' assigned class to
a final RDT class before the RDT-level assignment takes place.

Here is a sample configuration fragment for this controller which sets up
mappings for the 3 Kubernetes QoS classes and defines also a default class.

  rdt:
    ResctrlPath: /sys/fs/resctrl
    Classes:
      Guaranteed: BestRDT
      Burstable: ModerateRDT
      BestEffort: RestrictedRDT
      "*": RestrictedRDT
`
