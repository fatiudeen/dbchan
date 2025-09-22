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

package webhook

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
)

// log is for logging in this package.
var rolelog = logf.Log.WithName("role-resource")

// SetupRoleWebhookWithManager sets up the Role webhook with the Manager.
func SetupRoleWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&dbv1.DatabaseRole{}).
		WithValidator(&RoleWebhook{Client: mgr.GetClient()}).
		Complete()
}

//+kubebuilder:webhook:path=/validate-db-fatiudeen-dev-v1-role,mutating=false,failurePolicy=fail,sideEffects=None,groups=db.fatiudeen.dev,resources=roles,verbs=create;update,versions=v1,name=vrole.kb.io,admissionReviewVersions=v1

// RoleWebhook validates Role resources
type RoleWebhook struct {
	Client client.Client
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RoleWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	role, ok := obj.(*dbv1.DatabaseRole)
	if !ok {
		return nil, fmt.Errorf("expected a Role but got a %T", obj)
	}

	rolelog.Info("validating create", "name", role.Name)

	var allErrs field.ErrorList

	// Validate required fields
	if role.Spec.DatastoreRef.Name == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "datastoreRef", "name"), "datastoreRef.name is required"))
	}

	// Validate role name
	if err := r.validateRoleName(role.Name); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("metadata", "name"), role.Name, err.Error()))
	}

	// Validate privileges
	if err := r.validatePrivileges(role.Spec.Privileges); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "privileges"), role.Spec.Privileges, err.Error()))
	}

	// Validate datastore exists
	if err := r.validateDatastoreExists(ctx, role); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "datastoreRef", "name"), role.Spec.DatastoreRef.Name, err.Error()))
	}

	// Validate database exists if specified
	if role.Spec.DatabaseRef != nil && role.Spec.DatabaseRef.Name != "" {
		if err := r.validateDatabaseExists(ctx, role); err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "databaseRef", "name"), role.Spec.DatabaseRef.Name, err.Error()))
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, fmt.Errorf("validation failed: %v", allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RoleWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldRole, ok := oldObj.(*dbv1.DatabaseRole)
	if !ok {
		return nil, fmt.Errorf("expected a Role but got a %T", oldObj)
	}

	newRole, ok := newObj.(*dbv1.DatabaseRole)
	if !ok {
		return nil, fmt.Errorf("expected a Role but got a %T", newObj)
	}

	rolelog.Info("validating update", "name", newRole.Name)

	var allErrs field.ErrorList

	// Validate required fields
	if newRole.Spec.DatastoreRef.Name == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "datastoreRef", "name"), "datastoreRef.name is required"))
	}

	// Validate role name
	if err := r.validateRoleName(newRole.Name); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("metadata", "name"), newRole.Name, err.Error()))
	}

	// Validate privileges
	if err := r.validatePrivileges(newRole.Spec.Privileges); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "privileges"), newRole.Spec.Privileges, err.Error()))
	}

	// Validate datastore exists
	if err := r.validateDatastoreExists(ctx, newRole); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "datastoreRef", "name"), newRole.Spec.DatastoreRef.Name, err.Error()))
	}

	// Validate database exists if specified
	if newRole.Spec.DatabaseRef != nil && newRole.Spec.DatabaseRef.Name != "" {
		if err := r.validateDatabaseExists(ctx, newRole); err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "databaseRef", "name"), newRole.Spec.DatabaseRef.Name, err.Error()))
		}
	}

	// Check if datastore reference changed
	if oldRole.Spec.DatastoreRef.Name != newRole.Spec.DatastoreRef.Name {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "datastoreRef", "name"), newRole.Spec.DatastoreRef.Name, "datastoreRef.name cannot be changed"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, fmt.Errorf("validation failed: %v", allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RoleWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	role, ok := obj.(*dbv1.DatabaseRole)
	if !ok {
		return nil, fmt.Errorf("expected a Role but got a %T", obj)
	}

	rolelog.Info("validating delete", "name", role.Name)

	// Check if any users are using this role
	users, err := r.findUsersUsingRole(ctx, role)
	if err != nil {
		return nil, fmt.Errorf("failed to check users using role: %v", err)
	}

	if len(users) > 0 {
		return nil, fmt.Errorf("cannot delete role %s: %d users are still using this role", role.Name, len(users))
	}

	return nil, nil
}

