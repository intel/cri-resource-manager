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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	resapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	resmgr "github.com/intel/cri-resource-manager/pkg/apis/resmgr"
)

const (
	GroupName string = "criresmgr.intel.com"    // GroupName is the group of our CRD.
	Version   string = "v1alpha1"               // Version is the API version of our CRD.
	Kind      string = "Adjustment"             // Kind is the object kind of our CRD.
	Plural    string = "adjustments"            // Plural is Kind in plural form.
	Singular  string = "adjustment"             // Singular is Kind in singular form.
	Name      string = Plural + "." + GroupName // Name is the full name of our CRD.
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Adjustment is a CRD used to externally adjust containers resource assignments.
type Adjustment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AdjustmentSpec   `json:"spec"`
	Status AdjustmentStatus `json:"status"`
}

// AdjustmentSpec specifies the scope for an external adjustment.
type AdjustmentSpec struct {
	Scope        []AdjustmentScope            `json:"scope"`
	Resources    *corev1.ResourceRequirements `json:"resources"`
	Classes      *Classes                     `json:"classes"`
	ToptierLimit *resapi.Quantity             `json:"toptierLimit"`
}

// AdjustmentStatus represents the status of applying an adjustment.
type AdjustmentStatus struct {
	Nodes map[string]AdjustmentNodeStatus `json:"nodes"`
}

// AdjustmentNodeStatus represents the status of an adjustment on a node.
type AdjustmentNodeStatus struct {
	Errors map[string]string `json:"errors"`
}

// AdjustmentScope defines the scope for an adjustment.
type AdjustmentScope struct {
	Nodes      []string             `json:"nodes"`
	Containers []*resmgr.Expression `json:"containers"`
}

// Classes defines RDT and BlockIO class assignments.
type Classes struct {
	BlockIO *string `json:"blockio"`
	RDT     *string `json:"rdt"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AdjustmentList is a list of Adjustments.
type AdjustmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Adjustment `json:"items"`
}
