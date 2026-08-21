package observabilitycontrol

import (
	"context"

	"xiaodou/dai/internal/ai/domain"
)

const (
	defaultAuditLimit int32 = 100
	maxAuditLimit     int32 = 500
)

type AuditRepository interface {
	List(ctx context.Context, limit int32) ([]domain.AuditLog, error)
	Record(ctx context.Context, event domain.AdminAuditEvent) error
}

type AuditService struct {
	repo AuditRepository
}

func NewAuditService(repo AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) List(ctx context.Context, limit int32) ([]domain.AuditLog, error) {
	if limit <= 0 {
		limit = defaultAuditLimit
	}
	if limit > maxAuditLimit {
		limit = maxAuditLimit
	}
	return s.repo.List(ctx, limit)
}

func (s *AuditService) Record(ctx context.Context, event domain.AdminAuditEvent) error {
	return s.repo.Record(ctx, event)
}
