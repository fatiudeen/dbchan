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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	migrationFinalizer = "db.fatiudeen.dev/migration-finalizer"
	migrationsTable    = "schema_migrations"
)

// MigrationReconciler reconciles a Migration object
type MigrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=migrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=migrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=migrations/finalizers,verbs=update
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=datastores,verbs=get;list;watch
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Migration instance
	migration := &dbv1.Migration{}
	if err := r.Get(ctx, req.NamespacedName, migration); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Migration resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Migration")
		return ctrl.Result{}, err
	}

	// Check if the Migration is being deleted
	if migration.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(migration, migrationFinalizer) {
			// Perform rollback if RollbackSQL exists
			if migration.Spec.RollbackSQL != "" {
				if err := r.rollbackMigration(ctx, migration); err != nil {
					logger.Error(err, "Failed to rollback migration")
					return ctrl.Result{}, err
				}
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(migration, migrationFinalizer)
			if err := r.Update(ctx, migration); err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(migration, migrationFinalizer) {
		controllerutil.AddFinalizer(migration, migrationFinalizer)
		if err := r.Update(ctx, migration); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if migration is already applied
	if migration.Status.Applied {
		logger.Info("Migration already applied, skipping")
		return ctrl.Result{}, nil
	}

	// Initialize status if not set
	if migration.Status.Phase == "" {
		migration.Status.Phase = "Pending"
		migration.Status.Applied = false
		migration.Status.Message = "Migration pending"
		if err := r.Status().Update(ctx, migration); err != nil {
			logger.Error(err, "Failed to update migration status")
			return ctrl.Result{}, err
		}
	}

	// Get the database (support cross-namespace references)
	database := &dbv1.Database{}
	databaseKey := client.ObjectKey{
		Namespace: req.Namespace, // Default to same namespace
		Name:      migration.Spec.DatabaseRef.Name,
	}

	// Try to get database from same namespace first
	if err := r.Get(ctx, databaseKey, database); err != nil {
		// If not found in same namespace, try to find it in any namespace
		if errors.IsNotFound(err) {
			// List all databases to find the one with matching name
			databaseList := &dbv1.DatabaseList{}
			if listErr := r.List(ctx, databaseList); listErr != nil {
				migration.Status.Phase = "Failed"
				migration.Status.Applied = false
				migration.Status.Message = fmt.Sprintf("Failed to list databases: %v", listErr)
				if err := r.Status().Update(ctx, migration); err != nil {
					logger.Error(err, "Failed to update migration status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, listErr
			}

			// Find database by name across all namespaces
			found := false
			for _, db := range databaseList.Items {
				if db.Name == migration.Spec.DatabaseRef.Name {
					database = &db
					found = true
					break
				}
			}

			if !found {
				migration.Status.Phase = "Failed"
				migration.Status.Applied = false
				migration.Status.Message = fmt.Sprintf("Database %s not found in any namespace", migration.Spec.DatabaseRef.Name)
				if err := r.Status().Update(ctx, migration); err != nil {
					logger.Error(err, "Failed to update migration status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			migration.Status.Phase = "Failed"
			migration.Status.Applied = false
			migration.Status.Message = fmt.Sprintf("Database %s not found", migration.Spec.DatabaseRef.Name)
			if err := r.Status().Update(ctx, migration); err != nil {
				logger.Error(err, "Failed to update migration status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Check if database is ready
	if !database.Status.Ready {
		migration.Status.Phase = "Waiting"
		migration.Status.Applied = false
		migration.Status.Message = "Waiting for database to be ready"
		if err := r.Status().Update(ctx, migration); err != nil {
			logger.Error(err, "Failed to update migration status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Apply the migration
	if err := r.applyMigration(ctx, migration, database); err != nil {
		migration.Status.Phase = "Failed"
		migration.Status.Applied = false
		migration.Status.Message = fmt.Sprintf("Failed to apply migration: %v", err)
		if err := r.Status().Update(ctx, migration); err != nil {
			logger.Error(err, "Failed to update migration status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Migration applied successfully
	migration.Status.Phase = "Applied"
	migration.Status.Applied = true
	migration.Status.Message = "Migration applied successfully"
	now := metav1.Now()
	migration.Status.AppliedAt = &now

	// Calculate and store checksum
	checksum := r.calculateChecksum(migration.Spec.SQL)
	migration.Status.Checksum = checksum

	if err := r.Status().Update(ctx, migration); err != nil {
		logger.Error(err, "Failed to update migration status")
		return ctrl.Result{}, err
	}

	logger.Info("Migration applied successfully")
	return ctrl.Result{}, nil
}

// applyMigration applies a migration to the database
func (r *MigrationReconciler) applyMigration(ctx context.Context, migration *dbv1.Migration, database *dbv1.Database) error {
	// Get the datastore
	datastore := &dbv1.Datastore{}
	datastoreKey := client.ObjectKey{
		Namespace: database.Namespace,
		Name:      database.Spec.DatastoreRef.Name,
	}
	if err := r.Get(ctx, datastoreKey, datastore); err != nil {
		return fmt.Errorf("failed to get datastore: %v", err)
	}

	// Get datastore credentials
	secret, err := r.getDatastoreSecret(ctx, datastore)
	if err != nil {
		return fmt.Errorf("failed to get datastore secret: %v", err)
	}

	// Build connection string to the specific database
	connectionString := buildMigrationConnectionString(datastore.Spec.DatastoreType, database.Name, secret)

	// Connect to database
	db, err := r.connectToDatabase(ctx, datastore.Spec.DatastoreType, connectionString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create migrations table if it doesn't exist
	if err := r.createMigrationsTable(ctx, db, datastore.Spec.DatastoreType); err != nil {
		return fmt.Errorf("failed to create migrations table: %v", err)
	}

	// Check if migration is already applied
	applied, err := r.isMigrationApplied(ctx, db, migration.Spec.Version, datastore.Spec.DatastoreType)
	if err != nil {
		return fmt.Errorf("failed to check if migration is applied: %v", err)
	}
	if applied {
		// Update status to applied
		migration.Status.Applied = true
		migration.Status.Phase = "Applied"
		migration.Status.Message = "Migration already applied"
		return nil
	}

	// Apply the migration
	if err := r.executeMigration(ctx, db, migration, datastore.Spec.DatastoreType); err != nil {
		return fmt.Errorf("failed to execute migration: %v", err)
	}

	// Record the migration as applied
	if err := r.recordMigration(ctx, db, migration, datastore.Spec.DatastoreType); err != nil {
		return fmt.Errorf("failed to record migration: %v", err)
	}

	return nil
}

// rollbackMigration rolls back a migration
func (r *MigrationReconciler) rollbackMigration(ctx context.Context, migration *dbv1.Migration) error {
	// Get the database
	database := &dbv1.Database{}
	databaseKey := client.ObjectKey{
		Namespace: migration.Namespace,
		Name:      migration.Spec.DatabaseRef.Name,
	}
	if err := r.Get(ctx, databaseKey, database); err != nil {
		// If database is not found, migration is already cleaned up
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get database: %v", err)
	}

	// Get the datastore
	datastore := &dbv1.Datastore{}
	datastoreKey := client.ObjectKey{
		Namespace: database.Namespace,
		Name:      database.Spec.DatastoreRef.Name,
	}
	if err := r.Get(ctx, datastoreKey, datastore); err != nil {
		return fmt.Errorf("failed to get datastore: %v", err)
	}

	// Get datastore credentials
	secret, err := r.getDatastoreSecret(ctx, datastore)
	if err != nil {
		return fmt.Errorf("failed to get datastore secret: %v", err)
	}

	// Build connection string to the specific database
	connectionString := buildMigrationConnectionString(datastore.Spec.DatastoreType, database.Name, secret)

	// Connect to database
	db, err := r.connectToDatabase(ctx, datastore.Spec.DatastoreType, connectionString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Execute rollback SQL
	if migration.Spec.RollbackSQL != "" {
		if err := r.executeRollback(ctx, db, migration, datastore.Spec.DatastoreType); err != nil {
			return fmt.Errorf("failed to execute rollback: %v", err)
		}
	}

	// Remove migration record
	if err := r.removeMigrationRecord(ctx, db, migration.Spec.Version, datastore.Spec.DatastoreType); err != nil {
		return fmt.Errorf("failed to remove migration record: %v", err)
	}

	return nil
}

// getDatastoreSecret retrieves the secret containing datastore credentials
func (r *MigrationReconciler) getDatastoreSecret(ctx context.Context, datastore *dbv1.Datastore) (map[string][]byte, error) {
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

// buildMigrationConnectionString builds a connection string for the specific database
func buildMigrationConnectionString(datastoreType, databaseName string, secretData map[string][]byte) string {
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
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", username, password, host, port, databaseName)
	case "postgres", "postgresql":
		if port == "3306" {
			port = "5432" // default for PostgreSQL
		}
		sslmode := "disable"
		if ssl, ok := secretData["sslmode"]; ok {
			sslmode = string(ssl)
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", username, password, host, port, databaseName, sslmode)
	case "sqlserver", "mssql":
		if port == "3306" {
			port = "1433" // default for SQL Server
		}
		instance := ""
		if inst, ok := secretData["instance"]; ok {
			instance = string(inst)
		}
		return fmt.Sprintf("sqlserver://%s:%s@%s:%s%s?database=%s", username, password, host, port, instance, databaseName)
	default:
		return fmt.Sprintf("%s@%s", username, databaseName)
	}
}

// connectToDatabase establishes a connection to the database
func (r *MigrationReconciler) connectToDatabase(ctx context.Context, datastoreType, connectionString string) (*sql.DB, error) {
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

// createMigrationsTable creates the migrations tracking table
func (r *MigrationReconciler) createMigrationsTable(ctx context.Context, db *sql.DB, datastoreType string) error {
	var createTableSQL string

	switch datastoreType {
	case "mysql", "mariadb":
		createTableSQL = fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				version VARCHAR(255) PRIMARY KEY,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				checksum VARCHAR(64)
			)`, migrationsTable)
	case "postgres", "postgresql":
		createTableSQL = fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				version VARCHAR(255) PRIMARY KEY,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				checksum VARCHAR(64)
			)`, migrationsTable)
	case "sqlserver", "mssql":
		createTableSQL = fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS [%s] (
				[version] NVARCHAR(255) PRIMARY KEY,
				[applied_at] DATETIME2 DEFAULT GETDATE(),
				[checksum] NVARCHAR(64)
			)`, migrationsTable)
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastoreType)
	}

	_, err := db.ExecContext(ctx, createTableSQL)
	return err
}

// isMigrationApplied checks if a migration is already applied
func (r *MigrationReconciler) isMigrationApplied(ctx context.Context, db *sql.DB, version, datastoreType string) (bool, error) {
	var count int
	var query string

	switch datastoreType {
	case "mysql", "mariadb", "postgres", "postgresql":
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE version = ?", migrationsTable)
	case "sqlserver", "mssql":
		query = fmt.Sprintf("SELECT COUNT(*) FROM [%s] WHERE [version] = ?", migrationsTable)
	default:
		return false, fmt.Errorf("unsupported datastore type: %s", datastoreType)
	}

	err := db.QueryRowContext(ctx, query, version).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// executeMigration executes the migration SQL
func (r *MigrationReconciler) executeMigration(ctx context.Context, db *sql.DB, migration *dbv1.Migration, datastoreType string) error {
	// Execute the migration SQL
	if _, err := db.ExecContext(ctx, migration.Spec.SQL); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %v", err)
	}
	return nil
}

// executeRollback executes the rollback SQL
func (r *MigrationReconciler) executeRollback(ctx context.Context, db *sql.DB, migration *dbv1.Migration, datastoreType string) error {
	// Execute the rollback SQL
	if _, err := db.ExecContext(ctx, migration.Spec.RollbackSQL); err != nil {
		return fmt.Errorf("failed to execute rollback SQL: %v", err)
	}
	return nil
}

// recordMigration records a migration as applied
func (r *MigrationReconciler) recordMigration(ctx context.Context, db *sql.DB, migration *dbv1.Migration, datastoreType string) error {
	checksum := r.calculateChecksum(migration.Spec.SQL)
	var insertSQL string

	switch datastoreType {
	case "mysql", "mariadb", "postgres", "postgresql":
		insertSQL = fmt.Sprintf("INSERT INTO %s (version, checksum) VALUES (?, ?)", migrationsTable)
	case "sqlserver", "mssql":
		insertSQL = fmt.Sprintf("INSERT INTO [%s] ([version], [checksum]) VALUES (?, ?)", migrationsTable)
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastoreType)
	}

	_, err := db.ExecContext(ctx, insertSQL, migration.Spec.Version, checksum)
	return err
}

// removeMigrationRecord removes a migration record
func (r *MigrationReconciler) removeMigrationRecord(ctx context.Context, db *sql.DB, version, datastoreType string) error {
	var deleteSQL string

	switch datastoreType {
	case "mysql", "mariadb", "postgres", "postgresql":
		deleteSQL = fmt.Sprintf("DELETE FROM %s WHERE version = ?", migrationsTable)
	case "sqlserver", "mssql":
		deleteSQL = fmt.Sprintf("DELETE FROM [%s] WHERE [version] = ?", migrationsTable)
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastoreType)
	}

	_, err := db.ExecContext(ctx, deleteSQL, version)
	return err
}

// calculateChecksum calculates the SHA256 checksum of the SQL content
func (r *MigrationReconciler) calculateChecksum(sql string) string {
	hash := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(hash[:])
}

// SetupWithManager sets up the controller with the Manager.
func (r *MigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.Migration{}).
		Complete(r)
}
