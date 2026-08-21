package domain

import "time"

// AuditLog is the management-domain view of a row in ai_admin_audit_logs.
// Actor/ObjectType/ObjectID are empty when the underlying column is NULL;
// RequestSummary is the raw JSON payload; HttpStatus is nil when NULL.
type AuditLog struct {
	ID             string
	Actor          string
	Action         string
	ObjectType     string
	ObjectID       string
	RequestSummary []byte
	Result         string
	HttpStatus     *int32
	CreatedAt      time.Time
}

// AdminAuditEvent is the write-side domain model for an administrative action.
// Optional database columns use empty strings or nil without exposing sqlc or
// pgx types outside the persistence adapter.
type AdminAuditEvent struct {
	Actor          string
	Action         string
	ObjectType     string
	ObjectID       string
	RequestSummary []byte
	Result         string
	HttpStatus     *int32
}
