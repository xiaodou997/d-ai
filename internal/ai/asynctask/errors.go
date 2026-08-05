package asynctask

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrNotFound is returned when a task id does not exist, or exists but is not
// visible to the caller. Callers must not distinguish the two: a tenant probing
// ids should not learn which ones are real.
var ErrNotFound = errors.New("async task not found")

// Error is a client-facing failure with the status code the synchronous
// endpoint would have returned for the same condition.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("asynctask: %s (%d): %s", e.Code, e.Status, e.Message)
}

// Errorf builds an *Error.
func Errorf(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

// AsError extracts an *Error from err, falling back to a generic 500 so callers
// always have a status code to write.
func AsError(err error) *Error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return &Error{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "internal error",
	}
}

// retryableError marks a handler error as worth another attempt.
type retryableError struct{ err error }

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// Retryable marks err as a failure to *start* the work rather than a failure
// *of* the work, asking the engine for another attempt.
//
// It is a request, not a command. The engine still refuses when attempts are
// exhausted, and — more importantly — when the attempt's request_id already has
// an ai_usage_logs row, which means it reached billing and retrying would
// charge twice.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}

// IsRetryable reports whether err was marked by Retryable.
func IsRetryable(err error) bool {
	var r *retryableError
	return errors.As(err, &r)
}
