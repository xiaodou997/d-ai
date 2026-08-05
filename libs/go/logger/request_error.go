package logger

import (
	"context"
	"errors"
)

type requestErrorContextKey struct{}

type requestErrorRecorder struct {
	err error
}

func withRequestErrorRecorder(ctx context.Context) (context.Context, *requestErrorRecorder) {
	recorder := &requestErrorRecorder{}
	return context.WithValue(ctx, requestErrorContextKey{}, recorder), recorder
}

// RecordRequestError attaches the handler-level error to the current request.
// Request logging middleware reads it when the access log line is emitted.
func RecordRequestError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	if recorder, ok := ctx.Value(requestErrorContextKey{}).(*requestErrorRecorder); ok && recorder != nil {
		recorder.err = err
	}
}

func rootCause(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}
