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

package v1

import (
	"encoding/json"
)

var _ json.Marshaler = &JsonPatch{}

func (j *JsonPatch) MarshalJSON() ([]byte, error) {
	// Don't really encode anything. Op and Path are ascii strings and value
	// is assumed to be in marshaled format
	if len(j.Value) == 0 {
		return []byte(`{"op":"` + j.Op + `","path":"` + j.Path + `"}`), nil
	}
	return []byte(`{"op":"` + j.Op + `","path":"` + j.Path + `","value":` + j.Value + `}`), nil
}
