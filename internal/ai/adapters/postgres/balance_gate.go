package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/serving"
)

// RuntimeBalanceResolver reads the live package balance and debt used by the
// serving admission gate.
type RuntimeBalanceResolver struct {
	pool *pgxpool.Pool
}

func NewRuntimeBalanceResolver(pool *pgxpool.Pool) *RuntimeBalanceResolver {
	return &RuntimeBalanceResolver{pool: pool}
}

func (r *RuntimeBalanceResolver) ResolveTenantBalance(ctx context.Context, tenantID string) (serving.AccountBalance, error) {
	return r.resolve(ctx, "tenant", tenantID, tenantID, time.Now().UTC())
}

func (r *RuntimeBalanceResolver) ResolveUserBalance(ctx context.Context, tenantID, userID string) (serving.AccountBalance, error) {
	return r.resolve(ctx, "user", tenantID, userID, time.Now().UTC())
}

func (r *RuntimeBalanceResolver) resolve(
	ctx context.Context,
	accountType, tenantID, accountID string,
	now time.Time,
) (serving.AccountBalance, error) {
	var balance serving.AccountBalance
	var err error
	switch accountType {
	case "tenant":
		err = r.pool.QueryRow(ctx, `
			SELECT COALESCE(t.current_overdraft, 0),
			       COALESCE((
			           SELECT SUM(p.remaining_credits)
			           FROM bill_credit_packages p
			           WHERE p.package_type = 'tenant'
			             AND p.tenant_id = t.tenant_id
			             AND p.status = 'available'
			             AND p.remaining_credits > 0
			             AND (p.expires_at IS NULL OR p.expires_at > $2)
			       ), 0)
			FROM iam_tenants t
			WHERE t.tenant_id = $1
		`, accountID, now).Scan(&balance.DebtMicroUSD, &balance.AvailableMicroUSD)
	case "user":
		err = r.pool.QueryRow(ctx, `
			SELECT COALESCE(a.current_overdraft, 0),
			       COALESCE((
			           SELECT SUM(p.remaining_credits)
			           FROM bill_credit_packages p
			           WHERE p.package_type = 'user'
			             AND p.user_id = a.user_id
			             AND p.status = 'available'
			             AND p.remaining_credits > 0
			             AND (p.expires_at IS NULL OR p.expires_at > $3)
			       ), 0)
			FROM iam_accounts a
			WHERE a.tenant_id = $1 AND a.user_id = $2 AND a.user_type = 4
		`, tenantID, accountID, now).Scan(&balance.DebtMicroUSD, &balance.AvailableMicroUSD)
	default:
		return serving.AccountBalance{}, fmt.Errorf("unsupported account type %q", accountType)
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			return serving.AccountBalance{}, fmt.Errorf("%s billing account not found", accountType)
		}
		return serving.AccountBalance{}, fmt.Errorf("resolve %s billing balance: %w", accountType, err)
	}
	return balance, nil
}
