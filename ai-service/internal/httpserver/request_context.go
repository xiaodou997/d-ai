package httpserver

import "context"

type requestLogContextKey struct{}

type requestLogContext struct {
	TenantID     string
	UserID       string
	APIKeyIDHash string
	Role         string
}

func withRequestLogContext(ctx context.Context) (context.Context, *requestLogContext) {
	logCtx := &requestLogContext{}
	return context.WithValue(ctx, requestLogContextKey{}, logCtx), logCtx
}

func requestLogContextFromContext(ctx context.Context) *requestLogContext {
	logCtx, _ := ctx.Value(requestLogContextKey{}).(*requestLogContext)
	return logCtx
}

func setRequestIdentity(ctx context.Context, tenantID string, userID string, role string) {
	if logCtx := requestLogContextFromContext(ctx); logCtx != nil {
		logCtx.TenantID = tenantID
		logCtx.UserID = userID
		logCtx.Role = role
	}
}

func setRequestAPIKey(ctx context.Context, apiKeyIDHash string, tenantID string, userID string) {
	if logCtx := requestLogContextFromContext(ctx); logCtx != nil {
		logCtx.APIKeyIDHash = apiKeyIDHash
		logCtx.TenantID = tenantID
		logCtx.UserID = userID
	}
}
