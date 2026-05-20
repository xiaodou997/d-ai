package postgres

import (
	"context"
	"fmt"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/serving"
)

// QuotaReserver implements serving.QuotaReserver using atomic DB updates.
// The DB query returns the number of rows affected; 0 means insufficient quota.
type QuotaReserver struct {
	q *dbgen.Queries
}

func NewQuotaReserver(q *dbgen.Queries) *QuotaReserver {
	return &QuotaReserver{q: q}
}

// Reserve atomically reserves `amount` quota units on the API key.
// Returns an error if the key has insufficient remaining quota.
func (r *QuotaReserver) Reserve(ctx context.Context, req *serving.Request, amount int64) error {
	if req.APIKey == nil {
		return nil
	}
	keyID := mustParseUUID(req.APIKey.KeyID)

	rows, err := r.q.ReserveAPIKeyQuota(ctx, dbgen.ReserveAPIKeyQuotaParams{
		ID:            keyID,
		QuotaReserved: amount,
	})
	if err != nil {
		return fmt.Errorf("reserve quota: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("insufficient quota")
	}
	return nil
}

// Release returns reserved quota to the API key. Best-effort — errors are logged
// by the caller but do not propagate.
func (r *QuotaReserver) Release(ctx context.Context, req *serving.Request, amount int64) {
	if req.APIKey == nil || amount == 0 {
		return
	}
	_ = r.q.ReleaseAPIKeyQuotaReserve(ctx, dbgen.ReleaseAPIKeyQuotaReserveParams{
		ID:            mustParseUUID(req.APIKey.KeyID),
		QuotaReserved: amount,
	})
}
