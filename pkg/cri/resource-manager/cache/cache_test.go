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

package cache

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestKeysInNamespace(t *testing.T) {
	var m map[string]string
	var keys []string
	var expected []string

	// nil or empty map should return nothing
	keys = keysInNamespace(&m, "")
	if len(keys) != 0 {
		t.Errorf("Exptected empty list, received %v", keys)
	}
	keys = keysInNamespace(&m, "my.name.space")
	if len(keys) != 0 {
		t.Errorf("Exptected empty list, received %v", keys)
	}
	m = map[string]string{}
	keys = keysInNamespace(&m, "")
	if len(keys) != 0 {
		t.Errorf("Exptected empty list, received %v", keys)
	}

	// Fill map with some values
	m["no-namespace"] = ""
	m["my.name.space"] = ""
	m["my.name.space/key-1"] = ""
	m["my.name.space/key-2"] = ""
	m["other.name.space/other-key"] = ""

	// Keys with no namespace
	keys = keysInNamespace(&m, "")
	sort.Strings(keys)
	expected = []string{"my.name.space", "no-namespace"}
	if !cmp.Equal(keys, expected) {
		t.Errorf("Exptected %v, received %v", expected, keys)
	}

	// Keys in namespace
	keys = keysInNamespace(&m, "my.name.space")
	sort.Strings(keys)
	expected = []string{"key-1", "key-2"}
	if !cmp.Equal(keys, expected) {
		t.Errorf("Exptected %v, received %v", expected, keys)
	}
}
