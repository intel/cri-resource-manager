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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/yaml"
)

type jsonPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

type podResourceRequirements struct {
	InitContainers map[string]corev1.ResourceRequirements `json:"initContainers"`
	Containers     map[string]corev1.ResourceRequirements `json:"containers"`
}

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

// Module inatialization
func init() {
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(admissionv1.AddToScheme(scheme))
}

// Helper for creating an AdmissionResponse with an error
func errResponse(err error) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

// Dump req/rsp in human-readable form
func stringify(r interface{}) string {
	out, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Sprintf("!!!!!\nUnable to stringify %T: %v\n!!!!!", r, err)
	}
	return string(out)
}

// Handle HTTP requests
func handle(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// Check Content-Type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Printf("ERROR: incorrect Content-Type (received %s, expect application/json", contentType)
		return
	}

	// Deserialize AdmissionReview request and create an AdmissionReview response
	arReq := admissionv1.AdmissionReview{}
	arRsp := admissionv1.AdmissionReview{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &arReq); err != nil {
		log.Printf("ERROR: deserializing admission request failed: %v", err)
		arRsp.Response = errResponse(err)
	} else if arReq.Request == nil {
		log.Printf("REQUEST empty")
		arRsp.Response = errResponse(errors.New("Empty request"))
	} else {
		log.Printf("REQUEST:\n%s", stringify(&arReq))
		if arReq.Request.Resource.Group != "" || arReq.Request.Resource.Version != "v1" {
			arRsp.Response = errResponse(fmt.Errorf("Unexpected resource group/version '%s/%s'", arReq.Request.Resource.Group, arReq.Request.Resource.Version))
		} else {
			res := arReq.Request.Resource.Resource
			switch res {
			case "pods":
				arRsp.Kind = "AdmissionReview"
				arRsp.APIVersion = "admission.k8s.io/v1"
				arRsp.Response = mutatePodObject(&arReq.Request.Object)
			default:
				arRsp.Response = errResponse(fmt.Errorf("Unexpected resource %s", arReq.Request.Resource))
			}
		}
		// Use the same UID in response that was used in the request
		arRsp.Response.UID = arReq.Request.UID
	}

	log.Printf("RESPONSE:\n%s", stringify(arRsp.Response))

	respBytes, err := json.Marshal(arRsp)
	if err != nil {
		log.Printf("ERROR: json marshal failed: %v", err)
	}
	if _, err := w.Write(respBytes); err != nil {
		log.Printf("ERROR: failed to write HTTP response: %v", err)
	}
}

// Handle AdmissionReview requests for Pod objects
func mutatePodObject(rawObj *runtime.RawExtension) *admissionv1.AdmissionResponse {
	pod := corev1.Pod{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(rawObj.Raw, nil, &pod); err != nil {
		log.Printf("ERROR: failed to deserialize Pod object: %v", err)
		return errResponse(err)
	}

	reviewResponse := admissionv1.AdmissionResponse{}
	reviewResponse.Allowed = true

	patches := []jsonPatch{}
	// Add a patch to add an empty annotations object if no annotations are found
	if pod.ObjectMeta.Annotations == nil {
		patches = append(patches, jsonPatch{Op: "add", Path: "/metadata/annotations", Value: map[string]string{}})
	}

	patch, err := patchResourceAnnotation(&pod)
	if err != nil {
		return errResponse(err)
	}
	patches = append(patches, patch)

	reviewResponse.Patch, err = json.Marshal(patches)
	if err != nil {
		log.Printf("ERROR: failed to marshal Pod patch: %v", err)
		return errResponse(err)
	}
	patchType := admissionv1.PatchTypeJSONPatch
	reviewResponse.PatchType = &patchType

	return &reviewResponse
}

// Create a Pod (JSON) patch adding resource annotation
func patchResourceAnnotation(pod *corev1.Pod) (jsonPatch, error) {
	patch := jsonPatch{Op: "add", Path: "/metadata/annotations/intel.com~1resources"}

	// Create annotation that includes all resources of all (init)containers
	resourceAnnotation := podResourceRequirements{InitContainers: map[string]corev1.ResourceRequirements{},
		Containers: map[string]corev1.ResourceRequirements{}}
	for _, container := range pod.Spec.Containers {
		resourceAnnotation.Containers[container.Name] = container.Resources
	}
	for _, container := range pod.Spec.InitContainers {
		resourceAnnotation.InitContainers[container.Name] = container.Resources
	}
	resourceAnnotationBytes, err := json.Marshal(resourceAnnotation)
	if err != nil {
		log.Printf("ERROR: failed to marshal 'intel.com/resources' annotations: %v", err)
		return patch, err
	}

	// Patch Pod annotations to include the "resources" annotation
	patch.Value = string(resourceAnnotationBytes)

	return patch, nil
}
