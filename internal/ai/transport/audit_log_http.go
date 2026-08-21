package transport

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
)

// AuditLogHTTPDeps is the complete dependency boundary for the management
// audit-log read route. The write side remains a separate AdminAuditRecorder
// port used by cross-domain mutations in the core AI module.
type AuditLogHTTPDeps struct {
	Auth      HTTPAuthDeps
	AuditLogs AdminAuditLogReader
}

// AdminAuditLogReader exposes the read-only audit projection used by the
// management list endpoint.
type AdminAuditLogReader interface {
	List(ctx context.Context, limit int32) ([]domain.AuditLog, error)
}

// RegisterAuditLog owns the platform-admin authenticated audit-log route.
func RegisterAuditLog(api huma.API, d AuditLogHTTPDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d.Auth))
	registerAudit(management, d)
}
