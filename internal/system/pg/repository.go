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
	TenantRechargeAmount  int64   `json:"tenantRechargeAmount"`
	TenantRechargeCredits float64 `json:"tenantRechargeCredits"`
	ActiveTenants         int64   `json:"activeTenants"`
	TenantTotalCredits    float64 `json:"tenantTotalCredits"`
	UserRechargeAmount    int64   `json:"userRechargeAmount"`
	UserRechargeCredits   float64 `json:"userRechargeCredits"`
	NewUsers              int64   `json:"newUsers"`
	UserTotalCredits      float64 `json:"userTotalCredits"`
}

func (r *SystemRepository) GetGlobalStats(ctx context.Context, timeFrom, timeTo *time.Time) (GlobalStatsRow, error) {
	var row GlobalStatsRow
	var tenantRechargeMicro, tenantTotalMicro, userRechargeMicro, userTotalMicro int64
	err := r.pool.QueryRow(ctx, `
		SELECT
		  COALESCE((SELECT SUM(paid_amount) FROM bill_recharge_orders WHERE order_type IN ('platform_to_tenant', 'online_tenant_topup', 'cash_purchase') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
		  COALESCE((SELECT SUM(credit_amount) FROM bill_recharge_orders WHERE order_type IN ('platform_to_tenant', 'online_tenant_topup', 'cash_purchase') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
		  (SELECT COUNT(*) FROM iam_tenants WHERE status = 'active')::bigint,
		  COALESCE((SELECT SUM(remaining_credits) FROM bill_credit_packages WHERE package_type = 'tenant' AND status = 'available'), 0)::bigint,
		  COALESCE((SELECT SUM(paid_amount) FROM bill_recharge_orders WHERE order_type IN ('tenant_to_user', 'online_user_topup') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
		  COALESCE((SELECT SUM(credit_amount) FROM bill_recharge_orders WHERE order_type IN ('tenant_to_user', 'online_user_topup') AND status = 'active' AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)), 0)::bigint,
		  (SELECT COUNT(*) FROM iam_users WHERE ($1::timestamptz IS NULL OR created_at >= $1::timestamptz) AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz))::bigint,
		  COALESCE((SELECT SUM(remaining_credits) FROM bill_credit_packages WHERE package_type = 'user' AND status = 'available'), 0)::bigint
	`, timeFrom, timeTo).Scan(
		&row.TenantRechargeAmount, &tenantRechargeMicro,
		&row.ActiveTenants, &tenantTotalMicro,
		&row.UserRechargeAmount, &userRechargeMicro,
		&row.NewUsers, &userTotalMicro,
	)
	row.TenantRechargeCredits = billing.MicroToCredits(tenantRechargeMicro)
	row.TenantTotalCredits = billing.MicroToCredits(tenantTotalMicro)
	row.UserRechargeCredits = billing.MicroToCredits(userRechargeMicro)
	row.UserTotalCredits = billing.MicroToCredits(userTotalMicro)
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
		       SUM(COALESCE(tenant_credits, 0) + COALESCE(user_credits, 0)) AS total
		FROM bill_events
		WHERE status = 'succeeded'
		  AND event_type = 'charge'
		  AND ($1::timestamptz IS NULL OR created_at >= $1::timestamptz)
		  AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)
		  AND ($3::text IS NULL OR tenant_id = $3::text)
		  AND ($4::bigint IS NULL OR
		       ($4::bigint = 1 AND tenant_credits IS NOT NULL) OR
		       ($4::bigint = 2 AND user_credits IS NOT NULL))
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
		r.Total = billing.MicroToCredits(totalMicro)
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
		SELECT COALESCE(dt.client_id, '') AS client_id,
		       COALESCE(NULLIF(dt.client_id, ''), 'D-AI') AS client_name,
		       SUM(COALESCE(dt.tenant_credits, 0) + COALESCE(dt.user_credits, 0)) AS total
		FROM bill_events dt
		WHERE dt.status = 'succeeded'
		  AND dt.event_type = 'charge'
		  AND ($1::timestamptz IS NULL OR dt.created_at >= $1::timestamptz)
		  AND ($2::timestamptz IS NULL OR dt.created_at < $2::timestamptz)
		  AND ($3::text IS NULL OR dt.tenant_id = $3::text)
		GROUP BY dt.client_id ORDER BY total DESC
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
		row.Total = billing.MicroToCredits(totalMicro)
		list = append(list, row)
	}
	return list, rows.Err()
}
