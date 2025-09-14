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

// MigrationSpec defines the desired state of Migration
type MigrationSpec struct {
	// DatabaseRef references the database where this migration should be applied
	DatabaseRef corev1.LocalObjectReference `json:"databaseRef"`

	// SQL contains the SQL script to be executed
	SQL string `json:"sql"`

	// Version is a unique identifier for this migration (e.g., "001", "20240101_001")
	Version string `json:"version"`

	// Description provides a human-readable description of what this migration does
	Description string `json:"description,omitempty"`

	// RollbackSQL contains the SQL script to rollback this migration (optional)
	RollbackSQL string `json:"rollbackSql,omitempty"`
}

// MigrationStatus defines the observed state of Migration
type MigrationStatus struct {
	// Phase represents the current phase of the migration
	Phase string `json:"phase,omitempty"`

	// Applied indicates whether the migration has been successfully applied
	Applied bool `json:"applied"`

	// AppliedAt indicates when the migration was applied
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// Checksum stores the hash of the SQL content to detect changes
	Checksum string `json:"checksum,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Migration is the Schema for the migrations API
type Migration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MigrationSpec   `json:"spec,omitempty"`
	Status MigrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MigrationList contains a list of Migration
type MigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Migration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Migration{}, &MigrationList{})
}
