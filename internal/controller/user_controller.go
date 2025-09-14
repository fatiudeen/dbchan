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

package controller

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/microsoft/go-mssqldb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
)

const (
	userFinalizer = "db.fatiudeen.dev/user-finalizer"
)

// UserReconciler reconciles a User object
type UserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=users/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=users/finalizers,verbs=update
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=datastores,verbs=get;list;watch
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the User instance
	user := &dbv1.User{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("User resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get User")
		return ctrl.Result{}, err
	}

	// Check if the User is being deleted
	if user.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(user, userFinalizer) {
			// Perform cleanup
			if err := r.cleanupUser(ctx, user); err != nil {
				logger.Error(err, "Failed to cleanup user")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(user, userFinalizer)
			if err := r.Update(ctx, user); err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(user, userFinalizer) {
		controllerutil.AddFinalizer(user, userFinalizer)
		if err := r.Update(ctx, user); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Initialize status if not set
	if user.Status.Phase == "" {
		user.Status.Phase = "Creating"
		user.Status.Ready = false
		user.Status.Created = false
		user.Status.Message = "Creating database user"
		if err := r.Status().Update(ctx, user); err != nil {
			logger.Error(err, "Failed to update user status")
			return ctrl.Result{}, err
		}
	}

	// Get the datastore (support cross-namespace references)
	datastore := &dbv1.Datastore{}
	datastoreKey := client.ObjectKey{
		Namespace: req.Namespace, // Default to same namespace
		Name:      user.Spec.DatastoreRef.Name,
	}

	// Try to get datastore from same namespace first
	if err := r.Get(ctx, datastoreKey, datastore); err != nil {
		// If not found in same namespace, try to find it in any namespace
		if errors.IsNotFound(err) {
			// List all datastores to find the one with matching name
			datastoreList := &dbv1.DatastoreList{}
			if listErr := r.List(ctx, datastoreList); listErr != nil {
				user.Status.Phase = "Failed"
				user.Status.Ready = false
				user.Status.Message = fmt.Sprintf("Failed to list datastores: %v", listErr)
				if err := r.Status().Update(ctx, user); err != nil {
					logger.Error(err, "Failed to update user status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, listErr
			}

			// Find datastore by name across all namespaces
			found := false
			for _, ds := range datastoreList.Items {
				if ds.Name == user.Spec.DatastoreRef.Name {
					datastore = &ds
					found = true
					break
				}
			}

			if !found {
				user.Status.Phase = "Failed"
				user.Status.Ready = false
				user.Status.Message = fmt.Sprintf("Datastore %s not found in any namespace", user.Spec.DatastoreRef.Name)
				if err := r.Status().Update(ctx, user); err != nil {
					logger.Error(err, "Failed to update user status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			user.Status.Phase = "Failed"
			user.Status.Ready = false
			user.Status.Message = fmt.Sprintf("Datastore %s not found", user.Spec.DatastoreRef.Name)
			if err := r.Status().Update(ctx, user); err != nil {
				logger.Error(err, "Failed to update user status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Check if datastore is ready
	if !datastore.Status.Ready {
		user.Status.Phase = "Waiting"
		user.Status.Ready = false
		user.Status.Message = "Waiting for datastore to be ready"
		if err := r.Status().Update(ctx, user); err != nil {
			logger.Error(err, "Failed to update user status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Get database if specified (support cross-namespace references)
	var database *dbv1.Database
	if user.Spec.DatabaseRef != nil {
		database = &dbv1.Database{}
		databaseKey := client.ObjectKey{
			Namespace: req.Namespace, // Default to same namespace
			Name:      user.Spec.DatabaseRef.Name,
		}

		// Try to get database from same namespace first
		if err := r.Get(ctx, databaseKey, database); err != nil {
			// If not found in same namespace, try to find it in any namespace
			if errors.IsNotFound(err) {
				// List all databases to find the one with matching name
				databaseList := &dbv1.DatabaseList{}
				if listErr := r.List(ctx, databaseList); listErr != nil {
					user.Status.Phase = "Failed"
					user.Status.Ready = false
					user.Status.Message = fmt.Sprintf("Failed to list databases: %v", listErr)
					if err := r.Status().Update(ctx, user); err != nil {
						logger.Error(err, "Failed to update user status")
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, listErr
				}

				// Find database by name across all namespaces
				found := false
				for _, db := range databaseList.Items {
					if db.Name == user.Spec.DatabaseRef.Name {
						database = &db
						found = true
						break
					}
				}

				if !found {
					user.Status.Phase = "Failed"
					user.Status.Ready = false
					user.Status.Message = fmt.Sprintf("Database %s not found in any namespace", user.Spec.DatabaseRef.Name)
					if err := r.Status().Update(ctx, user); err != nil {
						logger.Error(err, "Failed to update user status")
						return ctrl.Result{}, err
					}
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			} else {
				user.Status.Phase = "Failed"
				user.Status.Ready = false
				user.Status.Message = fmt.Sprintf("Database %s not found", user.Spec.DatabaseRef.Name)
				if err := r.Status().Update(ctx, user); err != nil {
					logger.Error(err, "Failed to update user status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
	}

	// Create or update the database user
	if err := r.createOrUpdateUser(ctx, user, datastore, database); err != nil {
		user.Status.Phase = "Failed"
		user.Status.Ready = false
		user.Status.Message = fmt.Sprintf("Failed to create/update user: %v", err)
		if err := r.Status().Update(ctx, user); err != nil {
			logger.Error(err, "Failed to update user status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// User created/updated successfully
	user.Status.Phase = "Ready"
	user.Status.Ready = true
	user.Status.Created = true
	user.Status.Message = "User created successfully"
	if user.Status.CreatedAt == nil {
		now := metav1.Now()
		user.Status.CreatedAt = &now
	}
	if err := r.Status().Update(ctx, user); err != nil {
		logger.Error(err, "Failed to update user status")
		return ctrl.Result{}, err
	}

	logger.Info("User reconciled successfully")
	return ctrl.Result{}, nil
}

// createOrUpdateUser creates or updates a database user
func (r *UserReconciler) createOrUpdateUser(ctx context.Context, user *dbv1.User, datastore *dbv1.Datastore, database *dbv1.Database) error {
	// Get datastore credentials
	secret, err := r.getDatastoreSecret(ctx, datastore)
	if err != nil {
		return fmt.Errorf("failed to get datastore secret: %v", err)
	}

	// Build connection string
	connectionString := buildDatastoreConnectionString(datastore.Spec.DatastoreType, datastore.Name, secret)

	// Connect to database
	db, err := r.connectToDatabase(ctx, datastore.Spec.DatastoreType, connectionString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Decode password
	password, err := base64.StdEncoding.DecodeString(user.Spec.Password)
	if err != nil {
		return fmt.Errorf("failed to decode password: %v", err)
	}

	// Create or update user based on database type
	switch datastore.Spec.DatastoreType {
	case "mysql", "mariadb":
		return r.createOrUpdateMySQLUser(ctx, db, user, string(password), database)
	case "postgres", "postgresql":
		return r.createOrUpdatePostgreSQLUser(ctx, db, user, string(password), database)
	case "sqlserver", "mssql":
		return r.createOrUpdateSQLServerUser(ctx, db, user, string(password), database)
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastore.Spec.DatastoreType)
	}
}

// cleanupUser deletes a database user
func (r *UserReconciler) cleanupUser(ctx context.Context, user *dbv1.User) error {
	// Get the datastore
	datastore := &dbv1.Datastore{}
	datastoreKey := client.ObjectKey{
		Namespace: user.Namespace,
		Name:      user.Spec.DatastoreRef.Name,
	}
	if err := r.Get(ctx, datastoreKey, datastore); err != nil {
		// If datastore is not found, user is already cleaned up
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get datastore: %v", err)
	}

	// Get datastore credentials
	secret, err := r.getDatastoreSecret(ctx, datastore)
	if err != nil {
		return fmt.Errorf("failed to get datastore secret: %v", err)
	}

	// Build connection string
	connectionString := buildDatastoreConnectionString(datastore.Spec.DatastoreType, datastore.Name, secret)

	// Connect to database
	db, err := r.connectToDatabase(ctx, datastore.Spec.DatastoreType, connectionString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Delete user based on database type
	switch datastore.Spec.DatastoreType {
	case "mysql", "mariadb":
		return r.deleteMySQLUser(ctx, db, user)
	case "postgres", "postgresql":
		return r.deletePostgreSQLUser(ctx, db, user)
	case "sqlserver", "mssql":
		return r.deleteSQLServerUser(ctx, db, user)
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastore.Spec.DatastoreType)
	}
}

// getDatastoreSecret retrieves the secret containing datastore credentials
func (r *UserReconciler) getDatastoreSecret(ctx context.Context, datastore *dbv1.Datastore) (map[string][]byte, error) {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: datastore.Namespace,
		Name:      datastore.Spec.SecretRef.Name,
	}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		return nil, err
	}
	return secret.Data, nil
}

// buildDatastoreConnectionString builds a connection string for the datastore
func buildDatastoreConnectionString(datastoreType, datastoreName string, secretData map[string][]byte) string {
	// Get default key names
	usernameKey := "username"
	passwordKey := "password"
	hostKey := "host"
	portKey := "port"

	username := string(secretData[usernameKey])
	password := string(secretData[passwordKey])
	host := string(secretData[hostKey])
	port := "3306" // default for MySQL/MariaDB
	if p, ok := secretData[portKey]; ok {
		port = string(p)
	}

	switch datastoreType {
	case "mysql", "mariadb":
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", username, password, host, port, datastoreName)
	case "postgres", "postgresql":
		if port == "3306" {
			port = "5432" // default for PostgreSQL
		}
		sslmode := "disable"
		if ssl, ok := secretData["sslmode"]; ok {
			sslmode = string(ssl)
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", username, password, host, port, datastoreName, sslmode)
	case "sqlserver", "mssql":
		if port == "3306" {
			port = "1433" // default for SQL Server
		}
		instance := ""
		if inst, ok := secretData["instance"]; ok {
			instance = string(inst)
		}
		return fmt.Sprintf("sqlserver://%s:%s@%s:%s%s?database=%s", username, password, host, port, instance, datastoreName)
	default:
		return fmt.Sprintf("%s@%s", username, datastoreName)
	}
}

// connectToDatabase establishes a connection to the database
func (r *UserReconciler) connectToDatabase(ctx context.Context, datastoreType, connectionString string) (*sql.DB, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var driver string
	switch datastoreType {
	case "mysql", "mariadb":
		driver = "mysql"
	case "postgres", "postgresql":
		driver = "postgres"
	case "sqlserver", "mssql":
		driver = "sqlserver"
	default:
		return nil, fmt.Errorf("unsupported datastore type: %s", datastoreType)
	}

	db, err := sql.Open(driver, connectionString)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// createOrUpdateMySQLUser creates or updates a MySQL/MariaDB user
func (r *UserReconciler) createOrUpdateMySQLUser(ctx context.Context, db *sql.DB, user *dbv1.User, password string, database *dbv1.Database) error {
	// Create user
	createUserSQL := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'",
		user.Spec.Username, user.Spec.Host, password)
	if _, err := db.ExecContext(ctx, createUserSQL); err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	// Grant privileges
	if database != nil {
		// Database-specific privileges
		for _, privilege := range user.Spec.Privileges {
			grantSQL := fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%s'",
				privilege, database.Name, user.Spec.Username, user.Spec.Host)
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	} else {
		// Global privileges
		for _, privilege := range user.Spec.Privileges {
			grantSQL := fmt.Sprintf("GRANT %s ON *.* TO '%s'@'%s'",
				privilege, user.Spec.Username, user.Spec.Host)
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	}

	// Grant roles
	for _, role := range user.Spec.Roles {
		grantRoleSQL := fmt.Sprintf("GRANT %s TO '%s'@'%s'", role, user.Spec.Username, user.Spec.Host)
		if _, err := db.ExecContext(ctx, grantRoleSQL); err != nil {
			return fmt.Errorf("failed to grant role %s: %v", role, err)
		}
	}

	// Flush privileges
	if _, err := db.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("failed to flush privileges: %v", err)
	}

	return nil
}

// createOrUpdatePostgreSQLUser creates or updates a PostgreSQL user
func (r *UserReconciler) createOrUpdatePostgreSQLUser(ctx context.Context, db *sql.DB, user *dbv1.User, password string, database *dbv1.Database) error {
	// Create user
	createUserSQL := fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", user.Spec.Username, password)
	if _, err := db.ExecContext(ctx, createUserSQL); err != nil {
		// If user exists, update password
		alterUserSQL := fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s'", user.Spec.Username, password)
		if _, err := db.ExecContext(ctx, alterUserSQL); err != nil {
			return fmt.Errorf("failed to create/update user: %v", err)
		}
	}

	// Grant privileges
	if database != nil {
		// Database-specific privileges
		for _, privilege := range user.Spec.Privileges {
			grantSQL := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s",
				privilege, database.Name, user.Spec.Username)
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	} else {
		// Global privileges
		for _, privilege := range user.Spec.Privileges {
			grantSQL := fmt.Sprintf("GRANT %s TO %s", privilege, user.Spec.Username)
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	}

	// Grant roles
	for _, role := range user.Spec.Roles {
		grantRoleSQL := fmt.Sprintf("GRANT %s TO %s", role, user.Spec.Username)
		if _, err := db.ExecContext(ctx, grantRoleSQL); err != nil {
			return fmt.Errorf("failed to grant role %s: %v", role, err)
		}
	}

	return nil
}

// createOrUpdateSQLServerUser creates or updates a SQL Server user
func (r *UserReconciler) createOrUpdateSQLServerUser(ctx context.Context, db *sql.DB, user *dbv1.User, password string, database *dbv1.Database) error {
	// Create login
	createLoginSQL := fmt.Sprintf("CREATE LOGIN [%s] WITH PASSWORD = '%s'", user.Spec.Username, password)
	if _, err := db.ExecContext(ctx, createLoginSQL); err != nil {
		// If login exists, update password
		alterLoginSQL := fmt.Sprintf("ALTER LOGIN [%s] WITH PASSWORD = '%s'", user.Spec.Username, password)
		if _, err := db.ExecContext(ctx, alterLoginSQL); err != nil {
			return fmt.Errorf("failed to create/update login: %v", err)
		}
	}

	// Create user in database
	if database != nil {
		createUserSQL := fmt.Sprintf("CREATE USER [%s] FOR LOGIN [%s]", user.Spec.Username, user.Spec.Username)
		if _, err := db.ExecContext(ctx, createUserSQL); err != nil {
			// User might already exist, continue
		}

		// Grant privileges
		for _, privilege := range user.Spec.Privileges {
			grantSQL := fmt.Sprintf("GRANT %s TO [%s]", privilege, user.Spec.Username)
			if _, err := db.ExecContext(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant privilege %s: %v", privilege, err)
			}
		}
	}

	return nil
}

// deleteMySQLUser deletes a MySQL/MariaDB user
func (r *UserReconciler) deleteMySQLUser(ctx context.Context, db *sql.DB, user *dbv1.User) error {
	dropUserSQL := fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s'", user.Spec.Username, user.Spec.Host)
	if _, err := db.ExecContext(ctx, dropUserSQL); err != nil {
		return fmt.Errorf("failed to delete user: %v", err)
	}

	// Flush privileges
	if _, err := db.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("failed to flush privileges: %v", err)
	}

	return nil
}

// deletePostgreSQLUser deletes a PostgreSQL user
func (r *UserReconciler) deletePostgreSQLUser(ctx context.Context, db *sql.DB, user *dbv1.User) error {
	dropUserSQL := fmt.Sprintf("DROP USER IF EXISTS %s", user.Spec.Username)
	if _, err := db.ExecContext(ctx, dropUserSQL); err != nil {
		return fmt.Errorf("failed to delete user: %v", err)
	}

	return nil
}

// deleteSQLServerUser deletes a SQL Server user
func (r *UserReconciler) deleteSQLServerUser(ctx context.Context, db *sql.DB, user *dbv1.User) error {
	// Drop user first
	dropUserSQL := fmt.Sprintf("DROP USER IF EXISTS [%s]", user.Spec.Username)
	if _, err := db.ExecContext(ctx, dropUserSQL); err != nil {
		// Continue even if user doesn't exist
	}

	// Drop login
	dropLoginSQL := fmt.Sprintf("DROP LOGIN [%s]", user.Spec.Username)
	if _, err := db.ExecContext(ctx, dropLoginSQL); err != nil {
		return fmt.Errorf("failed to delete login: %v", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.User{}).
		Complete(r)
}
