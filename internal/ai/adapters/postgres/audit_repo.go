package postgres

import (
	"context"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/observabilitycontrol"
)

// AuditRepo implements observabilitycontrol.AuditRepository on top of sqlc.
type AuditRepo struct {
	q *dbgen.Queries
}

func NewAuditRepo(q *dbgen.Queries) *AuditRepo {
	return &AuditRepo{q: q}
}

var _ observabilitycontrol.AuditRepository = (*AuditRepo)(nil)

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

func (r *AuditRepo) Record(ctx context.Context, event domain.AdminAuditEvent) error {
	_, err := r.q.CreateAuditLog(ctx, dbgen.CreateAuditLogParams{
		Actor:          nullableText(event.Actor),
		Action:         event.Action,
		ObjectType:     nullableText(event.ObjectType),
		ObjectID:       nullableText(event.ObjectID),
		RequestSummary: event.RequestSummary,
		Result:         event.Result,
		HttpStatus:     int32PtrToInt4(event.HttpStatus),
	})
	return err
}
