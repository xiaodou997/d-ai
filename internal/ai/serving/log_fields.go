package serving

import "go.uber.org/zap"

func requestLogFields(req *Request, extra ...zap.Field) []zap.Field {
	fields := make([]zap.Field, 0, len(extra)+3)
	if req != nil {
		if req.RequestID != "" {
			fields = append(fields, zap.String("request_id", req.RequestID))
		}
		if subject := req.RuntimeSubject(); subject != nil {
			if subject.TenantID != "" {
				fields = append(fields, zap.String("tenant_id", subject.TenantID))
			}
			if subject.UserID != "" {
				fields = append(fields, zap.String("user_id", subject.UserID))
			}
		}
	}
	fields = append(fields, extra...)
	return fields
}
