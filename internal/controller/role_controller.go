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

package controller

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoleReconciler reconciles a Role object
type RoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=db.fatiudeen.dev,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=db.fatiudeen.dev,resources=roles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=db.fatiudeen.dev,resources=roles/finalizers,verbs=update
//+kubebuilder:rbac:groups=db.fatiudeen.dev,resources=datastores,verbs=get;list;watch
//+kubebuilder:rbac:groups=db.fatiudeen.dev,resources=databases,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling Role", "name", req.Name, "namespace", req.Namespace)

	// Fetch the Role instance
	var role dbv1.DatabaseRole
	if err := r.Get(ctx, req.NamespacedName, &role); err != nil {
		logger.Error(err, "unable to fetch Role")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the Role is being deleted
	if !role.DeletionTimestamp.IsZero() {
		logger.Info("Role is being deleted, handling finalizer", "name", role.Name)
		return r.handleDeletion(ctx, &role)
	}

	// Add finalizer if not present
	if !containsString(role.Finalizers, "db.fatiudeen.dev/role-finalizer") {
		logger.Info("Adding finalizer to Role", "name", role.Name)
		role.Finalizers = append(role.Finalizers, "db.fatiudeen.dev/role-finalizer")
		if err := r.Update(ctx, &role); err != nil {
			logger.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get the referenced Datastore
	var datastore dbv1.Datastore
	datastoreKey := client.ObjectKey{
		Namespace: role.Namespace,
		Name:      role.Spec.DatastoreRef.Name,
	}
	if err := r.Get(ctx, datastoreKey, &datastore); err != nil {
		logger.Error(err, "unable to fetch Datastore", "datastore", role.Spec.DatastoreRef.Name)
		return r.updateRoleStatus(ctx, &role, "Failed", false, false, fmt.Sprintf("Failed to fetch Datastore: %v", err))
	}

	// Check if Datastore is ready
	if datastore.Status.Phase != "Ready" || !datastore.Status.Ready {
		logger.Info("Datastore is not ready, requeuing", "datastore", datastore.Name, "phase", datastore.Status.Phase)
		return r.updateRoleStatus(ctx, &role, "Pending", false, false, "Waiting for Datastore to be ready")
	}

	// Get the referenced Database if specified
	var database *dbv1.Database
	if role.Spec.DatabaseRef != nil {
		var db dbv1.Database
		databaseKey := client.ObjectKey{
			Namespace: role.Namespace,
			Name:      role.Spec.DatabaseRef.Name,
		}
		if err := r.Get(ctx, databaseKey, &db); err != nil {
			logger.Error(err, "unable to fetch Database", "database", role.Spec.DatabaseRef.Name)
			return r.updateRoleStatus(ctx, &role, "Failed", false, false, fmt.Sprintf("Failed to fetch Database: %v", err))
		}
		database = &db

		// Check if Database is ready
		if database.Status.Phase != "Ready" || !database.Status.Ready {
			logger.Info("Database is not ready, requeuing", "database", database.Name, "phase", database.Status.Phase)
			return r.updateRoleStatus(ctx, &role, "Pending", false, false, "Waiting for Database to be ready")
		}
	}

	// Get the datastore secret
	secretKey := client.ObjectKey{
		Namespace: role.Namespace,
		Name:      datastore.Spec.SecretRef.Name,
	}
	var secret corev1.Secret
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		logger.Error(err, "unable to fetch Secret", "secret", datastore.Spec.SecretRef.Name)
		return r.updateRoleStatus(ctx, &role, "Failed", false, false, fmt.Sprintf("Failed to fetch Secret: %v", err))
	}

	// Create or update the role in the database
	if err := r.createOrUpdateRole(ctx, &role, &datastore, database, &secret); err != nil {
		logger.Error(err, "failed to create/update role")
		return r.updateRoleStatus(ctx, &role, "Failed", false, false, fmt.Sprintf("Failed to create/update role: %v", err))
	}

	// Update status to success
	return r.updateRoleStatus(ctx, &role, "Ready", true, true, "Role created/updated successfully")
}

// handleDeletion handles the deletion of a Role
func (r *RoleReconciler) handleDeletion(ctx context.Context, role *dbv1.DatabaseRole) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the referenced Datastore
	var datastore dbv1.Datastore
	datastoreKey := client.ObjectKey{
		Namespace: role.Namespace,
		Name:      role.Spec.DatastoreRef.Name,
	}
	if err := r.Get(ctx, datastoreKey, &datastore); err != nil {
		logger.Error(err, "unable to fetch Datastore for deletion", "datastore", role.Spec.DatastoreRef.Name)
		// Continue with finalizer removal even if datastore is not found
	} else {
		// Get the datastore secret
		secretKey := client.ObjectKey{
			Namespace: role.Namespace,
			Name:      datastore.Spec.SecretRef.Name,
		}
		var secret corev1.Secret
		if err := r.Get(ctx, secretKey, &secret); err != nil {
			logger.Error(err, "unable to fetch Secret for deletion", "secret", datastore.Spec.SecretRef.Name)
			// Continue with finalizer removal even if secret is not found
		} else {
			// Delete the role from the database
			if err := r.deleteRole(ctx, role, &datastore, &secret); err != nil {
				logger.Error(err, "failed to delete role from database")
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer
	role.Finalizers = removeString(role.Finalizers, "db.fatiudeen.dev/role-finalizer")
	if err := r.Update(ctx, role); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Role deleted successfully", "name", role.Name)
	return ctrl.Result{}, nil
}

// createOrUpdateRole creates or updates a role in the database
func (r *RoleReconciler) createOrUpdateRole(ctx context.Context, role *dbv1.DatabaseRole, datastore *dbv1.Datastore, database *dbv1.Database, secret *corev1.Secret) error {
	logger := log.FromContext(ctx)

	// Build connection string
	password := string(secret.Data[datastore.Spec.SecretRef.PasswordKey])
	connStr := buildConnectionString(
		datastore.Spec.DatastoreType,
		datastore.Name,
		datastore.Spec.Host,
		datastore.Spec.Username,
		password,
		datastore.Spec.Port,
		datastore.Spec.SSLMode,
		datastore.Spec.Instance,
	)

	// Connect to database
	db, err := connectToDatabase(connStr, datastore.Spec.DatastoreType)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create role based on database type
	switch datastore.Spec.DatastoreType {
	case "mysql", "mariadb":
		return r.createOrUpdateMySQLRole(ctx, role, database, db, logger)
	case "postgresql":
		return r.createOrUpdatePostgreSQLRole(ctx, role, database, db, logger)
	case "sqlserver":
		return r.createOrUpdateSQLServerRole(ctx, role, database, db, logger)
	default:
		return fmt.Errorf("unsupported database type: %s", datastore.Spec.DatastoreType)
	}
}

// createOrUpdateMySQLRole creates or updates a role in MySQL/MariaDB
func (r *RoleReconciler) createOrUpdateMySQLRole(ctx context.Context, role *dbv1.DatabaseRole, database *dbv1.Database, db *sql.DB, logger logr.Logger) error {
	// Create the role
	createRoleSQL := fmt.Sprintf("CREATE ROLE IF NOT EXISTS '%s'", role.Name)
	if _, err := db.ExecContext(ctx, createRoleSQL); err != nil {
		return fmt.Errorf("failed to create role: %v", err)
	}

	// Grant privileges to the role
	if len(role.Spec.Privileges) > 0 {
		for _, privilege := range role.Spec.Privileges {
			var grantSQL string
			if database != nil {
				// Database-specific privileges
				grantSQL = fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'",
					privilege, database.Name, role.Name)
			} else {
				// Global privileges
				grantSQL = fmt.Sprintf("GRANT %s ON *.* TO '%s'",
					privilege, role.Name)
			}
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	}

	// Flush privileges
	if _, err := db.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("failed to flush privileges: %v", err)
	}

	logger.Info("MySQL role created/updated successfully", "role", role.Name)
	return nil
}

// createOrUpdatePostgreSQLRole creates or updates a role in PostgreSQL
func (r *RoleReconciler) createOrUpdatePostgreSQLRole(ctx context.Context, role *dbv1.DatabaseRole, database *dbv1.Database, db *sql.DB, logger logr.Logger) error {
	// Create the role
	createRoleSQL := fmt.Sprintf("CREATE ROLE %s", role.Name)
	if _, err := db.ExecContext(ctx, createRoleSQL); err != nil {
		// Check if role already exists
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create role: %v", err)
		}
		logger.Info("Role already exists, continuing", "role", role.Name)
	}

	// Grant privileges to the role
	if len(role.Spec.Privileges) > 0 {
		for _, privilege := range role.Spec.Privileges {
			var grantSQL string
			if database != nil {
				// Database-specific privileges
				grantSQL = fmt.Sprintf("GRANT %s ON DATABASE \"%s\" TO %s",
					privilege, database.Name, role.Name)
			} else {
				// Global privileges
				grantSQL = fmt.Sprintf("GRANT %s ON ALL DATABASES TO %s",
					privilege, role.Name)
			}
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	}

	logger.Info("PostgreSQL role created/updated successfully", "role", role.Name)
	return nil
}

// createOrUpdateSQLServerRole creates or updates a role in SQL Server
func (r *RoleReconciler) createOrUpdateSQLServerRole(ctx context.Context, role *dbv1.DatabaseRole, database *dbv1.Database, db *sql.DB, logger logr.Logger) error {
	// Create the role
	createRoleSQL := fmt.Sprintf("CREATE ROLE [%s]", role.Name)
	if _, err := db.ExecContext(ctx, createRoleSQL); err != nil {
		// Check if role already exists
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create role: %v", err)
		}
		logger.Info("Role already exists, continuing", "role", role.Name)
	}

	// Grant privileges to the role
	if len(role.Spec.Privileges) > 0 {
		for _, privilege := range role.Spec.Privileges {
			var grantSQL string
			if database != nil {
				// Database-specific privileges
				grantSQL = fmt.Sprintf("GRANT %s TO [%s]",
					privilege, role.Name)
			} else {
				// Global privileges
				grantSQL = fmt.Sprintf("GRANT %s TO [%s]",
					privilege, role.Name)
			}
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	}

	logger.Info("SQL Server role created/updated successfully", "role", role.Name)
	return nil
}

// deleteRole deletes a role from the database
func (r *RoleReconciler) deleteRole(ctx context.Context, role *dbv1.DatabaseRole, datastore *dbv1.Datastore, secret *corev1.Secret) error {
	logger := log.FromContext(ctx)

	// Build connection string
	password := string(secret.Data[datastore.Spec.SecretRef.PasswordKey])
	connStr := buildConnectionString(
		datastore.Spec.DatastoreType,
		datastore.Name,
		datastore.Spec.Host,
		datastore.Spec.Username,
		password,
		datastore.Spec.Port,
		datastore.Spec.SSLMode,
		datastore.Spec.Instance,
	)

	// Connect to database
	db, err := connectToDatabase(connStr, datastore.Spec.DatastoreType)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Delete role based on database type
	switch datastore.Spec.DatastoreType {
	case "mysql", "mariadb":
		dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS '%s'", role.Name)
		if _, err := db.ExecContext(ctx, dropRoleSQL); err != nil {
			return fmt.Errorf("failed to drop role: %v", err)
		}
		// Flush privileges
		if _, err := db.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
			logger.V(1).Info("Failed to flush privileges", "error", err)
		}
	case "postgresql":
		dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS %s", role.Name)
		if _, err := db.ExecContext(ctx, dropRoleSQL); err != nil {
			return fmt.Errorf("failed to drop role: %v", err)
		}
	case "sqlserver":
		dropRoleSQL := fmt.Sprintf("DROP ROLE [%s]", role.Name)
		if _, err := db.ExecContext(ctx, dropRoleSQL); err != nil {
			return fmt.Errorf("failed to drop role: %v", err)
		}
	default:
		return fmt.Errorf("unsupported database type: %s", datastore.Spec.DatastoreType)
	}

	logger.Info("Role deleted successfully", "role", role.Name)
	return nil
}

// updateRoleStatus updates the status of a Role
func (r *RoleReconciler) updateRoleStatus(ctx context.Context, role *dbv1.DatabaseRole, phase string, ready, created bool, message string) (ctrl.Result, error) {
	now := metav1.Now()
	role.Status.Phase = phase
	role.Status.Ready = ready
	role.Status.Created = created
	role.Status.Message = message
	role.Status.LastUpdated = &now

	if err := r.Status().Update(ctx, role); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.DatabaseRole{}).
		Complete(r)
}

// containsString checks if a string slice contains a specific string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString removes a string from a string slice
func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// connectToDatabase establishes a connection to the database
func connectToDatabase(connStr, dbType string) (*sql.DB, error) {
	var driver string
	switch dbType {
	case "mysql", "mariadb":
		driver = "mysql"
	case "postgresql":
		driver = "postgres"
	case "sqlserver":
		driver = "sqlserver"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	db, err := sql.Open(driver, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %v", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	return db, nil
}