// validateRoleName validates the role name
func (r *RoleWebhook) validateRoleName(name string) error {
	if name == "" {
		return fmt.Errorf("role name cannot be empty")
	}

	// Check length
	if len(name) < 1 || len(name) > 32 {
		return fmt.Errorf("role name must be between 1 and 32 characters")
	}

	// Check for valid characters (alphanumeric, underscore, hyphen)
	validNameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("role name can only contain alphanumeric characters, underscores, and hyphens")
	}

	// Check for reserved names
	reservedNames := []string{
		"root", "admin", "administrator", "system", "mysql", "postgres",
		"information_schema", "performance_schema", "mysql", "sys",
		"public", "guest", "sa", "dbo", "db_owner", "db_accessadmin",
		"db_securityadmin", "db_ddladmin", "db_backupoperator",
		"db_datarader", "db_datawriter", "db_denydatareader", "db_denydatawriter",
	}
	for _, reserved := range reservedNames {
		if strings.EqualFold(name, reserved) {
			return fmt.Errorf("role name '%s' is reserved and cannot be used", name)
		}
	}

	// Check for leading/trailing hyphens
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("role name cannot start or end with a hyphen")
	}

	return nil
}

// validatePrivileges validates the privileges list
func (r *RoleWebhook) validatePrivileges(privileges []string) error {
	if len(privileges) == 0 {
		return nil // Privileges are optional
	}

	// Valid privileges for different database types
	validPrivileges := map[string]bool{
		// Common privileges
		"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
		"CREATE": true, "DROP": true, "ALTER": true, "INDEX": true,
		"REFERENCES": true, "TRIGGER": true, "EXECUTE": true,

		// MySQL/MariaDB specific
		"ALL": true, "ALL PRIVILEGES": true, "RELOAD": true,
		"SHUTDOWN": true, "PROCESS": true, "FILE": true, "GRANT OPTION": true,
		"REPLICATION CLIENT": true, "REPLICATION SLAVE": true, "SHOW DATABASES": true,
		"SUPER": true, "CREATE TEMPORARY TABLES": true, "LOCK TABLES": true,
		"CREATE VIEW": true, "SHOW VIEW": true, "CREATE ROUTINE": true,
		"ALTER ROUTINE": true, "CREATE USER": true, "EVENT": true,

		// PostgreSQL specific
		"CONNECT": true, "TEMPORARY": true, "TEMP": true,
		"RULE": true, "ON TABLESPACE": true,
		"ON SEQUENCE": true, "ON FUNCTION": true, "ON TYPE": true,
		"ON SCHEMA": true, "ON LANGUAGE": true, "ON DOMAIN": true,
		"ON FOREIGN DATA WRAPPER": true, "ON FOREIGN SERVER": true,
		"ON LARGE OBJECT": true,

		// SQL Server specific
		"CONTROL": true, "TAKE OWNERSHIP": true, "IMPERSONATE": true,
		"VIEW CHANGE TRACKING":       true,
		"ADMINISTER BULK OPERATIONS": true, "ALTER ANY": true, "CONTROL SERVER": true,
		"CREATE ANY": true, "VIEW ANY": true,
		"ALTER ANY LOGIN": true, "ALTER ANY SERVER ROLE": true,
		"ALTER ANY DATABASE": true, "ALTER ANY USER": true,
		"ALTER ANY ROLE": true, "ALTER ANY SCHEMA": true,
		"ALTER ANY ASSEMBLY": true, "ALTER ANY ASYMMETRIC KEY": true,
		"ALTER ANY CERTIFICATE": true, "ALTER ANY CONTRACT": true,
		"ALTER ANY CREDENTIAL": true, "ALTER ANY DATABASE SCOPED CREDENTIAL": true,
		"ALTER ANY ENDPOINT": true, "ALTER ANY EXTERNAL DATA SOURCE": true,
		"ALTER ANY EXTERNAL FILE FORMAT": true, "ALTER ANY EXTERNAL LIBRARY": true,
		"ALTER ANY FULLTEXT CATALOG": true, "ALTER ANY MASK": true,
		"ALTER ANY MESSAGE TYPE": true, "ALTER ANY REMOTE SERVICE BINDING": true,
		"ALTER ANY ROUTE": true, "ALTER ANY SECURITY POLICY": true,
		"ALTER ANY SERVICE": true, "ALTER ANY SYMMETRIC KEY": true,
		"ALTER ANY WORKLOAD GROUP": true, "AUTHENTICATE": true,
		"BACKUP DATABASE": true, "BACKUP LOG": true, "CHECKPOINT": true,
		"CONNECT ANY DATABASE": true, "CONNECT REPLICATION": true,
		"CONNECT SQL": true, "CONTROL ANY DATABASE": true,
		"CREATE ANY DATABASE": true, "CREATE DDL EVENT NOTIFICATION": true,
		"CREATE ENDPOINT": true, "CREATE TRACE EVENT NOTIFICATION": true,
		"EXTERNAL ACCESS ASSEMBLY": true, "IMPERSONATE ANY LOGIN": true,
		"SELECT ALL USER SECURABLES": true,
		"UNSAFE ASSEMBLY":            true, "VIEW ANY COLUMN ENCRYPTION KEY DEFINITION": true,
		"VIEW ANY COLUMN MASTER KEY DEFINITION": true, "VIEW ANY DEFINITION": true,
		"VIEW ANY PERFORMANCE DEFINITION": true, "VIEW ANY SECURITY DEFINITION": true,
		"VIEW ANY STATE": true,
	}

	for _, privilege := range privileges {
		if !validPrivileges[strings.ToUpper(privilege)] {
			return fmt.Errorf("invalid privilege '%s'. Valid privileges include: SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, ALTER, INDEX, REFERENCES, TRIGGER, EXECUTE, ALL, USAGE, CONNECT, CONTROL, etc.", privilege)
		}
	}

	return nil
}

