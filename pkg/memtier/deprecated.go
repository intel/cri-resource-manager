// Copyright 2023 Intel Corporation. All Rights Reserved.
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

package memtier

import (
	"fmt"
)

func deprecatedPolicyCgroupsConfig(policyName string) error {
	log.Errorf("DEPRECATED policy configuration detected: policy.cgroups")
	log.Errorf("Replace \"cgroups\" with:")
	log.Errorf("pidwatcher:")
	log.Errorf("  name: cgroups")
	log.Errorf("  config: |")
	log.Errorf("    cgroups:")
	log.Errorf("      - /path/to/a/cgroup ...")
	return fmt.Errorf("deprecated \"cgroups\" found in the %s policy configuration, use pidwatcher cgroups instead", policyName)
}

func deprecatedPolicyPidsConfig(policyName string) error {
	log.Errorf("DEPRECATED policy configuration detected: policy.pids")
	log.Errorf("Replace \"pids\" with:")
	log.Errorf("pidwatcher:")
	log.Errorf("  name: pidlist")
	log.Errorf("  config: |")
	log.Errorf("    pids:")
	log.Errorf("      - PID...")
	return fmt.Errorf("deprecated \"pids\" found in the %s policy configuration, use pidwatcher pidlist instead", policyName)
}
