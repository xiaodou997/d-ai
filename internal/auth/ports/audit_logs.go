package ports

import (
	"context"
	"time"
)

// AuthAuditLogFilter is the application-facing filter and pagination contract
// for the platform authentication audit list.
type AuthAuditLogFilter struct {
	EventType     string
	PrincipalType string
	UserID        string
	Decision      string
	Page          int
	Size          int
}

// AuthAuditLog is the non-secret authentication audit projection. Optional
// database columns are represented as empty strings at this boundary.
type AuthAuditLog struct {
	ID            int64
	EventType     string
	PrincipalType string
	UserID        string
	Decision      string
	ReasonCode    string
	ReasonMessage string
	CreatedAt     time.Time
}

type AuthAuditLogPage struct {
	Records []AuthAuditLog
	Total   int64
	Page    int
	Size    int
}

// AuthAuditLogReader exposes the read-only authentication audit capability to
// management handlers without leaking SQL or pgx types.
type AuthAuditLogReader interface {
	ListAuthAuditLogs(ctx context.Context, filter AuthAuditLogFilter) (AuthAuditLogPage, error)
}
