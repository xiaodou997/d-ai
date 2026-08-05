package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BillingRepository struct {
	pool *pgxpool.Pool
}

func NewBillingRepository(pool *pgxpool.Pool) *BillingRepository {
	return &BillingRepository{pool: pool}
}

// CreditPackage 积分包余额详情
type CreditPackage struct {
	PackageID        string     `json:"packageId"`
	TotalCredits     int64      `json:"totalCredits"`
	RemainingCredits int64      `json:"remainingCredits"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	Source           string     `json:"source"`
}

// CreateCreditPackageParams 创建积分包参数
type CreateCreditPackageParams struct {
	PackageID        string
	PackageType      string
	TenantID         string
	UserID           *string
	TotalCredits     int64
	RemainingCredits int64
	ExpiresAt        *time.Time
	Source           string
	RechargeOrderID  *string
	Status           string
	CreatedAt        *time.Time
	UpdatedAt        *time.Time
}

func (r *BillingRepository) GetTenantBalance(ctx context.Context, tenantID string, now time.Time) (int64, error) {
	var balance int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'tenant' AND tenant_id = $1 AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
	`, tenantID, now).Scan(&balance)
	return balance, err
}

func (r *BillingRepository) GetEndUserBalance(ctx context.Context, userID string, now time.Time) (int64, error) {
	var balance int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'user' AND user_id = $1 AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
	`, userID, now).Scan(&balance)
	return balance, err
}

func (r *BillingRepository) GetTenantFrozenCredits(ctx context.Context, tenantID string) (int64, error) {
	var v int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(frozen_credits, 0) FROM iam_tenants WHERE tenant_id = $1`, tenantID).Scan(&v)
	return v, err
}

func (r *BillingRepository) GetEndUserFrozenCredits(ctx context.Context, userID string) (int64, error) {
	var v int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(frozen_credits, 0) FROM iam_users WHERE user_id = $1`, userID).Scan(&v)
	return v, err
}

// GetTenantOverdraftInfo 返回租户透支额度上限和当前已用额度（非事务，只读）
func (r *BillingRepository) GetTenantOverdraftInfo(ctx context.Context, tenantID string) (limit, current int64, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(overdraft_limit, 0), COALESCE(current_overdraft, 0)
		FROM iam_tenants WHERE tenant_id = $1
	`, tenantID).Scan(&limit, &current)
	return limit, current, err
}

// GetEndUserOverdraftInfo 返回终端用户透支额度上限和当前已用额度（非事务，只读）
func (r *BillingRepository) GetEndUserOverdraftInfo(ctx context.Context, userID string) (limit, current int64, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(overdraft_limit, 0), COALESCE(current_overdraft, 0)
		FROM iam_users WHERE user_id = $1
	`, userID).Scan(&limit, &current)
	return limit, current, err
}

func (r *BillingRepository) GetEndUserTenant(ctx context.Context, userID string) (string, error) {
	var tenantID string
	err := r.pool.QueryRow(ctx, `SELECT tenant_id FROM iam_users WHERE user_id = $1`, userID).Scan(&tenantID)
	return tenantID, err
}

func (r *BillingRepository) ListCreditPackagesForBalance(ctx context.Context, packageType string, tenantID, userID *string, now *time.Time) ([]CreditPackage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT package_id, total_credits, remaining_credits, expires_at, source
		FROM bill_credit_packages
		WHERE package_type = $1
		  AND ($2::text IS NULL OR tenant_id = $2::text)
		  AND ($3::text IS NULL OR user_id = $3::text)
		  AND status = 'available'
		  AND remaining_credits > 0
		  AND (expires_at IS NULL OR expires_at > $4)
		ORDER BY expires_at ASC NULLS LAST, created_at ASC
	`, packageType, tenantID, userID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CreditPackage
	for rows.Next() {
		var p CreditPackage
		if err := rows.Scan(&p.PackageID, &p.TotalCredits, &p.RemainingCredits, &p.ExpiresAt, &p.Source); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (r *BillingRepository) CreateCreditPackage(ctx context.Context, arg CreateCreditPackageParams) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO bill_credit_packages
		(package_id, package_type, tenant_id, user_id,
		 total_credits, remaining_credits, expires_at, source,
		 recharge_order_id, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, $11, $12)
	`, arg.PackageID, arg.PackageType, arg.TenantID, arg.UserID,
		arg.TotalCredits, arg.RemainingCredits, arg.ExpiresAt, arg.Source,
		arg.RechargeOrderID, arg.Status, arg.CreatedAt, arg.UpdatedAt)
	return err
}
