// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
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
	"fmt"
	"strings"
	"testing"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

type evaluable struct {
	name      string
	namespace string
	qosclass  string
	labels    map[string]string
	tags      map[string]string
	parent    Evaluable
}

func newEvaluable(name, ns, qos string, labels, tags map[string]string, p Evaluable) *evaluable {
	return &evaluable{
		name:      name,
		namespace: ns,
		qosclass:  qos,
		labels:    labels,
		tags:      tags,
		parent:    p,
	}
}

func (e *evaluable) Eval(key string) interface{} {
	switch key {
	case KeyName:
		return e.name
	case KeyNamespace:
		return e.namespace
	case KeyQOSClass:
		return e.qosclass
	case KeyLabels:
		return e.labels
	case KeyTags:
		return e.tags
	case KeyPod:
		if e.parent != nil {
			return e.parent
		}
		fallthrough
	default:
		return fmt.Errorf("evaluable: cannot evaluate %q", key)
	}
}

func (e *evaluable) String() string {
	s := fmt.Sprintf("{ name: %q, namespace: %q, qosclass: %q, ", e.name, e.namespace, e.qosclass)
	labels, t := "{", ""
	for k, v := range e.labels {
		labels += t + fmt.Sprintf("%q:%q", k, v)
		t = ", "
	}
	labels += "}"
	tags, t := "{", ""
	for k, v := range e.tags {
		tags += t + fmt.Sprintf("%q:%q", k, v)
		t = ", "
	}
	tags += "}"
	s = fmt.Sprintf("%s, labels: %s, tags: %s }", s, labels, tags)
	return s
}

func TestResolveRefAndKeyValue(t *testing.T) {
	defer logger.Flush()

	pod := newEvaluable("P1", "pns", "pqos",
		map[string]string{"l1": "plone", "l2": "pltwo", "l5": "plfive"}, nil, nil)

	tcases := []struct {
		name      string
		subject   Evaluable
		keys      []string
		values    []string
		ok        []bool
		error     []bool
		keyvalues []string
	}{
		{
			name: "test resolving references",
			subject: newEvaluable("C1", "cns", "cqos",
				map[string]string{"l1": "clone", "l2": "cltwo", "l3": "clthree"},
				map[string]string{"t1": "ctone", "t2": "cttwo", "t3": "ctthree"}, pod),
			keys: []string{
				"name", "namespace", "qosclass",
				"labels/l1", "labels/l2", "labels/l3", "labels/l4",
				"tags/t1", "tags/t2", "tags/t3", "tags/t4",
				"pod/labels/l1",
				"pod/labels/l2",
				"pod/labels/l3",
				"pod/labels/l4",
				"pod/labels/l5",
				":,-pod/qosclass,pod/namespace,pod/name,name",
			},
			values: []string{
				"C1", "cns", "cqos",
				"clone", "cltwo", "clthree", "",
				"ctone", "cttwo", "ctthree", "",
				"plone", "pltwo", "", "", "plfive",
				"",
			},
			keyvalues: []string{
				"C1", "cns", "cqos",
				"clone", "cltwo", "clthree", "",
				"ctone", "cttwo", "ctthree", "",
				"plone", "pltwo", "", "", "plfive",
				"pqos-pns-P1-C1",
			},
			ok: []bool{
				true, true, true,
				true, true, true, false,
				true, true, true, false,
				true, true, false, false, true,
				false,
			},
			error: []bool{
				false, false, false,
				false, false, false, false,
				false, false, false, false,
				false, false, false, false, false,
				true,
			},
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.keys {
				value, ok, err := ResolveRef(tc.subject, tc.keys[i])
				if err != nil && !tc.error[i] {
					t.Errorf("ResolveRef %s/%q should have given %q, but failed: %v",
						tc.subject, tc.keys[i], tc.values[i], err)
					continue
				}
				if value != tc.values[i] || ok != tc.ok[i] {
					t.Errorf("ResolveRef %s@%q: expected %v, %v got %v, %v",
						tc.subject, tc.keys[i], tc.values[i], tc.ok[i], value, ok)
					continue
				}
				expr := &Expression{
					Key:    tc.keys[i],
					Op:     Equals,
					Values: []string{},
				}
				value, _ = expr.KeyValue(tc.subject)
				if value != tc.keyvalues[i] {
					t.Errorf("KeyValue %s@%q: expected %v, got %v",
						tc.subject, tc.keys[i], tc.keyvalues[i], value)
				}
			}
		})
	}
}

