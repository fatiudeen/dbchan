package webhook

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// DatastoreWebhook handles validation for Datastore resources
type DatastoreWebhook struct {
	Client  client.Client
	decoder *admission.Decoder
	Log     logr.Logger
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *DatastoreWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&dbv1.Datastore{}).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-db-fatiudeen-dev-v1-datastore,mutating=false,failurePolicy=fail,sideEffects=None,groups=db.fatiudeen.dev,resources=datastores,verbs=create;update,versions=v1,name=vdatastore.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &DatastoreWebhook{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DatastoreWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	datastore, ok := obj.(*dbv1.Datastore)
	if !ok {
		return nil, fmt.Errorf("expected a Datastore but got a %T", obj)
	}

	r.Log.Info("validating datastore creation", "name", datastore.Name, "namespace", datastore.Namespace)

	// Validate required fields
	if err := r.validateRequiredFields(datastore); err != nil {
		return nil, err
	}

	// Validate naming conventions
	if err := r.validateNamingConventions(datastore); err != nil {
		return nil, err
	}

	// Validate connection string format
	if err := r.validateConnectionStringFormat(datastore); err != nil {
		return nil, err
	}

	// Validate secret existence
	if err := r.validateSecretExistence(ctx, datastore); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DatastoreWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	datastore, ok := newObj.(*dbv1.Datastore)
	if !ok {
		return nil, fmt.Errorf("expected a Datastore but got a %T", newObj)
	}

	r.Log.Info("validating datastore update", "name", datastore.Name, "namespace", datastore.Namespace)

	// Validate required fields
	if err := r.validateRequiredFields(datastore); err != nil {
		return nil, err
	}

	// Validate naming conventions
	if err := r.validateNamingConventions(datastore); err != nil {
		return nil, err
	}

	// Validate connection string format
	if err := r.validateConnectionStringFormat(datastore); err != nil {
		return nil, err
	}

	// Validate secret existence
	if err := r.validateSecretExistence(ctx, datastore); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DatastoreWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	datastore, ok := obj.(*dbv1.Datastore)
	if !ok {
		return nil, fmt.Errorf("expected a Datastore but got a %T", obj)
	}

	r.Log.Info("validating datastore deletion", "name", datastore.Name, "namespace", datastore.Namespace)

	// Add any deletion validation logic here if needed
	return nil, nil
}

// validateRequiredFields checks that all required fields are present
func (r *DatastoreWebhook) validateRequiredFields(datastore *dbv1.Datastore) error {
	var missingFields []string

	if datastore.Spec.DatastoreType == "" {
		missingFields = append(missingFields, "datastoreType")
	}
	if datastore.Spec.Host == "" {
		missingFields = append(missingFields, "host")
	}
	if datastore.Spec.Username == "" {
		missingFields = append(missingFields, "username")
	}
	if datastore.Spec.SecretRef.Name == "" {
		missingFields = append(missingFields, "secretRef.name")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missingFields, ", "))
	}

	return nil
}

// validateNamingConventions ensures database names follow proper conventions
func (r *DatastoreWebhook) validateNamingConventions(datastore *dbv1.Datastore) error {
	// Validate datastore name (used as database name)
	name := datastore.Name

	// Check length (1-63 characters)
	if len(name) < 1 || len(name) > 63 {
		return fmt.Errorf("datastore name must be between 1 and 63 characters")
	}

	// Check for valid characters (alphanumeric, underscores, and hyphens, must start and end with alphanumeric)
	validNameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]*[a-zA-Z0-9])?$`)
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("datastore name must contain only alphanumeric characters, underscores, and hyphens, and must start and end with alphanumeric characters")
	}

	// Check for reserved names
	reservedNames := []string{"admin", "root", "mysql", "postgres", "information_schema", "performance_schema", "sys"}
	for _, reserved := range reservedNames {
		if strings.EqualFold(name, reserved) {
			return fmt.Errorf("datastore name '%s' is reserved and cannot be used", name)
		}
	}

	return nil
}

// validateConnectionStringFormat validates the connection parameters
func (r *DatastoreWebhook) validateConnectionStringFormat(datastore *dbv1.Datastore) error {
	// Validate datastore type
	validTypes := []string{"mysql", "mariadb", "postgres", "sqlserver"}
	validType := false
	for _, t := range validTypes {
		if datastore.Spec.DatastoreType == t {
			validType = true
			break
		}
	}
	if !validType {
		return fmt.Errorf("invalid datastore type '%s', must be one of: %s",
			datastore.Spec.DatastoreType, strings.Join(validTypes, ", "))
	}

	// Validate port range
	if datastore.Spec.Port < 1 || datastore.Spec.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	// Validate host format (basic validation)
	if datastore.Spec.Host != "" {
		// Check for valid hostname or IP
		hostRegex := regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
		if !hostRegex.MatchString(datastore.Spec.Host) {
			return fmt.Errorf("invalid host format: %s", datastore.Spec.Host)
		}
	}

	// Validate SSL mode for PostgreSQL
	if datastore.Spec.DatastoreType == "postgres" {
		validSSLModes := []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}
		validSSLMode := false
		for _, mode := range validSSLModes {
			if datastore.Spec.SSLMode == mode {
				validSSLMode = true
				break
			}
		}
		if !validSSLMode && datastore.Spec.SSLMode != "" {
			return fmt.Errorf("invalid SSL mode '%s' for PostgreSQL, must be one of: %s",
				datastore.Spec.SSLMode, strings.Join(validSSLModes, ", "))
		}
	}

	// Validate instance name for SQL Server
	if datastore.Spec.DatastoreType == "sqlserver" && datastore.Spec.Instance != "" {
		instanceRegex := regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
		if !instanceRegex.MatchString(datastore.Spec.Instance) {
			return fmt.Errorf("invalid SQL Server instance name format: %s", datastore.Spec.Instance)
		}
	}

	return nil
}

// validateSecretExistence checks that the referenced secret exists
func (r *DatastoreWebhook) validateSecretExistence(ctx context.Context, datastore *dbv1.Datastore) error {
	secretName := datastore.Spec.SecretRef.Name
	secretNamespace := datastore.Namespace

	// Check if secret exists
	secret := &corev1.Secret{}

	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}, secret)

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to check secret existence: %w", err)
		}
		return fmt.Errorf("referenced secret '%s' does not exist in namespace '%s'", secretName, secretNamespace)
	}

	return nil
}

// InjectDecoder injects the decoder
func (r *DatastoreWebhook) InjectDecoder(d *admission.Decoder) error {
	r.decoder = d
	return nil
}
