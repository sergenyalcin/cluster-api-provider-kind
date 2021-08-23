/*
Copyright 2021.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var KindOfKindCluster = "KINDCluster"

type KindClusterCondition struct {
	Timestamp metav1.Time `json:"timestamp,omitempty"`
	Message   string      `json:"message,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KINDClusterSpec defines the desired state of KINDCluster
type KINDClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//+kubebuilder:validation:MaxLength=64
	ClusterName string `json:"clusterName"`
}

// KINDClusterStatus defines the observed state of KINDCluster
type KINDClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Ready bool `json:"ready,omitempty"`

	Conditions []KindClusterCondition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`,description="Status of the resource"
//+kubebuilder:printcolumn:name="ClusterName",type=string,JSONPath=`.spec.clusterName`,description="ClusterName of the resource"
//+kubebuilder:resource:path=kindclusters,shortName=kc

// KINDCluster is the Schema for the kindclusters API
type KINDCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KINDClusterSpec   `json:"spec,omitempty"`
	Status KINDClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KINDClusterList contains a list of KINDCluster
type KINDClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KINDCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KINDCluster{}, &KINDClusterList{})
}
