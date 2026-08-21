package domain

import "errors"

// Sentinel errors returned by the service layer. The HTTP layer (console /
// gateway) maps these to status codes / envelope codes via errors.Is, so
// services never import net/http.
var (
	// ErrNotFound — requested resource does not exist (or is not visible to
	// the caller's scope).
	ErrNotFound = errors.New("not found")
	// ErrConflict — the operation conflicts with current state (e.g. unique
	// constraint, duplicate resource).
	ErrConflict = errors.New("conflict")
	// ErrForbidden — caller is authenticated but not allowed to perform this
	// action on this resource.
	ErrForbidden = errors.New("forbidden")
	// ErrValidation — input failed business validation. Prefer returning a
	// *ValidationError so the HTTP layer can surface the offending field.
	ErrValidation = errors.New("validation failed")
	// Persistence validation sentinels keep storage-specific SQLSTATE values
	// outside transport while preserving the established HTTP error classes.
	ErrReferencedResourceNotFound = errors.New("referenced resource not found")
	ErrInvalidFieldValue          = errors.New("invalid field value")
	ErrInvalidInputFormat         = errors.New("invalid input format")
)

type PersistenceErrorKind string

const (
	PersistenceNotFound          PersistenceErrorKind = "not_found"
	PersistenceConflict          PersistenceErrorKind = "conflict"
	PersistenceReferenceNotFound PersistenceErrorKind = "reference_not_found"
	PersistenceInvalidField      PersistenceErrorKind = "invalid_field"
	PersistenceInvalidFormat     PersistenceErrorKind = "invalid_format"
)

// PersistenceError classifies a database constraint failure without exposing
// driver types or SQL details to callers. Cause remains available to logs.
type PersistenceError struct {
	Kind  PersistenceErrorKind
	Cause error
}

func (e *PersistenceError) Error() string {
	switch e.Kind {
	case PersistenceNotFound:
		return ErrNotFound.Error()
	case PersistenceConflict:
		return ErrConflict.Error()
	case PersistenceReferenceNotFound:
		return ErrReferencedResourceNotFound.Error()
	case PersistenceInvalidField:
		return ErrInvalidFieldValue.Error()
	case PersistenceInvalidFormat:
		return ErrInvalidInputFormat.Error()
	default:
		return "persistence error"
	}
}

func (e *PersistenceError) Unwrap() error { return e.Cause }

func (e *PersistenceError) Is(target error) bool {
	switch e.Kind {
	case PersistenceNotFound:
		return target == ErrNotFound
	case PersistenceConflict:
		return target == ErrConflict
	case PersistenceReferenceNotFound:
		return target == ErrReferencedResourceNotFound
	case PersistenceInvalidField:
		return target == ErrInvalidFieldValue
	case PersistenceInvalidFormat:
		return target == ErrInvalidInputFormat
	default:
		return false
	}
}

// ValidationError carries a specific business-validation failure. It satisfies
// errors.Is(err, ErrValidation) so callers can branch on the sentinel while the
// HTTP layer renders Field/Message to the client.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

// Is makes *ValidationError match the ErrValidation sentinel under errors.Is.
func (e *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// NewValidationError builds a field-scoped validation error. Pass an empty
// field for messages that are not tied to a single input field.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// DispatchRulePriceConflict describes one group/rule that would become
// unroutable because its retail price book has no matching capability price.
type DispatchRulePriceConflict struct {
	GroupID            string `json:"group_id"`
	GroupName          string `json:"group_name"`
	RuleID             string `json:"rule_id"`
	APIFormat          string `json:"api_format"`
	MatchValue         string `json:"match_value"`
	TargetModel        string `json:"target_model"`
	RequiredCapability string `json:"required_capability"`
}

// DispatchRulePriceConflictError is returned when a control-plane write would
// break the dispatch-to-price invariant. It matches ErrConflict for generic
// callers while retaining structured conflicts for the HTTP transport.
type DispatchRulePriceConflictError struct {
	Conflicts []DispatchRulePriceConflict
}

func (e *DispatchRulePriceConflictError) Error() string {
	return "dispatch rule target is not priced for the required capability"
}

func (e *DispatchRulePriceConflictError) Is(target error) bool {
	return target == ErrConflict
}

type GroupDependencyCounts struct {
	UserBindings      int `json:"user_bindings"`
	APIKeyBindings    int `json:"api_key_bindings"`
	SubscriptionPlans int `json:"subscription_plans"`
}

func (c GroupDependencyCounts) Total() int {
	return c.UserBindings + c.APIKeyBindings + c.SubscriptionPlans
}

// GroupInUseError reports the business references that must be removed before
// group-owned configuration can be deleted.
type GroupInUseError struct {
	GroupID      string                `json:"group_id"`
	GroupName    string                `json:"group_name"`
	Dependencies GroupDependencyCounts `json:"dependencies"`
}

func (e *GroupInUseError) Error() string { return "group is still referenced" }

func (e *GroupInUseError) Is(target error) bool { return target == ErrConflict }
