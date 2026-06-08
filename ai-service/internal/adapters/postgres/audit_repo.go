package postgres

import (
	"context"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcaudit "xiaodou/unihub/ai-service/internal/service/audit"
)

// AuditRepo implements service/audit.Repository on top of sqlc.
type AuditRepo struct {
	q *dbgen.Queries
}

func NewAuditRepo(q *dbgen.Queries) *AuditRepo {
	return &AuditRepo{q: q}
}

var _ svcaudit.Repository = (*AuditRepo)(nil)

func (r *AuditRepo) List(ctx context.Context, limit int32) ([]domain.AuditLog, error) {
	rows, err := r.q.ListAuditLogs(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AuditLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.AuditLog{
			ID:             uuidToString(row.ID),
			Actor:          row.Actor.String,
			Action:         row.Action,
			ObjectType:     row.ObjectType.String,
			ObjectID:       row.ObjectID.String,
			RequestSummary: row.RequestSummary,
			Result:         row.Result,
			HttpStatus:     akInt4StrPtr(row.HttpStatus),
			CreatedAt:      row.CreatedAt.Time,
		})
	}
	return out, nil
}
