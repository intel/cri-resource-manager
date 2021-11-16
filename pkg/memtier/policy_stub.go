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

type PolicyStub struct {
}

func init() {
	PolicyRegister("stub", NewPolicyStub)
}

func NewPolicyStub() (Policy, error) {
	return &PolicyStub{}, nil
}

func (p *PolicyStub) SetConfigJson(configJson string) error {
	return nil
}

func (p *PolicyStub) GetConfigJson() string {
	return ""
}

func (p *PolicyStub) Start() error {
	return nil
}

func (p *PolicyStub) Stop() {
}

func (p *PolicyStub) Mover() *Mover {
	return nil
}

func (p *PolicyStub) Tracker() Tracker {
	return nil
}

func (p *PolicyStub) Dump() string {
	return ""
}
