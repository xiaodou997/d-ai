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
)

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
