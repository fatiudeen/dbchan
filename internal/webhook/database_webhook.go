package webhook

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
)

// DatabaseWebhook handles validation for Database resources
type DatabaseWebhook struct {
	Client  client.Client
	decoder *admission.Decoder
	Log     logr.Logger
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *DatabaseWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&dbv1.Database{}).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-db-fatiudeen-dev-v1-database,mutating=false,failurePolicy=fail,sideEffects=None,groups=db.fatiudeen.dev,resources=databases,verbs=create;update,versions=v1,name=vdatabase.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &DatabaseWebhook{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DatabaseWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	database, ok := obj.(*dbv1.Database)
	if !ok {
		return nil, fmt.Errorf("expected a Database but got a %T", obj)
	}

	r.Log.Info("validating database creation", "name", database.Name, "namespace", database.Namespace)

	// Validate naming conventions
	if err := r.validateNamingConventions(database); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DatabaseWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	database, ok := newObj.(*dbv1.Database)
	if !ok {
		return nil, fmt.Errorf("expected a Database but got a %T", newObj)
	}

	r.Log.Info("validating database update", "name", database.Name, "namespace", database.Namespace)

	// Validate naming conventions
	if err := r.validateNamingConventions(database); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DatabaseWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	database, ok := obj.(*dbv1.Database)
	if !ok {
		return nil, fmt.Errorf("expected a Database but got a %T", obj)
	}

	r.Log.Info("validating database deletion", "name", database.Name, "namespace", database.Namespace)

	return nil, nil
}

// validateNamingConventions ensures database names follow proper conventions
func (r *DatabaseWebhook) validateNamingConventions(database *dbv1.Database) error {
	// Validate database name
	name := database.Name

	// Check length (1-63 characters)
	if len(name) < 1 || len(name) > 63 {
		return fmt.Errorf("database name must be between 1 and 63 characters")
	}

	// Check for valid characters (alphanumeric, underscores, and hyphens, must start and end with alphanumeric)
	validNameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]*[a-zA-Z0-9])?$`)
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("database name must contain only alphanumeric characters, underscores, and hyphens, and must start and end with alphanumeric characters")
	}

	// Check for reserved names
	reservedNames := []string{"admin", "root", "mysql", "postgres", "information_schema", "performance_schema", "sys", "test", "mysql", "performance_schema", "information_schema"}
	for _, reserved := range reservedNames {
		if strings.EqualFold(name, reserved) {
			return fmt.Errorf("database name '%s' is reserved and cannot be used", name)
		}
	}

	// Check for names starting with hyphens (invalid for SQL)
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("database name cannot start with a hyphen")
	}

	// Check for names ending with hyphens (invalid for SQL)
	if strings.HasSuffix(name, "-") {
		return fmt.Errorf("database name cannot end with a hyphen")
	}

	return nil
}

// InjectDecoder injects the decoder
func (r *DatabaseWebhook) InjectDecoder(d *admission.Decoder) error {
	r.decoder = d
	return nil
}
