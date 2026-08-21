package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/billing/ledger"
)

// RuntimeBalanceResolver adapts the billing ledger to the serving admission
// gate. It holds no query of its own: reading a balance any other way is what
// the ledger package exists to prevent.
type RuntimeBalanceResolver struct {
	pool *translatingPool
}

func NewRuntimeBalanceResolver(pool *pgxpool.Pool) *RuntimeBalanceResolver {
	return &RuntimeBalanceResolver{pool: newTranslatingPool(pool)}
}

var _ serving.AccountBalanceResolver = (*RuntimeBalanceResolver)(nil)

// ResolveBalances reads the tenant and (optionally) the end user in one query,
// so the gate compares two numbers taken at the same instant.
func (r *RuntimeBalanceResolver) ResolveBalances(ctx context.Context, tenantID, userID string) (serving.AccountBalances, error) {
	refs := []ledger.Ref{{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}}
	if userID != "" {
		refs = append(refs, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID})
	}
	balances, err := ledger.Balances(ctx, r.pool, refs...)
	if err != nil {
		return serving.AccountBalances{}, err
	}
	tenantBalance, ok := balances[tenantID]
	if !ok {
		return serving.AccountBalances{}, fmt.Errorf("%w: tenant %s", ledger.ErrAccountNotFound, tenantID)
	}
	out := serving.AccountBalances{TenantMicroUSD: tenantBalance}
	if userID != "" {
		out.UserMicroUSD, out.UserPresent = balances[userID]
	}
	return out, nil
}
