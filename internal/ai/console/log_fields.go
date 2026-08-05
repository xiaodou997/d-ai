package console

import (
	"context"
	"net/http"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/httpx"

	"go.uber.org/zap"
)

func consoleLogFields(ctx context.Context, extra ...zap.Field) []zap.Field {
	fields := make([]zap.Field, 0, 1+len(extra)+4)
	if requestID := requestIDFromContext(ctx); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}
	fields = append(fields, httpx.IdentityFields(ctx)...)
	fields = append(fields, extra...)
	return fields
}

func consoleRequestLogFields(r *http.Request, extra ...zap.Field) []zap.Field {
	fields := []zap.Field{zap.String("path", r.URL.Path)}
	return consoleLogFields(r.Context(), append(fields, extra...)...)
}

func consoleSubjectLogFields(r *http.Request, subject *coreidentity.Subject, extra ...zap.Field) []zap.Field {
	tenantID, userID, _, _ := httpx.Identity(r.Context())
	if tenantID == "" && subject != nil {
		tenantID = subject.TenantID
	}
	if userID == "" && subject != nil {
		userID = subject.UserID
	}
	fields := []zap.Field{zap.String("path", r.URL.Path)}
	if tenantID != "" {
		fields = append(fields, zap.String("tenant_id", tenantID))
	}
	if userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}
	return consoleLogFields(r.Context(), append(fields, extra...)...)
}