func TestSimpleOperators(t *testing.T) {
	defer logger.Flush()

	pod := newEvaluable("P1", "pns", "pqos",
		map[string]string{"l1": "plone", "l2": "pltwo", "l5": "plfive"},
		nil,
		nil)
	sub := newEvaluable("C1", "cns", "cqos",
		map[string]string{"l1": "clone", "l2": "cltwo", "l3": "clthree"},
		map[string]string{"t1": "ctone", "t2": "cttwo", "t4": "ctfour"},
		pod)

	tcases := []struct {
		name    string
		subject Evaluable
		keys    []string
		ops     []Operator
		values  [][][]string
		results [][]bool
	}{
		{
			name:    "test Equals, NotEqual, In, NotIn operators",
			subject: sub,
			keys: []string{
				"name",
				"pod/name",
				"namespace",
				"pod/namespace",
				"qosclass",
				"pod/qosclass",
				"labels/l1",
				"labels/l2",
				"labels/l3",
				"labels/l4",
				"tags/t1",
				"tags/t2",
				"tags/t3",
				"tags/t4",
				"pod/labels/l1",
				"pod/labels/l2",
				"pod/labels/l3",
				"pod/labels/l4",
				"pod/labels/l5",
			},
			ops: []Operator{Equals, NotEqual, In, NotIn},
			values: [][][]string{
				{{"C1"}, {"C1"}, {"foo", "C1"}, {"foo"}},                    // name
				{{"P1"}, {"P1"}, {"foo", "P1"}, {"foo"}},                    // pod/name
				{{"cns"}, {"cns"}, {"foo", "cns"}, {"foo"}},                 // namespace
				{{"pns"}, {"pns"}, {"foo", "pns"}, {"pns"}},                 // pod/namespace
				{{"cqos"}, {"cqos"}, {"foo", "cqos"}, {"foo"}},              // qosclass
				{{"pqos"}, {"pqos"}, {"foo", "pqos"}, {"pqos"}},             // pod/qosclass
				{{"clone"}, {"clone"}, {"foo", "clone"}, {"foo"}},           // labels/l1
				{{"cltwo"}, {"cltwo"}, {"foo", "cltwo"}, {"foo"}},           // labels/l2
				{{"clthree"}, {"clthree"}, {"foo", "clthree"}, {"clthree"}}, // labels/l3
				{{"clfour"}, {"clfour"}, {"foo", "clfour"}, {"foo"}},        // labels/l4
				{{"ctone"}, {"ctone"}, {"foo", "ctone"}, {"foo"}},           // tags/t1
				{{"cttwo"}, {"cttwo"}, {"foo", "cttwo"}, {"foo"}},           // tags/t2
				{{"ctthree"}, {"ctthree"}, {"foo", "ctthree"}, {"foo"}},     // tags/t3
				{{"ctfour"}, {"ctfour"}, {"foo", "ctfour"}, {"ctfour"}},     // tags/t4
				{{"plone"}, {"plone"}, {"foo", "plone"}, {"foo"}},           // pod/labels/l1
				{{"pltwo"}, {"pltwo"}, {"foo", "pltwo"}, {"foo"}},           // pod/labels/l2
				{{"plthree"}, {"plthree"}, {"foo", "plthree"}, {"foo"}},     // pod/labels/l3
				{{"plfour"}, {"plfour"}, {"foo", "plfour"}, {"foo"}},        // pod/labels/l4
				{{"plfive"}, {"plfive"}, {"foo", "plfive"}, {"foo"}},        // pod/labels/l5
			},
			results: [][]bool{
				{true, false, true, true},  // name
				{true, false, true, true},  // pod/name
				{true, false, true, true},  // namespace
				{true, false, true, false}, // pod/namespace
				{true, false, true, true},  // qosclass
				{true, false, true, false}, // pod/qosclass
				{true, false, true, true},  // labels/l1
				{true, false, true, true},  // labels/l2
				{true, false, true, false}, // labels/l3
				{false, true, false, true}, // labels/l4
				{true, false, true, true},  // tags/t1
				{true, false, true, true},  // tags/t2
				{false, true, false, true}, // tags/t3
				{true, false, true, false}, // tags/t4
				{true, false, true, true},  // pod/labels/l1
				{true, false, true, true},  // pod/labels/l2
				{false, true, false, true}, // pod/labels/l3
				{false, true, false, true}, // pod/labels/l4
				{true, false, true, true},  // pod/labels/l5
			},
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			for k := range tc.keys {
				for o := range tc.ops {
					expr := &Expression{
						Key:    tc.keys[k],
						Op:     tc.ops[o],
						Values: tc.values[k][o],
					}
					expect := tc.results[k][o]
					result := expr.Evaluate(tc.subject)
					if result != expect {
						t.Errorf("%s for %s: expected %v, got %v", expr, tc.subject, expect, result)
					}
				}
			}
		})
	}
}

