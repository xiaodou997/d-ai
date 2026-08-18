package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/billing"
)

// Pagination 分页参数
type Pagination struct {
	Page   int64 `json:"page"`
	Size   int64 `json:"size"`
	Offset int64 `json:"-"`
}

func NewPagination(page, size int64) Pagination {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	return Pagination{Page: page, Size: size, Offset: (page - 1) * size}
}

type PaginatedResult[T any] struct {
	Records []T   `json:"records"`
	Total   int64 `json:"total"`
	Page    int64 `json:"page"`
	Size    int64 `json:"size"`
}

type SystemRepository struct {
	pool *pgxpool.Pool
}

func NewSystemRepository(pool *pgxpool.Pool) *SystemRepository {
	return &SystemRepository{pool: pool}
}

type GlobalStatsRow struct {
	Currency                string  `json:"currency"`
	TenantRechargePaidMinor int64   `json:"tenantRechargePaidMinor"`
	TenantRechargeAmountUSD float64 `json:"tenantRechargeAmountUsd"`
	ActiveTenants           int64   `json:"activeTenants"`
	TenantTotalBalanceUSD   float64 `json:"tenantTotalBalanceUsd"`
	UserRechargePaidMinor   int64   `json:"userRechargePaidMinor"`
	UserRechargeAmountUSD   float64 `json:"userRechargeAmountUsd"`
	NewUsers                int64   `json:"newUsers"`
	UserTotalBalanceUSD     float64 `json:"userTotalBalanceUsd"`
}

func (r *SystemRepository) GetGlobalStats(ctx context.Context, timeFrom, timeTo *time.Time) (GlobalStatsRow, error) {
	var row GlobalStatsRow
	var tenantRechargeMicro, tenantTotalMicro, userRechargeMicro, userTotalMicro int64
	err := r.pool.QueryRow(ctx, `
		SELECT
		  COALESCE((SELECT SUM(paid_amount) FROM bill_recharge_orders WHERE order_type IN ('platform_to_tenant', 'online_tenant_topup') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
		  COALESCE((SELECT SUM(credit_amount) FROM bill_recharge_orders WHERE order_type IN ('platform_to_tenant', 'online_tenant_topup') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
		  (SELECT COUNT(*) FROM iam_tenants WHERE status = 'active')::bigint,
		  COALESCE((SELECT SUM(GREATEST(balance_micro, 0)) FROM bill_accounts WHERE account_kind = 1), 0)::bigint,
		  COALESCE((SELECT SUM(paid_amount) FROM bill_recharge_orders WHERE order_type IN ('tenant_to_user', 'online_user_topup') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
		  COALESCE((SELECT SUM(credit_amount) FROM bill_recharge_orders WHERE order_type IN ('tenant_to_user', 'online_user_topup') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
			  (SELECT COUNT(*) FROM iam_accounts WHERE user_type = 4 AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz))::bigint,
		  COALESCE((SELECT SUM(GREATEST(balance_micro, 0)) FROM bill_accounts WHERE account_kind = 2), 0)::bigint
	`, timeFrom, timeTo).Scan(
		&row.TenantRechargePaidMinor, &tenantRechargeMicro,
		&row.ActiveTenants, &tenantTotalMicro,
		&row.UserRechargePaidMinor, &userRechargeMicro,
		&row.NewUsers, &userTotalMicro,
	)
	row.Currency = "USD"
	row.TenantRechargeAmountUSD = billing.MicroToUSD(tenantRechargeMicro)
	row.TenantTotalBalanceUSD = billing.MicroToUSD(tenantTotalMicro)
	row.UserRechargeAmountUSD = billing.MicroToUSD(userRechargeMicro)
	row.UserTotalBalanceUSD = billing.MicroToUSD(userTotalMicro)
	return row, err
}

type ConsumptionTrendRow struct {
	Day   time.Time `json:"day"`
	Total float64   `json:"total"`
}

type GetConsumptionTrendParams struct {
	TimeFrom    *time.Time
	TimeTo      *time.Time
	TenantID    *string
	AccountType *int64
}

func (r *SystemRepository) GetConsumptionTrend(ctx context.Context, params GetConsumptionTrendParams) ([]ConsumptionTrendRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT date_trunc('day', created_at) AS day_bucket,
		       SUM(COALESCE(tenant_payable, 0) + COALESCE(user_charged, 0)) AS total
		FROM ai_usage_logs
		WHERE billing_status = 'settled'
		  AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz)
		  AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)
		  AND ($3::text IS NULL OR tenant_id = $3::text)
		  AND ($4::bigint IS NULL OR
		       ($4::bigint = 1 AND tenant_payable > 0) OR
		       ($4::bigint = 2 AND user_charged > 0))
		GROUP BY day_bucket ORDER BY day_bucket
	`, params.TimeFrom, params.TimeTo, params.TenantID, params.AccountType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ConsumptionTrendRow
	for rows.Next() {
		var r ConsumptionTrendRow
		var totalMicro int64
		if err := rows.Scan(&r.Day, &totalMicro); err != nil {
			return nil, err
		}
		r.Total = billing.MicroToUSD(totalMicro)
		list = append(list, r)
	}
	return list, rows.Err()
}

type ResourceStatisticsRow struct {
	ClientID   string  `json:"clientId"`
	ClientName string  `json:"clientName"`
	Total      float64 `json:"total"`
}

type GetResourceStatisticsParams struct {
	TimeFrom *time.Time
	TimeTo   *time.Time
	TenantID *string
}

func (r *SystemRepository) GetResourceStatistics(ctx context.Context, params GetResourceStatisticsParams) ([]ResourceStatisticsRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(dt.request_source, '') AS client_id,
		       COALESCE(NULLIF(dt.request_source, ''), 'D-AI') AS client_name,
		       SUM(COALESCE(dt.tenant_payable, 0) + COALESCE(dt.user_charged, 0)) AS total
		FROM ai_usage_logs dt
		WHERE dt.billing_status = 'settled'
		  AND ($1::timestamptz IS NULL OR dt.created_at >= $1::timestamptz)
		  AND ($2::timestamptz IS NULL OR dt.created_at < $2::timestamptz)
		  AND ($3::text IS NULL OR dt.tenant_id = $3::text)
		GROUP BY dt.request_source ORDER BY total DESC
	`, params.TimeFrom, params.TimeTo, params.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ResourceStatisticsRow
	for rows.Next() {
		var row ResourceStatisticsRow
		var totalMicro int64
		if err := rows.Scan(&row.ClientID, &row.ClientName, &totalMicro); err != nil {
			return nil, err
		}
		row.Total = billing.MicroToUSD(totalMicro)
		list = append(list, row)
	}
	return list, rows.Err()
}
