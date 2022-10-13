// Copyright 2022 Intel Corporation. All Rights Reserved.
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

package config_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/intel/cri-resource-manager/pkg/config"
	"github.com/stretchr/testify/require"
)

type dummyCfg struct{}

func (*dummyCfg) Reset()           {}
func (*dummyCfg) Describe() string { return "" }

func TestInvalidRegistration(t *testing.T) {
	config.ReInitialize()

	var (
		i = 3
		s = dummyCfg{}
	)

	type testCase struct {
		name string
		path string
		ptr  interface{}
	}

	for _, tc := range []testCase{
		{name: "nil", path: "nil", ptr: nil},
		{name: "non-pointer", path: "nonPtr", ptr: i},
		{name: "pointer to non-struct", path: "ptrToNonStruct", ptr: &i},
		{name: "empty path", path: "", ptr: &s},
		{name: "invalid path", path: "test..path", ptr: &s},
		{name: "non-fragment ptr", path: "nonFragmentPtr", ptr: &struct{}{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := config.Register(tc.path, tc.ptr)
			require.Error(t, err, tc.name)
		})
	}
}

func TestConflictingRegistration(t *testing.T) {
	config.ReInitialize()

	type testCase struct {
		name  string
		path  string
		valid bool
	}

	for _, tc := range []testCase{
		{name: "register fragment #1", path: "main.group.module1", valid: true},
		{name: "conflicting path #1", path: "Main.group.module1"},
		{name: "conflicting path #2", path: "main.Group.module1"},
		{name: "conflicting path #3", path: "main.group.Module1"},
		{name: "conflicting path #4", path: "main.Group.Module1"},
		{name: "conflicting path #5", path: "Main.group.Module1"},
		{name: "conflicting path #6", path: "Main.Group.module1"},
		{name: "conflicting path #7", path: "Main.Group.Module1"},
		{name: "register fragment #2", path: "main.group.module1.sub-module", valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := config.Register(tc.path, &dummyCfg{})
			if tc.valid {
				require.NoError(t, err, tc.name)
			} else {
				require.Error(t, err, tc.name)
			}
		})
	}
}

type testData struct {
	Int    int `json:"integer"`
	String string
	Float  float64
	Map    map[string]string
	Slice  []string
	Bool   bool
}

type testMod1 testData

func (*testMod1) Describe() string { return "" }

func (d *testMod1) Reset() {
	*d = testMod1{}
}

func (d *testMod1) Validate() error {
	if d.Int < 0 {
		return fmt.Errorf("invalid (negative) integer %d", d.Int)
	}
	if strings.Contains(d.String, "invalid") {
		return fmt.Errorf("invalid string %q", d.String)
	}
	if t := float64(d.Int) + d.Float; t == 6.66 || t == 66.6 || t == 666.0 {
		return fmt.Errorf("invalid total of %d + %f", d.Int, d.Float)
	}
	return nil
}

type testMod2 testData

func (d *testMod2) Reset() {
	*d = testMod2{
		String: d.String,
		Map:    d.Map,
	}
	if d.String == "" {
		d.String = "default mod2 string"
	}
	if d.Map == nil {
		d.Map = map[string]string{
			"name1": "defaultValue1",
		}
	}

}

func (*testMod2) Describe() string { return "" }

type testMod3 testData

func (d *testMod3) Reset() {
	*d = testMod3{
		String: "default mod3 string",
		Map: map[string]string{
			"mod3name1": "mod3DefaultValue1",
		},
	}
}

func (*testMod3) Describe() string { return "" }