func TestMatching(t *testing.T) {
	defer logger.Flush()

	p1 := newEvaluable("P1", "pns1", "pqos1",
		map[string]string{"l1": "plv1", "l2": "plv2", "l5": "plv5"},
		nil,
		nil)
	c11 := newEvaluable("C11", "cns1", "cqos11",
		map[string]string{"l1": "clv1", "l2": "clv2", "l3": "clv3"},
		map[string]string{"t1": "ctv1", "t2": "tag2", "t4": "ctv4"},
		p1)
	c12 := newEvaluable("C12", "cns1", "cqos12",
		map[string]string{"l1": "clv1", "l2": "clv2", "l3": "clv3"},
		map[string]string{"t1": "ctv1", "t2": "foo", "t4": "ctv4"},
		p1)
	c13 := newEvaluable("C12", "cns1", "cqos13",
		map[string]string{"l1": "clv1", "l2": "clv2", "l3": "clv3"},
		map[string]string{"t1": "ctv1", "t2": "ctv2", "t4": "ctv4"},
		p1)

	p2 := newEvaluable("P2", "pns2", "pqos2",
		map[string]string{"l1": "plv1", "l2": "plv2", "l5": "plv5"},
		nil,
		nil)
	c21 := newEvaluable("C21", "cns1", "cqos21",
		map[string]string{"l1": "clv1", "l2": "clv2", "l3": "clv3"},
		map[string]string{"t1": "ctv1", "t2": "tag2", "t4": "ctv4"},
		p2)
	c22 := newEvaluable("C22", "cns1", "cqos22",
		map[string]string{"l1": "clv1", "l2": "clv2", "l3": "clv3"},
		map[string]string{"t1": "ctv1", "t2": "ctv2", "t4": "ctv4"},
		p2)
	c23 := newEvaluable("C23", "cns1", "cqos23",
		map[string]string{"l1": "clv1", "l2": "clv2", "l3": "clv3"},
		map[string]string{"t1": "ctv1", "t2": "foo", "t4": "ctv4"},
		p2)

	p3 := newEvaluable("P3", "pns3", "pqos3",
		map[string]string{"l1": "plv1", "l2": "plv2", "l5": "plv5"},
		nil,
		nil)
	c3 := newEvaluable("C3", "cns3", "cqos3",
		map[string]string{"l1": "clv1", "l2": "clv2", "l3": "clv3"},
		map[string]string{"t1": "ctv1", "t2": "tag2", "t4": "ctv4"},
		p3)

	tcases := []struct {
		name      string
		subjects  []Evaluable
		selectors []*Expression
		expected  [][]string
	}{
		{
			name:     "test inverted membership operator",
			subjects: []Evaluable{c11, c12, c13, c21, c22, c23, c3},
			selectors: []*Expression{
				{
					Key: ":,:pod/qosclass,pod/namespace,pod/name,qosclass,name",
					Op:  Matches,
					Values: []string{
						"pqos2:*:*:*:*",
					},
				},
				{
					Key:    "tags/t2",
					Op:     Matches,
					Values: []string{"[tf][ao][go]*"},
				},
			},
			expected: [][]string{
				{"C21", "C22", "C23"},
				{"C11", "C12", "C21", "C23", "C3"},
			},
		},
	}

	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			for i, expr := range tc.selectors {
				results := []string{}
				for _, s := range tc.subjects {
					if expr.Evaluate(s) {
						results = append(results, s.Eval("name").(string))
					}
				}
				expected := strings.Join(tc.expected[i], ",")
				got := strings.Join(results, ",")
				if expected != got {
					t.Errorf("%s: expected %s, got %s", expr, expected, got)
				}
			}
		})
	}
}
