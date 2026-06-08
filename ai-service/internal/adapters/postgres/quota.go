package postgres

import (
	"context"
	"fmt"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/serving"
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
	identity := req.RuntimeIdentity()
	if identity == nil || !identity.UsesAPIKeyQuota() {
		return nil
	}
	keyID := mustParseUUID(identity.APIKeyID)

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
	identity := req.RuntimeIdentity()
	if identity == nil || !identity.UsesAPIKeyQuota() || amount == 0 {
		return
	}
	_ = r.q.ReleaseAPIKeyQuotaReserve(ctx, dbgen.ReleaseAPIKeyQuotaReserveParams{
		ID:            mustParseUUID(identity.APIKeyID),
		QuotaReserved: amount,
	})
}
