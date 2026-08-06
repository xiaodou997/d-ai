package transport

import (
	"xiaodou/dai/internal/auth"

	"go.uber.org/zap"
)

func principalLogFields(userID, tenantID string, extra ...zap.Field) []zap.Field {
	fields := make([]zap.Field, 0, len(extra)+2)
	if userID != "" {
		fields = append(fields, zap.String("userId", userID))
	}
	if tenantID != "" {
		fields = append(fields, zap.String("tenantId", tenantID))
	}
	fields = append(fields, extra...)
	return fields
}

func claimsLogFields(claims *auth.Claims, extra ...zap.Field) []zap.Field {
	if claims == nil {
		return extra
	}
	return principalLogFields(claims.UserID, claims.TenantID, extra...)
}
