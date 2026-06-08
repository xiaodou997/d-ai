package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svclimit "xiaodou/unihub/ai-service/internal/service/limit"
)

// LimitRepo implements service/limit.Repository on top of sqlc.
type LimitRepo struct {
	q *dbgen.Queries
}

func NewLimitRepo(q *dbgen.Queries) *LimitRepo {
	return &LimitRepo{q: q}
}

var _ svclimit.Repository = (*LimitRepo)(nil)

func (r *LimitRepo) Create(ctx context.Context, w svclimit.PolicyWrite) (domain.RuntimeLimitPolicy, error) {
	row, err := r.q.CreateLimitPolicy(ctx, dbgen.CreateLimitPolicyParams{
		ScopeType:        w.ScopeType,
		ScopeID:          w.ScopeID,
		CapabilityType:   w.CapabilityType,
		ModelCode:        akText(w.ModelCode),
		RpmLimit:         akInt4Ptr(w.RpmLimit),
		TpmLimit:         akInt4Ptr(w.TpmLimit),
		ConcurrencyLimit: akInt4Ptr(w.ConcurrencyLimit),
		Status:           w.Status,
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

func (r *LimitRepo) Update(ctx context.Context, id string, w svclimit.PolicyWrite) (domain.RuntimeLimitPolicy, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.RuntimeLimitPolicy{}, err
	}
	row, err := r.q.UpdateLimitPolicy(ctx, dbgen.UpdateLimitPolicyParams{
		ID:               uid,
		ScopeType:        w.ScopeType,
		ScopeID:          w.ScopeID,
		CapabilityType:   w.CapabilityType,
		ModelCode:        akText(w.ModelCode),
		RpmLimit:         akInt4Ptr(w.RpmLimit),
		TpmLimit:         akInt4Ptr(w.TpmLimit),
		ConcurrencyLimit: akInt4Ptr(w.ConcurrencyLimit),
		Status:           w.Status,
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

func limitFromRow(row dbgen.AiRuntimeLimitPolicy) domain.RuntimeLimitPolicy {
	return domain.RuntimeLimitPolicy{
		ID:               uuidToString(row.ID),
		ScopeType:        row.ScopeType,
		ScopeID:          row.ScopeID,
		CapabilityType:   row.CapabilityType,
		ModelCode:        akTextStrPtr(row.ModelCode),
		RpmLimit:         akInt4StrPtr(row.RpmLimit),
		TpmLimit:         akInt4StrPtr(row.TpmLimit),
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
