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

package resmgr

import (
	// List of builtin policies
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/balloons"
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/none"
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/podpools"
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/static"
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/static-plus"
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/static-pools"
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/topology-aware"
	_ "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/policy/builtin/dynamic-pools"
)

// TODO: add unit tests to verify that all builtin policies are found
