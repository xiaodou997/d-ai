package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	commercial "xiaodou/dai/internal/ai/commercial"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

// LimitRepo is the legacy-backed postgres adapter for commercial limit storage.
type LimitRepo struct {
	q *dbgen.Queries
}

func NewLimitRepo(q *dbgen.Queries) *LimitRepo {
	return &LimitRepo{q: q}
}

func (r *LimitRepo) Create(ctx context.Context, w commercial.LimitPolicyWrite) (domain.RuntimeLimitPolicy, error) {
	row, err := r.q.CreateLimitPolicy(ctx, dbgen.CreateLimitPolicyParams{
		ScopeType:        string(w.ScopeType),
		ScopeID:          w.ScopeID,
		ConcurrencyLimit: akInt4Ptr(intPtrToInt32Ptr(w.ConcurrencyLimit)),
		Status:           string(w.Status),
		CreatedBy:        akText(w.CreatedBy),
	})
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	return limitFromRow(row), nil
}

func (r *LimitRepo) List(ctx context.Context) ([]domain.RuntimeLimitPolicy, error) {
	rows, err := r.q.ListLimitPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RuntimeLimitPolicy, 0, len(rows))
	for _, row := range rows {
		out = append(out, limitFromRow(row))
	}
	return out, nil
}

func (r *LimitRepo) Update(ctx context.Context, id string, w commercial.LimitPolicyWrite) (domain.RuntimeLimitPolicy, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	row, err := r.q.UpdateLimitPolicy(ctx, dbgen.UpdateLimitPolicyParams{
		ID:               uid,
		ScopeType:        string(w.ScopeType),
		ScopeID:          w.ScopeID,
		ConcurrencyLimit: akInt4Ptr(intPtrToInt32Ptr(w.ConcurrencyLimit)),
		Status:           string(w.Status),
	})
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	return limitFromRow(row), nil
}

func (r *LimitRepo) UpdateStatus(ctx context.Context, id, status string) (domain.RuntimeLimitPolicy, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	row, err := r.q.UpdateLimitPolicyStatus(ctx, dbgen.UpdateLimitPolicyStatusParams{
		ID:     uid,
		Status: status,
	})
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	return limitFromRow(row), nil
}

func (r *LimitRepo) DeleteByScope(ctx context.Context, scopeType, scopeID string) error {
	return r.q.DeleteLimitPoliciesByScope(ctx, dbgen.DeleteLimitPoliciesByScopeParams{
		ScopeType: scopeType,
		ScopeID:   scopeID,
	})
}

func limitFromRow(row dbgen.AiRuntimeLimitPolicy) domain.RuntimeLimitPolicy {
	return domain.RuntimeLimitPolicy{
		ID:               uuidToString(row.ID),
		ScopeType:        row.ScopeType,
		ScopeID:          row.ScopeID,
		ConcurrencyLimit: akInt4StrPtr(row.ConcurrencyLimit),
		Status:           row.Status,
		CreatedBy:        row.CreatedBy.String,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

// ---- nullable int4 / text pointer <-> pgtype helpers (shared across domains) ----

func akTextStrPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func akInt4Ptr(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func akInt4StrPtr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}
