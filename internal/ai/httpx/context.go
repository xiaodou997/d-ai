// Package httpx holds HTTP-layer infrastructure shared by the runtime gateway
// and the management console: the per-request log context (populated by each
// plane's auth middleware and read by the request logger) plus the request
// logging / panic-recovery middleware.
package httpx

import (
	"context"

	"go.uber.org/zap"
)

type logContextKey struct{}

// LogContext accumulates request-scoped identity for structured access logging.
// Auth middleware writes into it; the RequestLogger middleware reads it when the
// request completes.
type LogContext struct {
	TenantID     string
	UserID       string
	APIKeyIDHash string
	Role         string
}

// WithLogContext attaches a fresh LogContext to ctx and returns both. Called
// once per request by the RequestLogger middleware.
func WithLogContext(ctx context.Context) (context.Context, *LogContext) {
	lc := &LogContext{}
	return context.WithValue(ctx, logContextKey{}, lc), lc
}

func logContextFromContext(ctx context.Context) *LogContext {
	lc, _ := ctx.Value(logContextKey{}).(*LogContext)
	return lc
}

// SetIdentity records tenant/user/role on the request log context (JWT planes).
func SetIdentity(ctx context.Context, tenantID, userID, role string) {
	if lc := logContextFromContext(ctx); lc != nil {
		lc.TenantID = tenantID
		lc.UserID = userID
		lc.Role = role
	}
}

// SetAPIKey records the API-key id hash + tenant/user on the request log
// context (runtime API-key plane).
func SetAPIKey(ctx context.Context, apiKeyIDHash, tenantID, userID string) {
	if lc := logContextFromContext(ctx); lc != nil {
		lc.APIKeyIDHash = apiKeyIDHash
		lc.TenantID = tenantID
		lc.UserID = userID
	}
}

// Identity returns the current request-scoped identity values, if any.
func Identity(ctx context.Context) (tenantID, userID, role, apiKeyIDHash string) {
	if lc := logContextFromContext(ctx); lc != nil {
		return lc.TenantID, lc.UserID, lc.Role, lc.APIKeyIDHash
	}
	return "", "", "", ""
}

// IdentityFields returns zap fields for the current request-scoped identity.
func IdentityFields(ctx context.Context) []zap.Field {
	tenantID, userID, role, apiKeyIDHash := Identity(ctx)
	fields := make([]zap.Field, 0, 4)
	if tenantID != "" {
		fields = append(fields, zap.String("tenant_id", tenantID))
	}
	if userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}
	if role != "" {
		fields = append(fields, zap.String("role", role))
	}
	if apiKeyIDHash != "" {
		fields = append(fields, zap.String("api_key_id_hash", apiKeyIDHash))
	}
	return fields
}
