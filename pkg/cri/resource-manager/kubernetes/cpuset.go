// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package kubernetes

import (
	"strconv"
	"strings"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

// ShortCPUSet prints the cpuset as a string, trying to further shorten compared to .String().
func ShortCPUSet(cset cpuset.CPUSet) string {
	str, sep := "", ""

	beg, end, step := -1, -1, -1
	for _, cpu := range strings.Split(cset.String(), ",") {
		if strings.Contains(cpu, "-") {
			str += sep + cpu
			sep = ","
			continue
		}
		i, err := strconv.ParseInt(cpu, 10, 0)
		if err != nil {
			return cset.String()
		}
		id := int(i)
		if beg < 0 {
			beg, end = id, id
			continue
		}
		if step < 0 {
			end = id
			step = end - beg
			continue
		}
		if id-end == step {
			end = id
			continue
		}
		str += sep + mkRange(beg, end, step)
		sep = ","
		beg, end = id, id
		step = -1
	}

	if beg >= 0 {
		str += sep + mkRange(beg, end, step)
	}

	return str
}

func mkRange(beg, end, step int) string {
	if beg < 0 {
		return ""
	}
	if beg == end {
		return strconv.FormatInt(int64(beg), 10)
	}

	b, e := strconv.FormatInt(int64(beg), 10), strconv.FormatInt(int64(end), 10)
	if step == 1 {
		return b + "-" + e
	}
	if beg+step == end {
		return b + "," + e
	}

	s := strconv.FormatInt(int64(step), 10)
	return b + "-" + e + ":" + s
}
