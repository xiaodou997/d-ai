package transport

import (
	"context"
	"encoding/json"

	"xiaodou/dai/internal/ai/domain"
)

// voidAdminAudit records management audit events without allowing an audit
// persistence failure to change the business operation's response.
func voidAdminAudit(ctx context.Context, recorder AdminAuditRecorder, action, objectType, objectID string, summary map[string]any, result string, httpStatus int32) {
	if recorder == nil {
		return
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		raw = []byte("{}")
	}
	var status *int32
	if httpStatus > 0 {
		status = &httpStatus
	}
	_ = recorder.Record(ctx, domain.AdminAuditEvent{
		Actor:          claimsUserID(ctx),
		Action:         action,
		ObjectType:     objectType,
		ObjectID:       objectID,
		RequestSummary: raw,
		Result:         result,
		HttpStatus:     status,
	})
}
