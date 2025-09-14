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

// UserSpec defines the desired state of User
type UserSpec struct {
	// Username is the name of the database user/role
	Username string `json:"username"`

	// Password is the base64-encoded password for the user
	Password string `json:"password"`

	// DatastoreRef references the datastore where this user should be created
	DatastoreRef corev1.LocalObjectReference `json:"datastoreRef"`

	// DatabaseRef references the specific database (optional, for database-specific users)
	DatabaseRef *corev1.LocalObjectReference `json:"databaseRef,omitempty"`

	// Roles defines the database roles/permissions for this user
	Roles []string `json:"roles,omitempty"`

	// Privileges defines specific database privileges for this user
	Privileges []string `json:"privileges,omitempty"`

	// Host specifies the host from which the user can connect (e.g., "localhost", "%")
	Host string `json:"host,omitempty"`
}

// UserStatus defines the observed state of User
type UserStatus struct {
	// Phase represents the current phase of the user
	Phase string `json:"phase,omitempty"`

	// Ready indicates whether the user is ready for use
	Ready bool `json:"ready"`

	// Created indicates whether the user has been created in the database
	Created bool `json:"created"`

	// CreatedAt indicates when the user was created
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// User is the Schema for the users API
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec,omitempty"`
	Status UserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserList contains a list of User
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}
