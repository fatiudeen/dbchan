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
	databaseFinalizer = "db.fatiudeen.dev/database-finalizer"
)

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=datastores,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting database reconciliation", "database", req.NamespacedName)

	// Fetch the Database instance
	database := &dbv1.Database{}
	if err := r.Get(ctx, req.NamespacedName, database); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Database resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Database")
		return ctrl.Result{}, err
	}

	// Check if the Database is being deleted
	if database.DeletionTimestamp != nil {
		logger.Info("Database is being deleted", "database", database.Name)
		if controllerutil.ContainsFinalizer(database, databaseFinalizer) {
			logger.Info("Performing database cleanup")
			// Perform cleanup
			if err := r.cleanupDatabase(ctx, database); err != nil {
				logger.Error(err, "Failed to cleanup database")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			logger.Info("Removing finalizer from database")
			controllerutil.RemoveFinalizer(database, databaseFinalizer)
			if err := r.Update(ctx, database); err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
			logger.Info("Database cleanup completed")
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(database, databaseFinalizer) {
		logger.Info("Adding finalizer to database")
		controllerutil.AddFinalizer(database, databaseFinalizer)
		if err := r.Update(ctx, database); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		logger.Info("Finalizer added to database")
	}

	// Initialize status if not set
	if database.Status.Phase == "" {
		database.Status.Phase = "Creating"
		database.Status.Ready = false
		database.Status.Message = "Creating database"
		if err := r.Status().Update(ctx, database); err != nil {
			logger.Error(err, "Failed to update database status")
			return ctrl.Result{}, err
		}
	}

	// Get the datastore (support cross-namespace references)
	datastore := &dbv1.Datastore{}
	datastoreKey := client.ObjectKey{
		Namespace: req.Namespace, // Default to same namespace
		Name:      database.Spec.DatastoreRef.Name,
	}

	// Try to get datastore from same namespace first
	if err := r.Get(ctx, datastoreKey, datastore); err != nil {
		// If not found in same namespace, try to find it in any namespace
		if errors.IsNotFound(err) {
			// List all datastores to find the one with matching name
			datastoreList := &dbv1.DatastoreList{}
			if listErr := r.List(ctx, datastoreList); listErr != nil {
				database.Status.Phase = "Failed"
				database.Status.Ready = false
				database.Status.Message = fmt.Sprintf("Failed to list datastores: %v", listErr)
				if err := r.Status().Update(ctx, database); err != nil {
					logger.Error(err, "Failed to update database status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, listErr
			}

			// Find datastore by name across all namespaces
			found := false
			for _, ds := range datastoreList.Items {
				if ds.Name == database.Spec.DatastoreRef.Name {
					datastore = &ds
					found = true
					break
				}
			}

			if !found {
				database.Status.Phase = "Failed"
				database.Status.Ready = false
				database.Status.Message = fmt.Sprintf("Datastore %s not found in any namespace", database.Spec.DatastoreRef.Name)
				if err := r.Status().Update(ctx, database); err != nil {
					logger.Error(err, "Failed to update database status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			database.Status.Phase = "Failed"
			database.Status.Ready = false
			database.Status.Message = fmt.Sprintf("Datastore %s not found", database.Spec.DatastoreRef.Name)
			if err := r.Status().Update(ctx, database); err != nil {
				logger.Error(err, "Failed to update database status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Check if datastore is ready
	if !datastore.Status.Ready {
		database.Status.Phase = "Waiting"
		database.Status.Ready = false
		database.Status.Message = "Waiting for datastore to be ready"
		if err := r.Status().Update(ctx, database); err != nil {
			logger.Error(err, "Failed to update database status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Create or update the database
	if err := r.createOrUpdateDatabase(ctx, database, datastore); err != nil {
		database.Status.Phase = "Failed"
		database.Status.Ready = false
		database.Status.Message = fmt.Sprintf("Failed to create/update database: %v", err)
		if err := r.Status().Update(ctx, database); err != nil {
			logger.Error(err, "Failed to update database status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Database created/updated successfully
	database.Status.Phase = "Ready"
	database.Status.Ready = true
	database.Status.Message = "Database created successfully"
	if database.Status.CreatedAt == nil {
		now := metav1.Now()
		database.Status.CreatedAt = &now
	}
	if err := r.Status().Update(ctx, database); err != nil {
		logger.Error(err, "Failed to update database status")
		return ctrl.Result{}, err
	}

	logger.Info("Database reconciled successfully")
	return ctrl.Result{}, nil
}

// createOrUpdateDatabase creates or updates a logical database
func (r *DatabaseReconciler) createOrUpdateDatabase(ctx context.Context, database *dbv1.Database, datastore *dbv1.Datastore) error {
	// Get datastore credentials
	secret, err := r.getDatastoreSecret(ctx, datastore)
	if err != nil {
		return fmt.Errorf("failed to get datastore secret: %v", err)
	}

	// Build connection string
	connectionString := buildDatabaseConnectionString(datastore.Spec.DatastoreType, datastore.Name, secret)

	// Connect to database
	db, err := r.connectToDatabase(ctx, datastore.Spec.DatastoreType, connectionString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create or update database based on database type
	switch datastore.Spec.DatastoreType {
	case "mysql", "mariadb":
		return r.createOrUpdateMySQLDatabase(ctx, db, database)
	case "postgres", "postgresql":
		return r.createOrUpdatePostgreSQLDatabase(ctx, db, database)
	case "sqlserver", "mssql":
		return r.createOrUpdateSQLServerDatabase(ctx, db, database)
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastore.Spec.DatastoreType)
	}
}

// cleanupDatabase deletes a logical database
func (r *DatabaseReconciler) cleanupDatabase(ctx context.Context, database *dbv1.Database) error {
	// Get the datastore
	datastore := &dbv1.Datastore{}
	datastoreKey := client.ObjectKey{
		Namespace: database.Namespace,
		Name:      database.Spec.DatastoreRef.Name,
	}
	if err := r.Get(ctx, datastoreKey, datastore); err != nil {
		// If datastore is not found, database is already cleaned up
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
	connectionString := buildDatabaseConnectionString(datastore.Spec.DatastoreType, datastore.Name, secret)

	// Connect to database
	db, err := r.connectToDatabase(ctx, datastore.Spec.DatastoreType, connectionString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Delete database based on database type
	switch datastore.Spec.DatastoreType {
	case "mysql", "mariadb":
		return r.deleteMySQLDatabase(ctx, db, database)
	case "postgres", "postgresql":
		return r.deletePostgreSQLDatabase(ctx, db, database)
	case "sqlserver", "mssql":
		return r.deleteSQLServerDatabase(ctx, db, database)
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastore.Spec.DatastoreType)
	}
}

// getDatastoreSecret retrieves the secret containing datastore credentials
func (r *DatabaseReconciler) getDatastoreSecret(ctx context.Context, datastore *dbv1.Datastore) (map[string][]byte, error) {
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

// buildDatabaseConnectionString builds a connection string for the datastore
func buildDatabaseConnectionString(datastoreType, datastoreName string, secretData map[string][]byte) string {
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
func (r *DatabaseReconciler) connectToDatabase(ctx context.Context, datastoreType, connectionString string) (*sql.DB, error) {
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

// createOrUpdateMySQLDatabase creates or updates a MySQL/MariaDB database
func (r *DatabaseReconciler) createOrUpdateMySQLDatabase(ctx context.Context, db *sql.DB, database *dbv1.Database) error {
	// Create database
	createDBSQL := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", database.Name)
	if database.Spec.Charset != "" || database.Spec.Collation != "" {
		createDBSQL += " CHARACTER SET"
		if database.Spec.Charset != "" {
			createDBSQL += fmt.Sprintf(" %s", database.Spec.Charset)
		} else {
			createDBSQL += " utf8mb4"
		}
		if database.Spec.Collation != "" {
			createDBSQL += fmt.Sprintf(" COLLATE %s", database.Spec.Collation)
		}
	}

	if _, err := db.ExecContext(ctx, createDBSQL); err != nil {
		return fmt.Errorf("failed to create database: %v", err)
	}

	return nil
}

// createOrUpdatePostgreSQLDatabase creates or updates a PostgreSQL database
func (r *DatabaseReconciler) createOrUpdatePostgreSQLDatabase(ctx context.Context, db *sql.DB, database *dbv1.Database) error {
	// Check if database exists
	var exists bool
	checkSQL := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	if err := db.QueryRowContext(ctx, checkSQL, database.Name).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check if database exists: %v", err)
	}

	if !exists {
		// Create database
		createDBSQL := fmt.Sprintf("CREATE DATABASE \"%s\"", database.Name)
		if database.Spec.Charset != "" {
			createDBSQL += fmt.Sprintf(" ENCODING '%s'", database.Spec.Charset)
		}
		if database.Spec.Collation != "" {
			createDBSQL += fmt.Sprintf(" LC_COLLATE '%s'", database.Spec.Collation)
		}

		if _, err := db.ExecContext(ctx, createDBSQL); err != nil {
			return fmt.Errorf("failed to create database: %v", err)
		}
	}

	return nil
}

// createOrUpdateSQLServerDatabase creates or updates a SQL Server database
func (r *DatabaseReconciler) createOrUpdateSQLServerDatabase(ctx context.Context, db *sql.DB, database *dbv1.Database) error {
	// Create database
	createDBSQL := fmt.Sprintf("CREATE DATABASE [%s]", database.Name)
	if database.Spec.Collation != "" {
		createDBSQL += fmt.Sprintf(" COLLATE %s", database.Spec.Collation)
	}

	if _, err := db.ExecContext(ctx, createDBSQL); err != nil {
		// Database might already exist, which is fine
		// Check if it actually exists
		var exists bool
		checkSQL := "SELECT CASE WHEN DB_ID(?) IS NOT NULL THEN 1 ELSE 0 END"
		if err := db.QueryRowContext(ctx, checkSQL, database.Name).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check if database exists: %v", err)
		}
		if !exists {
			return fmt.Errorf("failed to create database: %v", err)
		}
	}

	return nil
}

// deleteMySQLDatabase deletes a MySQL/MariaDB database
func (r *DatabaseReconciler) deleteMySQLDatabase(ctx context.Context, db *sql.DB, database *dbv1.Database) error {
	dropDBSQL := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", database.Name)
	if _, err := db.ExecContext(ctx, dropDBSQL); err != nil {
		return fmt.Errorf("failed to delete database: %v", err)
	}

	return nil
}

// deletePostgreSQLDatabase deletes a PostgreSQL database
func (r *DatabaseReconciler) deletePostgreSQLDatabase(ctx context.Context, db *sql.DB, database *dbv1.Database) error {
	// Terminate connections to the database first
	terminateSQL := fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()", database.Name)
	if _, err := db.ExecContext(ctx, terminateSQL); err != nil {
		// Continue even if termination fails
	}

	dropDBSQL := fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\"", database.Name)
	if _, err := db.ExecContext(ctx, dropDBSQL); err != nil {
		return fmt.Errorf("failed to delete database: %v", err)
	}

	return nil
}

// deleteSQLServerDatabase deletes a SQL Server database
func (r *DatabaseReconciler) deleteSQLServerDatabase(ctx context.Context, db *sql.DB, database *dbv1.Database) error {
	// Set database to single user mode first
	setSingleUserSQL := fmt.Sprintf("ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE", database.Name)
	if _, err := db.ExecContext(ctx, setSingleUserSQL); err != nil {
		// Continue even if this fails
	}

	dropDBSQL := fmt.Sprintf("DROP DATABASE [%s]", database.Name)
	if _, err := db.ExecContext(ctx, dropDBSQL); err != nil {
		return fmt.Errorf("failed to delete database: %v", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.Database{}).
		Complete(r)
}
