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

// UserWebhook handles validation for User resources
type UserWebhook struct {
	Client  client.Client
	decoder *admission.Decoder
	Log     logr.Logger
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *UserWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&dbv1.User{}).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-db-fatiudeen-dev-v1-user,mutating=false,failurePolicy=fail,sideEffects=None,groups=db.fatiudeen.dev,resources=users,verbs=create;update,versions=v1,name=vuser.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &UserWebhook{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *UserWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	user, ok := obj.(*dbv1.User)
	if !ok {
		return nil, fmt.Errorf("expected a User but got a %T", obj)
	}

	r.Log.Info("validating user creation", "name", user.Name, "namespace", user.Namespace)

	// Validate required fields
	if err := r.validateRequiredFields(user); err != nil {
		return nil, err
	}

	// Validate naming conventions
	if err := r.validateNamingConventions(user); err != nil {
		return nil, err
	}

	// Validate password format
	if err := r.validatePasswordFormat(user); err != nil {
		return nil, err
	}

	// Validate privileges and roles
	if err := r.validatePrivilegesAndRoles(user); err != nil {
		return nil, err
	}

	// Validate host format
	if err := r.validateHostFormat(user); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *UserWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	user, ok := newObj.(*dbv1.User)
	if !ok {
		return nil, fmt.Errorf("expected a User but got a %T", newObj)
	}

	r.Log.Info("validating user update", "name", user.Name, "namespace", user.Namespace)

	// Validate required fields
	if err := r.validateRequiredFields(user); err != nil {
		return nil, err
	}

	// Validate naming conventions
	if err := r.validateNamingConventions(user); err != nil {
		return nil, err
	}

	// Validate password format
	if err := r.validatePasswordFormat(user); err != nil {
		return nil, err
	}

	// Validate privileges and roles
	if err := r.validatePrivilegesAndRoles(user); err != nil {
		return nil, err
	}

	// Validate host format
	if err := r.validateHostFormat(user); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *UserWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	user, ok := obj.(*dbv1.User)
	if !ok {
		return nil, fmt.Errorf("expected a User but got a %T", obj)
	}

	r.Log.Info("validating user deletion", "name", user.Name, "namespace", user.Namespace)

	return nil, nil
}

// validateRequiredFields checks that all required fields are present
func (r *UserWebhook) validateRequiredFields(user *dbv1.User) error {
	var missingFields []string

	if user.Spec.Username == "" {
		missingFields = append(missingFields, "username")
	}
	if user.Spec.Password == "" {
		missingFields = append(missingFields, "password")
	}
	if user.Spec.DatastoreRef.Name == "" {
		missingFields = append(missingFields, "datastoreRef.name")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missingFields, ", "))
	}

	return nil
}

// validateNamingConventions ensures usernames follow proper conventions
func (r *UserWebhook) validateNamingConventions(user *dbv1.User) error {
	// Validate username
	username := user.Spec.Username

	// Check length (1-32 characters for most databases)
	if len(username) < 1 || len(username) > 32 {
		return fmt.Errorf("username must be between 1 and 32 characters")
	}

	// Check for valid characters (alphanumeric, underscore, hyphen, dot)
	validUsernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !validUsernameRegex.MatchString(username) {
		return fmt.Errorf("username must contain only alphanumeric characters, underscores, hyphens, and dots")
	}

	// Check for reserved usernames
	reservedUsernames := []string{"root", "admin", "mysql", "postgres", "sa", "guest", "test", "user", "system"}
	for _, reserved := range reservedUsernames {
		if strings.EqualFold(username, reserved) {
			return fmt.Errorf("username '%s' is reserved and cannot be used", username)
		}
	}

	// Check for names starting with numbers (some databases don't allow this)
	if regexp.MustCompile(`^[0-9]`).MatchString(username) {
		return fmt.Errorf("username cannot start with a number")
	}

	return nil
}

// validatePasswordFormat validates the password format
func (r *UserWebhook) validatePasswordFormat(user *dbv1.User) error {
	// Check if password is base64 encoded
	if user.Spec.Password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	// Basic validation - password should be base64 encoded
	// We don't decode it here for security reasons, just check format
	base64Regex := regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
	if !base64Regex.MatchString(user.Spec.Password) {
		return fmt.Errorf("password must be base64 encoded")
	}

	// Check minimum length (base64 encoded password should be at least 4 characters)
	if len(user.Spec.Password) < 4 {
		return fmt.Errorf("password is too short")
	}

	return nil
}

// validatePrivilegesAndRoles validates privileges and roles
func (r *UserWebhook) validatePrivilegesAndRoles(user *dbv1.User) error {
	// Validate privileges
	validPrivileges := []string{
		"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "INDEX",
		"REFERENCES", "RELOAD", "SHUTDOWN", "PROCESS", "FILE", "GRANT", "REVOKE",
		"LOCK TABLES", "REPLICATION CLIENT", "REPLICATION SLAVE", "SHOW DATABASES",
		"SUPER", "CREATE TEMPORARY TABLES", "LOCK TABLES", "EXECUTE", "REPLICATION SLAVE",
		"REPLICATION CLIENT", "CREATE VIEW", "SHOW VIEW", "CREATE ROUTINE", "ALTER ROUTINE",
		"CREATE USER", "EVENT", "TRIGGER", "CREATE TABLESPACE", "CREATE ROLE", "DROP ROLE",
		"CONNECT", "TEMPORARY", "USAGE", "EXECUTE", "ALL", "ALL PRIVILEGES",
	}

	for _, privilege := range user.Spec.Privileges {
		valid := false
		for _, validPriv := range validPrivileges {
			if strings.EqualFold(privilege, validPriv) {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid privilege '%s', must be one of: %s",
				privilege, strings.Join(validPrivileges, ", "))
		}
	}

	// Validate roles
	validRoles := []string{
		"readonly", "readwrite", "admin", "developer", "analyst", "backup", "replication",
		"monitoring", "read", "write", "superuser", "user", "guest", "public",
	}

	for _, role := range user.Spec.Roles {
		valid := false
		for _, validRole := range validRoles {
			if strings.EqualFold(role, validRole) {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid role '%s', must be one of: %s",
				role, strings.Join(validRoles, ", "))
		}
	}

	return nil
}

// validateHostFormat validates the host format
func (r *UserWebhook) validateHostFormat(user *dbv1.User) error {
	host := user.Spec.Host

	// If host is empty, use default
	if host == "" {
		user.Spec.Host = "%"
		return nil
	}

	// Validate host format
	validHostPatterns := []string{
		"localhost", "127.0.0.1", "%", "::1", // Common patterns
	}

	// Check if it's a valid IP address
	ipRegex := regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	// Check if it's a valid hostname pattern
	hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
	// Check if it's a wildcard pattern
	wildcardRegex := regexp.MustCompile(`^[%*]+$`)

	isValid := false
	for _, pattern := range validHostPatterns {
		if host == pattern {
			isValid = true
			break
		}
	}

	if !isValid && !ipRegex.MatchString(host) && !hostnameRegex.MatchString(host) && !wildcardRegex.MatchString(host) {
		return fmt.Errorf("invalid host format '%s', must be a valid IP address, hostname, or wildcard pattern", host)
	}

	return nil
}

// InjectDecoder injects the decoder
func (r *UserWebhook) InjectDecoder(d *admission.Decoder) error {
	r.decoder = d
	return nil
}