func TestConfigSetting0(t *testing.T) {
	type testCase struct {
		name    string
		data    string
		expect1 *testMod1
		expect2 *testMod2
		expect3 *testMod3
	}

	mod1 := &testMod1{}
	mod2 := &testMod2{}
	mod3 := &testMod3{}

	require.NoError(t, config.Register("main.group.mod1", mod1))
	require.NoError(t, config.Register("main.group.mod2", mod2))
	require.NoError(t, config.Register("main.group.mod2.mod3", mod3))

	for _, tc := range []testCase{
		{
			name: "set integer",
			data: `
main:
  group:
    mod1:
      integer: 3
`,
			expect1: &testMod1{
				Int: 3,
			},
		},
		{
			name: "set string",
			data: `
main:
  group:
    mod1:
      String: test string
`,
			expect1: &testMod1{
				String: "test string",
			},
		},
		{
			name: "set float",
			data: `
main:
  group:
    mod1:
      Float: 1.23
`,
			expect1: &testMod1{
				Float: 1.23,
			},
		},
		{
			name: "set bool",
			data: `
main:
  group:
    mod1:
      Bool: true
`,
			expect1: &testMod1{
				Bool: true,
			},
		},
		{
			name: "set map",
			data: `
main:
  group:
    mod1:
      Map:
        name1: value1
        name2: value2
        name3: value3
`,
			expect1: &testMod1{
				Map: map[string]string{
					"name1": "value1",
					"name2": "value2",
					"name3": "value3",
				},
			},
		},
		{
			name: "set slice",
			data: `
main:
  group:
    mod1:
      Slice:
        - var1=val1
        - var2=val2
        - var3=val3
`,
			expect1: &testMod1{
				Slice: []string{
					"var1=val1",
					"var2=val2",
					"var3=val3",
				},
			},
		},
		{
			name: "set all",
			data: `
main:
  group:
    mod1:
      integer: 123
      Float: 9.81
      String: foobar
      Bool: true
      Map:
        foo: bar
        foobar: xyzzy
      Slice:
        - s0
        - s1
        - s2
`,
			expect1: &testMod1{
				Int:    123,
				Float:  9.81,
				String: "foobar",
				Bool:   true,
				Map: map[string]string{
					"foo":    "bar",
					"foobar": "xyzzy",
				},
				Slice: []string{
					"s0",
					"s1",
					"s2",
				},
			},
		},
		{
			name: "non-empty default (string) kept intact",
			data: `
main:
  group:
    mod2:
      Map:
        name1: value1
        name2: value2
        name3: value3
`,
			expect2: &testMod2{
				String: "default mod2 string",
				Map: map[string]string{
					"name1": "value1",
					"name2": "value2",
					"name3": "value3",
				},
			},
		},
		{
			name: "non-empty default (string) overridden",
			data: `
main:
  group:
    mod2:
      Float: 3.14
      String: string new value
`,
			expect2: &testMod2{
				Float:  3.14,
				String: "string new value",
				Map: map[string]string{
					"name1": "value1",
					"name2": "value2",
					"name3": "value3",
				},
			},
		},
		{
			name: "non-empty default map kept intact",
			data: `
main:
  group:
    mod2:
      integer: 1
`,
			expect2: &testMod2{
				Int:    1,
				String: "string new value",
				Map: map[string]string{
					"name1": "value1",
					"name2": "value2",
					"name3": "value3",
				},
			},
		},
		{
			name: "set fragment registered sub-module of another fragment",
			data: `
main:
  group:
    mod2:
      integer: 1
      Float: 5.678
      mod3:
        integer: 2
        Float: 9.87
        Bool: true
        Slice:
          - one
          - two
          - three
`,
			expect2: &testMod2{
				Int:    1,
				Float:  5.678,
				String: "string new value",
				Map: map[string]string{
					"name1": "value1",
					"name2": "value2",
					"name3": "value3",
				},
			},
			expect3: &testMod3{
				Int:    2,
				String: "default mod3 string",
				Float:  9.87,
				Bool:   true,
				Map: map[string]string{
					"mod3name1": "mod3DefaultValue1",
				},
				Slice: []string{
					"one",
					"two",
					"three",
				},
			},
		},
	} {
		require.NoError(t, config.SetYAML([]byte(tc.data)), tc.name)
		if tc.expect1 != nil {
			require.Equal(t, tc.expect1, mod1, tc.name+"/mod1")
		}
		if tc.expect2 != nil {
			require.Equal(t, tc.expect2, mod2, tc.name+"/mod2")
		}
		if tc.expect3 != nil {
			require.Equal(t, tc.expect3, mod3, tc.name+"/mod3")
		}
	}
}

func TestFragmentValidation(t *testing.T) {
	config.ReInitialize()

	type testCase struct {
		name  string
		data  string
		valid bool
	}

	var (
		mod1 = &testMod1{}
	)

	require.NoError(t, config.Register("config.group.mod1", mod1), "register config.group.mod1")

	for _, tc := range []testCase{
		{
			name: "invalid integer",
			data: `
config:
  group:
    mod1:
      integer: -1
`,
		},
		{
			name: "invalid string",
			data: `
config:
  group:
    mod1:
      String: an invalid string
`,
		},
		{
			name: "invalid sum #1",
			data: `
config:
  group:
    mod1:
      integer: 5
      Float: 1.66
`,
		},
		{
			name: "invalid sum #2",
			data: `
config:
  group:
    mod1:
      integer: 11
      Float: 55.6
`,
		},
		{
			name: "invalid sum #3",
			data: `
config:
  group:
    mod1:
      integer: 111
      Float: 555.0
`,
		},
		{
			name: "valid integer",
			data: `
config:
  group:
    mod1:
      integer: 1
`,
			valid: true,
		},
		{
			name: "valid string",
			data: `
config:
  group:
    mod1:
      String: a valid string
`,
			valid: true,
		},
		{
			name: "valid sum",
			data: `
config:
  group:
    mod1:
      integer: 222
      Float: 555.5
`,
			valid: true,
		},
	} {
		if tc.valid {
			require.NoError(t, config.SetYAML([]byte(tc.data)), tc.name)
		} else {
			require.Error(t, config.SetYAML([]byte(tc.data)), tc.name)
		}
	}
}

func TestGetConfig(t *testing.T) {
	config.ReInitialize()

	var (
		mod1 = &dummyCfg{}
		mod2 = &dummyCfg{}
		mod3 = &dummyCfg{}
	)

	require.NoError(t, config.Register("config.mod1", mod1), "register mod1")
	require.NoError(t, config.Register("config.mod2", mod2), "register mod2")
	require.NoError(t, config.Register("config.mod3", mod3), "register mod3")

	m1, ok := config.GetConfig("config.mod1")
	require.True(t, ok && m1.(*dummyCfg) == mod1, "check mod1")

	m2, ok := config.GetConfig("config.mod2")
	require.True(t, ok && m2.(*dummyCfg) == mod2, "check mod2")

	m3, ok := config.GetConfig("config.mod3")
	require.True(t, ok && m3.(*dummyCfg) == mod3, "chcek mod3")
}
