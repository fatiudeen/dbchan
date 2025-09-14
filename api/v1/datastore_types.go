/*
Copyright 2025.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DatastoreSpec defines the desired state of Datastore
type DatastoreSpec struct {
	// DatastoreName is the name of the database/datastore
	DatastoreName string `json:"datastoreName"`

	// SecretRef references a secret containing database credentials
	SecretRef corev1.SecretKeySelector `json:"secretRef"`
}

// DatastoreStatus defines the observed state of Datastore
type DatastoreStatus struct {
	// Phase represents the current phase of the datastore
	Phase string `json:"phase,omitempty"`

	// Ready indicates whether the datastore is ready for use
	Ready bool `json:"ready"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Datastore is the Schema for the datastores API
type Datastore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatastoreSpec   `json:"spec,omitempty"`
	Status DatastoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatastoreList contains a list of Datastore
type DatastoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Datastore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Datastore{}, &DatastoreList{})
}
