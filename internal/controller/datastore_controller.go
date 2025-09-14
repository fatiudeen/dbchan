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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
)

// getSecretKeys returns a slice of all keys in the secret data
func getSecretKeys(data map[string][]byte) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}

// DatastoreReconciler reconciles a Datastore object
type DatastoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=datastores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=datastores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.fatiudeen.dev,resources=datastores/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// buildConnectionString constructs a database connection string based on the datastore type, name and credentials
func buildConnectionString(datastoreType, datastoreName, host, username, password string, port int32, sslMode, instance string) string {
	switch datastoreType {
	case "mysql", "mariadb":
		// Set default port if not specified
		if port == 0 {
			port = 3306
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", username, password, host, port, datastoreName)

	case "postgres", "postgresql":
		// Set default port if not specified
		if port == 0 {
			port = 5432
		}
		// Set default SSL mode if not specified
		if sslMode == "" {
			sslMode = "disable"
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", username, password, host, port, datastoreName, sslMode)

	case "sqlserver", "mssql":
		// Set default port if not specified
		if port == 0 {
			port = 1433
		}
		instancePath := ""
		if instance != "" {
			instancePath = "/" + instance
		}
		return fmt.Sprintf("sqlserver://%s:%s@%s:%d%s?database=%s", username, password, host, port, instancePath, datastoreName)

	default:
		// Fallback to a generic connection string format
		return fmt.Sprintf("%s@%s", username, datastoreName)
	}
}

// testConnection attempts to connect to the database using the provided connection string and driver
func testConnection(ctx context.Context, datastoreType, connectionString string) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Map datastore types to their drivers
	var driver string
	switch datastoreType {
	case "mysql", "mariadb":
		driver = "mysql"
	case "postgres", "postgresql":
		driver = "postgres"
	case "sqlserver", "mssql":
		driver = "sqlserver"
	default:
		return fmt.Errorf("unsupported datastore type: %s", datastoreType)
	}

	db, err := sql.Open(driver, connectionString)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v", err)
	}
	defer db.Close()

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// Connection successful
	return nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatastoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "datastore", req.NamespacedName)

	// Fetch the Datastore instance
	datastore := &dbv1.Datastore{}
	if err := r.Get(ctx, req.NamespacedName, datastore); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Datastore resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Datastore")
		return ctrl.Result{}, err
	}

	// Initialize status if not set
	if datastore.Status.Phase == "" {
		datastore.Status.Phase = "Connecting"
		datastore.Status.Ready = false
		datastore.Status.Message = "Attempting to connect to database"
		if err := r.Status().Update(ctx, datastore); err != nil {
			logger.Error(err, "Failed to update datastore status")
			return ctrl.Result{}, err
		}
	}

	// Get the secret containing database credentials (support cross-namespace references)
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: req.Namespace, // Default to same namespace
		Name:      datastore.Spec.SecretRef.Name,
	}

	logger.Info("Looking for secret", "secret", datastore.Spec.SecretRef.Name, "namespace", req.Namespace)

	// Try to get secret from same namespace first
	if err := r.Get(ctx, secretKey, secret); err != nil {
		// If not found in same namespace, try to find it in any namespace
		if errors.IsNotFound(err) {
			logger.Info("Secret not found in same namespace, searching all namespaces")
			// List all secrets to find the one with matching name
			secretList := &corev1.SecretList{}
			if listErr := r.List(ctx, secretList); listErr != nil {
				datastore.Status.Phase = "Failed"
				datastore.Status.Ready = false
				datastore.Status.Message = fmt.Sprintf("Failed to list secrets: %v", listErr)
				if err := r.Status().Update(ctx, datastore); err != nil {
					logger.Error(err, "Failed to update datastore status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, listErr
			}

			// Find secret by name across all namespaces
			found := false
			logger.Info("Found secrets", "count", len(secretList.Items))
			for _, s := range secretList.Items {
				logger.V(1).Info("Checking secret", "name", s.Name, "namespace", s.Namespace)
				if s.Name == datastore.Spec.SecretRef.Name {
					secret = &s
					secretKey.Namespace = s.Namespace
					found = true
					logger.Info("Found secret in different namespace", "secret", s.Name, "namespace", s.Namespace)
					break
				}
			}

			if !found {
				datastore.Status.Phase = "Failed"
				datastore.Status.Ready = false
				datastore.Status.Message = fmt.Sprintf("Secret %s not found in any namespace", datastore.Spec.SecretRef.Name)
				if err := r.Status().Update(ctx, datastore); err != nil {
					logger.Error(err, "Failed to update datastore status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			logger.Error(err, "Failed to get secret from same namespace")
			datastore.Status.Phase = "Failed"
			datastore.Status.Ready = false
			datastore.Status.Message = fmt.Sprintf("Secret %s not found", datastore.Spec.SecretRef.Name)
			if err := r.Status().Update(ctx, datastore); err != nil {
				logger.Error(err, "Failed to update datastore status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Get password from secret
	passwordKey := datastore.Spec.SecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}
	logger.Info("Using password key", "key", passwordKey)

	// Extract password from secret
	password, ok := secret.Data[passwordKey]
	if !ok {
		logger.Error(nil, "Password key not found in secret", "key", passwordKey, "available_keys", getSecretKeys(secret.Data))
		datastore.Status.Phase = "Failed"
		datastore.Status.Ready = false
		datastore.Status.Message = fmt.Sprintf("Key %s not found in secret", passwordKey)
		if err := r.Status().Update(ctx, datastore); err != nil {
			logger.Error(err, "Failed to update datastore status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	logger.Info("Found password in secret")

	// Try to connect to the database
	logger.Info("Building connection string", "datastore_type", datastore.Spec.DatastoreType, "datastore_name", datastore.Name, "host", datastore.Spec.Host, "username", datastore.Spec.Username)
	connectionString := buildConnectionString(datastore.Spec.DatastoreType, datastore.Name, datastore.Spec.Host, datastore.Spec.Username, string(password), datastore.Spec.Port, datastore.Spec.SSLMode, datastore.Spec.Instance)
	logger.V(1).Info("Connection string built", "connection_string", connectionString)

	logger.Info("Testing database connection")
	if err := testConnection(ctx, datastore.Spec.DatastoreType, connectionString); err != nil {
		logger.Error(err, "Database connection failed")
		datastore.Status.Phase = "Failed"
		datastore.Status.Ready = false
		datastore.Status.Message = fmt.Sprintf("Connection failed: %v", err)
		if err := r.Status().Update(ctx, datastore); err != nil {
			logger.Error(err, "Failed to update datastore status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Connection successful
	logger.Info("Database connection successful")
	datastore.Status.Phase = "Ready"
	datastore.Status.Ready = true
	datastore.Status.Message = "Successfully connected to database"

	logger.Info("Updating datastore status to Ready")
	if err := r.Status().Update(ctx, datastore); err != nil {
		logger.Error(err, "Failed to update datastore status")
		return ctrl.Result{}, err
	}

	logger.Info("Datastore reconciliation completed successfully")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatastoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.Datastore{}).
		Complete(r)
}
