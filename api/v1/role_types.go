/*
Copyright 2024.

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

// RoleSpec defines the desired state of Role
type RoleSpec struct {
	// DatastoreRef references the datastore where this role will be created
	DatastoreRef corev1.LocalObjectReference `json:"datastoreRef"`

	// Privileges defines the database privileges for this role
	Privileges []string `json:"privileges,omitempty"`

	// DatabaseRef references a specific database (optional, for database-specific roles)
	DatabaseRef *corev1.LocalObjectReference `json:"databaseRef,omitempty"`

	// Description provides a human-readable description of the role
	Description string `json:"description,omitempty"`

	// IsGlobal indicates whether this is a global role (affects all databases)
	IsGlobal bool `json:"isGlobal,omitempty"`
}

// RoleStatus defines the observed state of Role
type RoleStatus struct {
	// Phase represents the current phase of the role
	Phase string `json:"phase,omitempty"`

	// Ready indicates whether the role is ready for use
	Ready bool `json:"ready"`

	// Created indicates whether the role has been created in the database
	Created bool `json:"created"`

	// Message provides additional information about the role status
	Message string `json:"message,omitempty"`

	// LastUpdated indicates when the role was last updated
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Created",type="boolean",JSONPath=".status.created"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Role is the Schema for the roles API
type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoleSpec   `json:"spec,omitempty"`
	Status RoleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RoleList contains a list of Role
type RoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Role `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Role{}, &RoleList{})
}
