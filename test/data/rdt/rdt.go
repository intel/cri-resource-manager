/*
Copyright 2020 Intel Corporation

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

package rdtdata

import (
	"path/filepath"
	"runtime"
)

var thisDir []string

func init() {
	_, this, _, _ := runtime.Caller(0)
	thisDir = []string{filepath.Dir(this)}
}

// Path returns an absolute path to test data
func Path(elem ...string) string {
	return filepath.Join(append(thisDir, elem...)...)
}
