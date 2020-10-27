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

package stp

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

func TestParseContainerCmdline(t *testing.T) {
	stp := &stp{Logger: logger.NewLogger(PolicyName + "-test")}

	// 1. empty command line should return a nil pointer
	args := stp.parseContainerCmdline([]string{}, []string{})
	if args != nil {
		t.Errorf("Exptected <nil> but got %v", *args)
	}

	// 2. case where cmk isolate command is in container "Command"
	args = stp.parseContainerCmdline([]string{"cmk", "isolate", "--pool", "foo", "--socket-id=2", "--conf-dir=/etc", "cmd", "-arg"}, []string{})
	expected := cmkLegacyArgs{Pool: "foo", SocketID: 2, Command: []string{"cmd", "-arg"}}
	if args == nil || !cmp.Equal(expected, *args) {
		t.Errorf("Exptected %v but got %v", expected, *args)
	}

	// 3. we should ignore unknown cmk options
	args = stp.parseContainerCmdline([]string{"cmk", "isolate", "--invalid-1=inv1", "--pool", "foo", "--invalid-2=inv2", "cmd", "--arg"}, []string{})
	expected = cmkLegacyArgs{Pool: "foo", SocketID: -1, Command: []string{"cmd", "--arg"}}
	if args == nil || !cmp.Equal(expected, *args) {
		t.Errorf("Exptected %v but got %v", expected, *args)
	}

	// 4. --pool should be defined in cmk options
	args = stp.parseContainerCmdline([]string{"cmk", "isolate", "--socket-id=2", "cmd", "--arg"}, []string{})
	if args != nil {
		t.Errorf("Exptected <nil> but got %v", *args)
	}

	// 5. parsing from container "Args"
	args = stp.parseContainerCmdline([]string{"bash"}, []string{"-c", "cmk isolate --pool=foo --socket-id=2 cmd --arg"})
	expected = cmkLegacyArgs{Pool: "foo", SocketID: 2, Command: []string{"cmd", "--arg"}}
	if args == nil || !cmp.Equal(expected, *args) {
		t.Errorf("Exptected %v but got %v", expected, *args)
	}

	// 6. Only _cmk_ isolate should be accepted
	args = stp.parseContainerCmdline([]string{"bash"}, []string{"-c", "dmk isolate --pool=foo cmd --arg"})
	if args != nil {
		t.Errorf("Exptected <nil> but got %v", *args)
	}
}

func TestCachableData(t *testing.T) {
	ccr := &stpContainerCache{"id1": stpContainerStatus{Pool: "p", Socket: 1}}

	// Test JSON marshalling of cached data
	data, err := json.Marshal(ccr)
	if err != nil {
		t.Errorf("JSON marshal failed: %v", err)
	}
	expected := []byte(`{"id1":{"Pool":"p","Socket":1,"NExclusiveCPUs":0,"Cpusets":null,"NoAffinity":false}}`)
	if !cmp.Equal(expected, data) {
		t.Errorf("Exptected %s but got %s", expected, data)
	}

	// Test JSON unmarshalling of cached data
	ccr2 := &stpContainerCache{}
	err = json.Unmarshal(data, ccr2)
	if err != nil {
		t.Errorf("JSON unmarshal failed: %v", err)
	}
	if !cmp.Equal(*ccr, *ccr2) {
		t.Errorf("Exptected %v but got %v", *ccr, *ccr2)
	}
}
