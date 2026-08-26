package transport

import "go.uber.org/zap"

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
