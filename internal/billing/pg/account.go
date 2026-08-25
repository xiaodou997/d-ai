package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/billing/ledger"
	billingports "xiaodou/dai/internal/billing/ports"
	"xiaodou/dai/internal/domain"
)

type AccountRepository struct {
	pool *pgxpool.Pool
}

var _ billingports.AccountQueryReader = (*AccountRepository)(nil)

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

// BalanceResponse and AccountBalanceLot remain adapter-level aliases for
// compatibility. New callers should use billing/ports projections.
type BalanceResponse = billingports.BalanceResponse
type AccountBalanceLot = billingports.AccountBalanceLot

func (r *AccountRepository) listBalanceLots(ctx context.Context, accountID string) ([]AccountBalanceLot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT lot_id, granted_micro, granted_micro - consumed_micro, created_at, expires_at, source
		FROM bill_credit_lots
		WHERE account_id = $1
		  AND expired_at IS NULL
		  AND revoked_at IS NULL
		  AND consumed_micro < granted_micro
		ORDER BY expires_at ASC NULLS LAST, created_at ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AccountBalanceLot
	for rows.Next() {
		var lot AccountBalanceLot
		var grantedMicro, remainingMicro int64
		if err := rows.Scan(&lot.BalanceLotID, &grantedMicro, &remainingMicro, &lot.CreatedAt, &lot.ExpiresAt, &lot.Source); err != nil {
			return nil, err
		}
		lot.TotalUSD = billing.MicroToUSD(grantedMicro)
		lot.RemainingUSD = billing.MicroToUSD(remainingMicro)
		list = append(list, lot)
	}
	return list, rows.Err()
}

func (r *AccountRepository) GetTenantBalance(ctx context.Context, tenantID string, detail bool) (*BalanceResponse, error) {
	return r.getBalance(ctx, ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, detail)
}

func (r *AccountRepository) GetUserBalance(ctx context.Context, userID string, detail bool) (*BalanceResponse, error) {
	return r.getBalance(ctx, ledger.Ref{Kind: ledger.KindUser, ID: userID}, detail)
}

// getBalance projects the account's signed balance onto the response shape the
// portal already speaks.
//
// The response still reports availableUsd and outstandingDebtMicroUsd as two
// non-negative numbers, but they are now two views of one value rather than two
// stored columns that could disagree: exactly one of them is non-zero. Lot
// totals are attribution only — the balance is what the admission gate reads,
// and it is read here through the same function the gate uses.
func (r *AccountRepository) getBalance(ctx context.Context, ref ledger.Ref, detail bool) (*BalanceResponse, error) {
	balance, err := ledger.Balance(ctx, r.pool, ref)
	if err != nil {
		if errors.Is(err, ledger.ErrAccountNotFound) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("查询账户余额失败: %w", err)
	}

	var grantedMicroUSD, remainingMicroUSD, permanentMicroUSD int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(granted_micro), 0),
		       COALESCE(SUM(granted_micro - consumed_micro), 0),
		       COALESCE(SUM(granted_micro - consumed_micro) FILTER (WHERE expires_at IS NULL), 0)
		FROM bill_credit_lots
		WHERE account_id = $1 AND expired_at IS NULL AND revoked_at IS NULL
	`, ref.ID).Scan(&grantedMicroUSD, &remainingMicroUSD, &permanentMicroUSD); err != nil {
		return nil, fmt.Errorf("查询额度批次失败: %w", err)
	}

	available := max(balance, 0)
	debt := max(-balance, 0)

	resp := &BalanceResponse{
		Currency:                "USD",
		TotalUSD:                billing.MicroToUSD(grantedMicroUSD),
		UsedUSD:                 billing.MicroToUSD(grantedMicroUSD - remainingMicroUSD),
		RemainingUSD:            billing.MicroToUSD(available),
		AvailableUSD:            billing.MicroToUSD(available),
		PermanentUSD:            billing.MicroToUSD(permanentMicroUSD),
		TimedUSD:                billing.MicroToUSD(remainingMicroUSD - permanentMicroUSD),
		OutstandingDebtMicroUSD: debt,
		ServiceState:            AccountServiceState(balance),
	}

	if detail {
		lots, err := r.listBalanceLots(ctx, ref.ID)
		if err == nil {
			resp.BalanceLots = lots
		}
	}

	return resp, nil
}

// AccountServiceState maps a signed balance onto the two states the portal
// renders. It is the only place that translation happens.
func AccountServiceState(balanceMicro int64) string {
	if balanceMicro > 0 {
		return "active"
	}
	return "blocked_debt"
}

// RechargeRecordRow and AccountStatsResult remain adapter-level aliases for
// compatibility. New callers should use billing/ports projections.
type RechargeRecordRow = billingports.RechargeRecordRow

func (r *AccountRepository) ListRechargeRecords(ctx context.Context, query billingports.RechargeRecordsQuery) ([]RechargeRecordRow, int64, error) {

	base := `
		FROM bill_recharge_orders r
		LEFT JOIN iam_accounts eu ON eu.user_id = r.user_id AND eu.user_type = 4
		LEFT JOIN iam_tenants t ON t.tenant_id = r.tenant_id
		WHERE 1=1`

	var args []any
	argIdx := 1

	if query.TenantID != "" {
		base += fmt.Sprintf(" AND r.tenant_id = $%d", argIdx)
		args = append(args, query.TenantID)
		argIdx++
	}
	if query.UserID != "" {
		base += fmt.Sprintf(" AND r.user_id = $%d", argIdx)
		args = append(args, query.UserID)
		argIdx++
	}
	if len(query.OrderTypes) > 0 {
		base += fmt.Sprintf(" AND r.order_type = ANY($%d)", argIdx)
		args = append(args, query.OrderTypes)
		argIdx++
	}
	if query.TenantName != "" {
		base += fmt.Sprintf(" AND t.tenant_name LIKE $%d", argIdx)
		args = append(args, "%"+query.TenantName+"%")
		argIdx++
	}
	if query.Username != "" {
		base += fmt.Sprintf(" AND eu.username LIKE $%d", argIdx)
		args = append(args, "%"+query.Username+"%")
		argIdx++
	}
	if query.TimeFrom != nil {
		base += fmt.Sprintf(" AND r.created_at >= $%d", argIdx)
		args = append(args, *query.TimeFrom)
		argIdx++
	}
	if query.TimeTo != nil {
		base += fmt.Sprintf(" AND r.created_at < $%d", argIdx)
		args = append(args, *query.TimeTo)
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

	queryArgs := append(args, int32(query.Size), int32((query.Page-1)*query.Size))
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

// AccountStatsResult remains an adapter-level alias for compatibility.
type AccountStatsResult = billingports.AccountStatsResult

func (r *AccountRepository) GetAccountStats(ctx context.Context, tenantID string) (*AccountStatsResult, error) {
	var result AccountStatsResult
	var userDeductionMicro int64
	err := r.pool.QueryRow(ctx, `
			SELECT
			  (SELECT COUNT(*) FROM iam_accounts WHERE tenant_id = $1 AND user_type = 4)::bigint,
			  (SELECT COUNT(*) FROM iam_invitation_codes WHERE tenant_id = $1)::bigint,
			  COALESCE((SELECT SUM(user_charged) FROM ai_usage_logs WHERE tenant_id = $1 AND billing_status = 'settled'), 0)::bigint
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
