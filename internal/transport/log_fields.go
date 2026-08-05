package transport

import (
	"xiaodou/dai/internal/auth"

	"go.uber.org/zap"
)

func principalLogFields(userID, tenantID, clientType, clientID string, extra ...zap.Field) []zap.Field {
	fields := make([]zap.Field, 0, len(extra)+4)
	if userID != "" {
		fields = append(fields, zap.String("userId", userID))
	}
	if tenantID != "" {
		fields = append(fields, zap.String("tenantId", tenantID))
	}
	if clientType != "" {
		fields = append(fields, zap.String("clientType", clientType))
	}
	if clientID != "" {
		fields = append(fields, zap.String("clientId", clientID))
	}
	fields = append(fields, extra...)
	return fields
}

func claimsLogFields(claims *auth.Claims, extra ...zap.Field) []zap.Field {
	if claims == nil {
		return extra
	}
	return principalLogFields(claims.UserID, claims.TenantID, claims.ClientType, claims.ClientID, extra...)
}
