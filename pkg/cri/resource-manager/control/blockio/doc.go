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

package blockio

var configHelp = `
Resource Manager Block I/O enforcement controller.

The Block I/O controller enforces container policy block I/O decisions
using the blkio cgroup pseudo-filesystem. It takes the assigned block I/O
class of a container and puts all processes in the containers' cgroup to
that class. If the container is not assigned to any block I/O class by a
policy, the block I/O controller will use the QOS class of the container.

The controller can be configured to map the containers' assigned class to
a real block I/O class before the blkio-level assignment takes place.

Here is a sample configuration fragment to set up a class mapping for the
3 Kubernetes QoS classes and define a default class.

  blockio:
    Classes:
      Guaranteed: class1
      Burstable: class2
      BestEffort: class3
      "*": class4
`
