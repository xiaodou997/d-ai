package audit

import (
	"context"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Limit bounds, mirroring the previous handler-layer behaviour: missing/<=0
// limit falls back to the default; anything above the max is capped.
const (
	defaultLimit int32 = 100
	maxLimit     int32 = 500
)

// Service implements audit-log read business logic.
type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns up to a normalized limit of audit logs, newest first. A limit
// of 0 (or negative) uses the default; values above the max are capped.
func (s *Service) List(ctx context.Context, limit int32) ([]domain.AuditLog, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return s.repo.List(ctx, limit)
}
