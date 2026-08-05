package asynctask

import (
	"context"

	"xiaodou/dai/internal/ai/core/identity"
)

// SubjectRef is everything the engine persists about who submitted a task.
//
// It is a reference, not a snapshot. The engine re-resolves it before every
// attempt, so a revoked key, an exhausted quota or a changed group grant all
// take effect on tasks still sitting in the queue. Snapshotting the expanded
// Subject would be a footgun: a queued task would keep running with the
// authorization it had at submit time.
//
// AuthMethod holds identity.AuthMethod values verbatim (api_key / invoke_key /
// jwt) — this is that enum persisted, so it must not invent a synonym.
type SubjectRef struct {
	AuthMethod  identity.AuthMethod
	TenantID    string
	UserID      string
	APIKeyID    string
	InvokeKeyID string
}

// SubjectRefFrom derives the persistable reference from an authenticated caller.
func SubjectRefFrom(sub identity.Subject) SubjectRef {
	return SubjectRef{
		AuthMethod:  sub.AuthMethod,
		TenantID:    sub.TenantID,
		UserID:      sub.UserID,
		APIKeyID:    sub.APIKeyID,
		InvokeKeyID: sub.InvokeKeyID,
	}
}

// SubjectResolver turns a persisted reference back into a live Subject.
//
// The engine owns no credential lookup of its own — resolving an API key or
// expanding an app key lives in the gateway, and the engine must not depend on
// it. Implementations return an error when the credential is gone; the engine
// fails the task with subject_unavailable rather than running it unauthorized.
type SubjectResolver interface {
	Resolve(ctx context.Context, ref SubjectRef) (identity.Subject, error)
}

// SubjectResolverFunc adapts a function to SubjectResolver.
type SubjectResolverFunc func(ctx context.Context, ref SubjectRef) (identity.Subject, error)

// Resolve implements SubjectResolver.
func (f SubjectResolverFunc) Resolve(ctx context.Context, ref SubjectRef) (identity.Subject, error) {
	return f(ctx, ref)
}
