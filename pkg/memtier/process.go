// Copyright 2021 Intel Corporation. All Rights Reserved.
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

type Process struct {
	pid int
}

func NewProcess(pid int) *Process {
	return &Process{pid}
}

// AddressRanges returns address ranges of the process
func (p *Process) AddressRanges() (*AddrRanges, error) {
	ar, err := procMaps(p.pid)
	if err != nil {
		return nil, err
	}
	return &AddrRanges{pid: p.pid, addrs: ar}, nil
}