// validateDatastoreExists validates that the referenced datastore exists
func (r *RoleWebhook) validateDatastoreExists(ctx context.Context, role *dbv1.DatabaseRole) error {
	var datastore dbv1.Datastore
	key := client.ObjectKey{
		Namespace: role.Namespace,
		Name:      role.Spec.DatastoreRef.Name,
	}
	if err := r.Client.Get(ctx, key, &datastore); err != nil {
		return fmt.Errorf("datastore '%s' not found: %v", role.Spec.DatastoreRef.Name, err)
	}
	return nil
}

// validateDatabaseExists validates that the referenced database exists
func (r *RoleWebhook) validateDatabaseExists(ctx context.Context, role *dbv1.DatabaseRole) error {
	var database dbv1.Database
	key := client.ObjectKey{
		Namespace: role.Namespace,
		Name:      role.Spec.DatabaseRef.Name,
	}
	if err := r.Client.Get(ctx, key, &database); err != nil {
		return fmt.Errorf("database '%s' not found: %v", role.Spec.DatabaseRef.Name, err)
	}
	return nil
}

// findUsersUsingRole finds users that are using the specified role
func (r *RoleWebhook) findUsersUsingRole(ctx context.Context, role *dbv1.DatabaseRole) ([]string, error) {
	var userList dbv1.UserList
	if err := r.Client.List(ctx, &userList, client.InNamespace(role.Namespace)); err != nil {
		return nil, err
	}

	var users []string
	for _, user := range userList.Items {
		for _, userRole := range user.Spec.Roles {
			if userRole == role.Name {
				users = append(users, user.Name)
				break
			}
		}
	}

	return users, nil
}
