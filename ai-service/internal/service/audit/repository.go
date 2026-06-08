// Package audit holds the business logic for the admin audit-log read API
// (the console management plane). It is read-only. The underlying query takes
// only a row limit (no filtering, offset or total count), so the port mirrors
// that. Service owns limit normalization; persistence is reached through
// Repository, defined on the consumer side.
package audit

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the audit service.
type Repository interface {
	List(ctx context.Context, limit int32) ([]domain.AuditLog, error)
}
