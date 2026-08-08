package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/domain"
)

type AccountRepository struct {
	pool *pgxpool.Pool
}

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

// BalanceResponse 账户余额响应
type BalanceResponse struct {
	Currency                string              `json:"currency"`
	TotalUSD                float64             `json:"totalUsd"`
	UsedUSD                 float64             `json:"usedUsd"`
	RemainingUSD            float64             `json:"remainingUsd"`
	AvailableUSD            float64             `json:"availableUsd"`
	PermanentUSD            float64             `json:"permanentUsd"`
	TimedUSD                float64             `json:"timedUsd"`
	OutstandingDebtMicroUSD int64               `json:"outstandingDebtMicroUsd"`
	ServiceState            string              `json:"serviceState"`
	BalanceLots             []AccountBalanceLot `json:"balanceLots,omitempty"`
}

type AccountBalanceLot struct {
	BalanceLotID string     `json:"balanceLotId"`
	TotalUSD     float64    `json:"totalUsd"`
	RemainingUSD float64    `json:"remainingUsd"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	Source       string     `json:"source"`
}

func (r *AccountRepository) listBalanceLots(ctx context.Context, packageType string, tenantID, userID *string, now time.Time) ([]AccountBalanceLot, error) {
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

	var list []AccountBalanceLot
	for rows.Next() {
		var lot AccountBalanceLot
		var totalMicro, remainingMicro int64
		if err := rows.Scan(&lot.BalanceLotID, &totalMicro, &remainingMicro, &lot.ExpiresAt, &lot.Source); err != nil {
			return nil, err
		}
		lot.TotalUSD = billing.MicroToUSD(totalMicro)
		lot.RemainingUSD = billing.MicroToUSD(remainingMicro)
		list = append(list, lot)
	}
	return list, rows.Err()
}

func (r *AccountRepository) GetTenantBalance(tenantID string, detail bool) (*BalanceResponse, error) {
	ctx := context.Background()
	now := billing.NowUTC()

	var debt int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(current_overdraft, 0)
		FROM iam_tenants WHERE tenant_id = $1
	`, tenantID).Scan(&debt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("查询租户账户失败: %w", err)
	}

	var remainingMicroUSD int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'tenant' AND tenant_id = $1 AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
	`, tenantID, now).Scan(&remainingMicroUSD); err != nil {
		return nil, fmt.Errorf("查询租户剩余余额失败: %w", err)
	}

	var totalMicroUSD int64
	r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'tenant' AND tenant_id = $1 AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
	`, tenantID, now).Scan(&totalMicroUSD)

	var permanentMicroUSD int64
	r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'tenant' AND tenant_id = $1 AND status = 'available' AND expires_at IS NULL
	`, tenantID).Scan(&permanentMicroUSD)

	resp := &BalanceResponse{
		Currency:                "USD",
		TotalUSD:                billing.MicroToUSD(totalMicroUSD),
		UsedUSD:                 billing.MicroToUSD(totalMicroUSD - remainingMicroUSD),
		RemainingUSD:            billing.MicroToUSD(remainingMicroUSD),
		AvailableUSD:            billing.MicroToUSD(remainingMicroUSD),
		PermanentUSD:            billing.MicroToUSD(permanentMicroUSD),
		TimedUSD:                billing.MicroToUSD(remainingMicroUSD - permanentMicroUSD),
		OutstandingDebtMicroUSD: debt,
		ServiceState:            accountServiceState(debt),
	}

	if detail {
		lots, err := r.listBalanceLots(ctx, billing.PackageTypeTenant, &tenantID, nil, now)
		if err == nil {
			resp.BalanceLots = lots
		}
	}

	return resp, nil
}

func (r *AccountRepository) GetUserBalance(userID string, detail bool) (*BalanceResponse, error) {
	ctx := context.Background()
	now := billing.NowUTC()

	var debt int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(current_overdraft, 0)
		FROM iam_accounts WHERE user_id = $1 AND user_type = 4
	`, userID).Scan(&debt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("查询用户账户失败: %w", err)
	}

	var remainingMicroUSD int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'user' AND user_id = $1 AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
	`, userID, now).Scan(&remainingMicroUSD); err != nil {
		return nil, fmt.Errorf("查询用户剩余余额失败: %w", err)
	}

	var totalMicroUSD int64
	r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'user' AND user_id = $1 AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
	`, userID, now).Scan(&totalMicroUSD)

	var permanentMicroUSD int64
	r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'user' AND user_id = $1 AND status = 'available' AND expires_at IS NULL
	`, userID).Scan(&permanentMicroUSD)

	resp := &BalanceResponse{
		Currency:                "USD",
		TotalUSD:                billing.MicroToUSD(totalMicroUSD),
		UsedUSD:                 billing.MicroToUSD(totalMicroUSD - remainingMicroUSD),
		RemainingUSD:            billing.MicroToUSD(remainingMicroUSD),
		AvailableUSD:            billing.MicroToUSD(remainingMicroUSD),
		PermanentUSD:            billing.MicroToUSD(permanentMicroUSD),
		TimedUSD:                billing.MicroToUSD(remainingMicroUSD - permanentMicroUSD),
		OutstandingDebtMicroUSD: debt,
		ServiceState:            accountServiceState(debt),
	}

	if detail {
		lots, err := r.listBalanceLots(ctx, billing.PackageTypeUser, nil, &userID, now)
		if err == nil {
			resp.BalanceLots = lots
		}
	}

	return resp, nil
}

func accountServiceState(debt int64) string {
	if debt > 0 {
		return "blocked_debt"
	}
	return "active"
}

// EventRow 消费流水行
type EventRow struct {
	EventID         string  `json:"eventId"`
	UserID          string  `json:"userId"`
	Description     string  `json:"description"`
	TenantAmountUSD float64 `json:"tenantAmountUsd"`
	UserAmountUSD   float64 `json:"userAmountUsd"`
	Status          string  `json:"status"`
	TerminalNote    string  `json:"terminalNote"`
	Metadata        string  `json:"metadata"`
	CreatedTime     *int64  `json:"createdTime"`
	FinishedTime    *int64  `json:"finishedTime,omitempty"`
	Username        string  `json:"username"`
	TenantName      string  `json:"tenantName"`
	ClientID        string  `json:"clientId"`
	AppName         string  `json:"appName"`
}

// ListTransactionsParams 消费流水查询参数
type ListTransactionsParams struct {
	TenantID   string
	UserID     string
	TenantName string
	Username   string
	ClientName string
	Status     string     // 空=不过滤
	TimeFrom   *time.Time // 按 created_at 过滤
	TimeTo     *time.Time
	Page       int
	Size       int
}

func (r *AccountRepository) ListTransactions(p ListTransactionsParams) ([]EventRow, int64, error) {
	ctx := context.Background()

	base := `
		FROM bill_events dt
		LEFT JOIN iam_tenants t ON t.tenant_id = dt.tenant_id
		LEFT JOIN iam_accounts eu ON eu.user_id = dt.user_id AND eu.user_type = 4
		WHERE dt.event_type = 'charge'`

	var args []any
	argIdx := 1

	if p.TenantID != "" {
		base += fmt.Sprintf(" AND dt.tenant_id = $%d", argIdx)
		args = append(args, p.TenantID)
		argIdx++
	}
	if p.UserID != "" {
		base += fmt.Sprintf(" AND dt.user_id = $%d", argIdx)
		args = append(args, p.UserID)
		argIdx++
	}
	if p.TenantName != "" {
		base += fmt.Sprintf(" AND t.tenant_name LIKE $%d", argIdx)
		args = append(args, "%"+p.TenantName+"%")
		argIdx++
	}
	if p.Username != "" {
		base += fmt.Sprintf(" AND eu.username LIKE $%d", argIdx)
		args = append(args, "%"+p.Username+"%")
		argIdx++
	}
	if p.ClientName != "" {
		base += fmt.Sprintf(" AND dt.client_id ILIKE $%d", argIdx)
		args = append(args, "%"+p.ClientName+"%")
		argIdx++
	}
	if p.Status != "" {
		base += fmt.Sprintf(" AND dt.status = $%d", argIdx)
		args = append(args, p.Status)
		argIdx++
	}
	if p.TimeFrom != nil {
		base += fmt.Sprintf(" AND dt.created_at >= $%d", argIdx)
		args = append(args, *p.TimeFrom)
		argIdx++
	}
	if p.TimeTo != nil {
		base += fmt.Sprintf(" AND dt.created_at < $%d", argIdx)
		args = append(args, *p.TimeTo)
		argIdx++
	}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计消费流水失败: %w", err)
	}

	selectSQL := fmt.Sprintf(
		`SELECT dt.event_id, COALESCE(dt.user_id,''), COALESCE(dt.description,''),
		        COALESCE(dt.tenant_credits,0), COALESCE(dt.user_credits,0),
		        dt.status, COALESCE(dt.terminal_note,''), COALESCE(dt.metadata::text,'{}'),
		        dt.created_at, dt.finished_at,
		        COALESCE(eu.username,''), COALESCE(t.tenant_name,''), COALESCE(dt.client_id,''),
		        COALESCE(NULLIF(dt.client_id,''),'D-AI') `+
			base+` ORDER BY dt.created_at DESC LIMIT $%d OFFSET $%d`,
		argIdx, argIdx+1,
	)

	queryArgs := append(args, int32(p.Size), int32((p.Page-1)*p.Size))
	rows, err := r.pool.Query(ctx, selectSQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询消费流水失败: %w", err)
	}
	defer rows.Close()

	var list []EventRow
	for rows.Next() {
		var row EventRow
		var tenantMicro, userMicro int64
		var createdAt time.Time
		var finishedAt *time.Time
		if err := rows.Scan(
			&row.EventID, &row.UserID, &row.Description,
			&tenantMicro, &userMicro,
			&row.Status, &row.TerminalNote, &row.Metadata,
			&createdAt, &finishedAt,
			&row.Username, &row.TenantName, &row.ClientID, &row.AppName,
		); err != nil {
			return nil, 0, fmt.Errorf("扫描消费流水失败: %w", err)
		}
		row.CreatedTime = unixMilliPtr(createdAt)
		row.TenantAmountUSD = billing.MicroToUSD(tenantMicro)
		row.UserAmountUSD = billing.MicroToUSD(userMicro)
		if finishedAt != nil {
			row.FinishedTime = unixMilliPtr(*finishedAt)
		}
		list = append(list, row)
	}

	return list, total, nil
}

// RechargeRecordRow 充值记录行
type RechargeRecordRow struct {
	OrderID         string  `json:"orderId"`
	OrderType       string  `json:"orderType"`
	PaidAmountMinor int64   `json:"paidAmountMinor"`
	AmountUSD       float64 `json:"amountUsd"`
	Status          string  `json:"status"`
	Note            string  `json:"note"`
	UserID          string  `json:"userId"`
	Username        string  `json:"username"`
	TenantName      string  `json:"tenantName"`
	CreatedTime     *int64  `json:"createdTime"`
}

func (r *AccountRepository) ListRechargeRecords(
	tenantID string,
	userID string,
	tenantName string,
	username string,
	orderTypes []string,
	timeFrom *time.Time,
	timeTo *time.Time,
	page int,
	size int,
) ([]RechargeRecordRow, int64, error) {
	ctx := context.Background()

	base := `
		FROM bill_recharge_orders r
		LEFT JOIN iam_accounts eu ON eu.user_id = r.user_id AND eu.user_type = 4
		LEFT JOIN iam_tenants t ON t.tenant_id = r.tenant_id
		WHERE 1=1`

	var args []any
	argIdx := 1

	if tenantID != "" {
		base += fmt.Sprintf(" AND r.tenant_id = $%d", argIdx)
		args = append(args, tenantID)
		argIdx++
	}
	if userID != "" {
		base += fmt.Sprintf(" AND r.user_id = $%d", argIdx)
		args = append(args, userID)
		argIdx++
	}
	if len(orderTypes) > 0 {
		base += fmt.Sprintf(" AND r.order_type = ANY($%d)", argIdx)
		args = append(args, orderTypes)
		argIdx++
	}
	if tenantName != "" {
		base += fmt.Sprintf(" AND t.tenant_name LIKE $%d", argIdx)
		args = append(args, "%"+tenantName+"%")
		argIdx++
	}
	if username != "" {
		base += fmt.Sprintf(" AND eu.username LIKE $%d", argIdx)
		args = append(args, "%"+username+"%")
		argIdx++
	}
	if timeFrom != nil {
		base += fmt.Sprintf(" AND r.created_at >= $%d", argIdx)
		args = append(args, *timeFrom)
		argIdx++
	}
	if timeTo != nil {
		base += fmt.Sprintf(" AND r.created_at < $%d", argIdx)
		args = append(args, *timeTo)
		argIdx++
	}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计充值记录失败: %w", err)
	}

	selectSQL := fmt.Sprintf(
		`SELECT r.order_id, r.order_type, r.paid_amount, r.credit_amount, r.status, COALESCE(r.note,''), COALESCE(r.user_id,''), COALESCE(eu.username,''), COALESCE(t.tenant_name,''), r.created_at `+
			base+` ORDER BY r.created_at DESC LIMIT $%d OFFSET $%d`,
		argIdx, argIdx+1,
	)

	queryArgs := append(args, int32(size), int32((page-1)*size))
	rows, err := r.pool.Query(ctx, selectSQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询充值记录失败: %w", err)
	}
	defer rows.Close()

	var list []RechargeRecordRow
	for rows.Next() {
		var row RechargeRecordRow
		var creditMicro int64
		var createdAt time.Time
		if err := rows.Scan(
			&row.OrderID, &row.OrderType, &row.PaidAmountMinor, &creditMicro, &row.Status,
			&row.Note, &row.UserID, &row.Username, &row.TenantName, &createdAt,
		); err != nil {
			return nil, 0, fmt.Errorf("扫描充值记录失败: %w", err)
		}
		row.AmountUSD = billing.MicroToUSD(creditMicro)
		row.CreatedTime = unixMilliPtr(createdAt)
		list = append(list, row)
	}

	return list, total, nil
}

// AccountStatsResult 账户统计结果
type AccountStatsResult struct {
	EndUserCount     int64   `json:"endUserCount"`
	InviteCodeCount  int64   `json:"inviteCodeCount"`
	UserDeductionUSD float64 `json:"userDeductionUsd"`
}

func (r *AccountRepository) GetAccountStats(tenantID string) (*AccountStatsResult, error) {
	ctx := context.Background()
	var result AccountStatsResult
	var userDeductionMicro int64
	err := r.pool.QueryRow(ctx, `
			SELECT
			  (SELECT COUNT(*) FROM iam_accounts WHERE tenant_id = $1 AND user_type = 4)::bigint,
			  (SELECT COUNT(*) FROM iam_invitation_codes WHERE tenant_id = $1)::bigint,
			  COALESCE((SELECT SUM(user_credits) FROM bill_events WHERE tenant_id = $1 AND status = 'succeeded' AND event_type = 'charge'), 0)::bigint
		`, tenantID).Scan(&result.EndUserCount, &result.InviteCodeCount, &userDeductionMicro)
	if err != nil {
		return nil, fmt.Errorf("查询账户统计失败: %w", err)
	}
	result.UserDeductionUSD = billing.MicroToUSD(userDeductionMicro)
	return &result, nil
}

func unixMilliPtr(t time.Time) *int64 {
	value := t.UnixMilli()
	return &value
}
