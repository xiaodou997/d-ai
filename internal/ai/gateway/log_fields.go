package gateway

import (
	"context"

	"xiaodou/dai/internal/ai/httpx"

	"go.uber.org/zap"
)

func gatewayLogFields(ctx context.Context, tenantID, userID string, extra ...zap.Field) []zap.Field {
	fields := make([]zap.Field, 0, len(extra)+4)
	if requestID := requestIDFromContext(ctx); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}
	ctxTenantID, ctxUserID, _, apiKeyIDHash := httpx.Identity(ctx)
	if ctxTenantID != "" {
		tenantID = ctxTenantID
	}
	if ctxUserID != "" {
		userID = ctxUserID
	}
	if tenantID != "" {
		fields = append(fields, zap.String("tenant_id", tenantID))
	}
	if userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}
	if apiKeyIDHash != "" {
		fields = append(fields, zap.String("api_key_id_hash", apiKeyIDHash))
	}
	fields = append(fields, extra...)
	return fields
}
