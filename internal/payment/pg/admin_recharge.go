package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/payment"
)

type ListAdminRechargeOrdersParams struct {
	Keyword           string
	Method            string
	TargetType        string
	PaymentStatus     string
	FulfillmentStatus string
	RefundStatus      string
	TimeFrom          *time.Time
	TimeTo            *time.Time
	Page              int
	Size              int
}

const adminRechargeOrdersCTE = `
WITH recharge_orders AS (
    SELECT p.order_id,
           COALESCE(p.balance_order_id, '') AS balance_order_id,
           CASE p.scene WHEN 'user_topup' THEN 'online_user_topup' ELSE 'online_tenant_topup' END AS order_type,
           'online'::text AS method,
           CASE p.scene WHEN 'user_topup' THEN 'user' ELSE 'tenant' END AS target_type,
           p.tenant_id, COALESCE(t.tenant_name, '') AS tenant_name,
           COALESCE(p.user_id, '') AS user_id, COALESCE(u.username, '') AS username,
           p.payment_amount_minor, p.gross_amount_micro_usd, p.fee_amount_micro_usd,
           p.gift_amount_micro_usd, p.credited_amount_micro_usd, p.tenant_income_micro_usd,
	           p.status AS payment_status, p.fulfillment_status, p.refund_status,
           p.out_trade_no, COALESCE(p.transaction_id, '') AS transaction_id,
           p.topup_mode, COALESCE(p.package_name, '') AS package_name, p.channel,
           COALESCE(r.note, '') AS note, COALESCE(p.fail_note, '') AS fail_note,
           p.created_at, p.paid_at, p.expires_at AS payment_expires_at,
           p.balance_expires_at, r.reversed_at, COALESCE(r.reversed_by, '') AS reversed_by,
           COALESCE(r.reversal_reason, '') AS reversal_reason
    FROM pay_orders p
    LEFT JOIN bill_recharge_orders r ON r.order_id = p.balance_order_id
    LEFT JOIN iam_tenants t ON t.tenant_id = p.tenant_id
    LEFT JOIN iam_accounts u ON u.user_id = p.user_id AND u.user_type = 4

    UNION ALL

    SELECT r.order_id, r.order_id AS balance_order_id, r.order_type,
           'manual'::text AS method,
           CASE WHEN r.user_id IS NULL THEN 'tenant' ELSE 'user' END AS target_type,
           r.tenant_id, COALESCE(t.tenant_name, '') AS tenant_name,
           COALESCE(r.user_id, '') AS user_id, COALESCE(u.username, '') AS username,
           r.paid_amount AS payment_amount_minor, r.credit_amount AS gross_amount_micro_usd,
           0::bigint AS fee_amount_micro_usd, 0::bigint AS gift_amount_micro_usd,
           r.credit_amount AS credited_amount_micro_usd, 0::bigint AS tenant_income_micro_usd,
           'not_required'::text AS payment_status,
	           CASE
	             WHEN r.status = 'active' THEN 'credited'
	             WHEN r.lost_amount_micro > 0 THEN 'partially_reversed'
	             ELSE 'reversed'
	           END AS fulfillment_status,
	           'not_applicable'::text AS refund_status,
           ''::text AS out_trade_no, ''::text AS transaction_id,
           ''::text AS topup_mode, ''::text AS package_name, 'manual'::text AS channel,
           COALESCE(r.note, '') AS note, ''::text AS fail_note,
           r.created_at, NULL::timestamptz AS paid_at, NULL::timestamptz AS payment_expires_at,
           r.expires_at AS balance_expires_at, r.reversed_at, COALESCE(r.reversed_by, '') AS reversed_by,
           COALESCE(r.reversal_reason, '') AS reversal_reason
    FROM bill_recharge_orders r
    LEFT JOIN iam_tenants t ON t.tenant_id = r.tenant_id
    LEFT JOIN iam_accounts u ON u.user_id = r.user_id AND u.user_type = 4
    WHERE r.order_type IN ('platform_to_tenant', 'tenant_to_user')
)
`

const adminRechargeOrderColumns = `
order_id, balance_order_id, method, target_type, order_type,
tenant_id, tenant_name, user_id, username,
payment_amount_minor, gross_amount_micro_usd, fee_amount_micro_usd,
gift_amount_micro_usd, credited_amount_micro_usd, tenant_income_micro_usd,
payment_status, fulfillment_status, refund_status, out_trade_no, transaction_id,
topup_mode, package_name, channel, note, fail_note,
created_at, paid_at, payment_expires_at, balance_expires_at,
reversed_at, reversed_by, reversal_reason`

func ListAdminRechargeOrders(ctx context.Context, pool *pgxpool.Pool, p ListAdminRechargeOrdersParams) ([]payment.AdminRechargeOrder, int64, error) {
	where, args := adminRechargeFilters(p)
	var total int64
	if err := pool.QueryRow(ctx, adminRechargeOrdersCTE+`SELECT COUNT(*) FROM recharge_orders `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计充值订单失败: %w", err)
	}
	page, size := p.Page, p.Size
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	queryArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := pool.Query(ctx, fmt.Sprintf(
		adminRechargeOrdersCTE+`SELECT %s FROM recharge_orders %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		adminRechargeOrderColumns, where, len(args)+1, len(args)+2,
	), queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询充值订单失败: %w", err)
	}
	defer rows.Close()
	items := make([]payment.AdminRechargeOrder, 0)
	for rows.Next() {
		item, err := scanAdminRechargeOrder(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("扫描充值订单失败: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func GetAdminRechargeOrder(ctx context.Context, pool *pgxpool.Pool, orderID string) (*payment.AdminRechargeOrder, error) {
	row := pool.QueryRow(ctx, adminRechargeOrdersCTE+`SELECT `+adminRechargeOrderColumns+` FROM recharge_orders WHERE order_id = $1`, orderID)
	item, err := scanAdminRechargeOrder(row)
	if err != nil {
		return nil, err
	}
	credits, err := listRechargeCredits(ctx, pool, item.OrderID, item.BalanceOrderID, item.Method == "online")
	if err != nil {
		return nil, err
	}
	item.Credits = credits
	refund, err := getAdminRefund(ctx, pool, item.OrderID)
	if err != nil {
		return nil, err
	}
	item.Refund = refund
	return &item, nil
}

func adminRechargeFilters(p ListAdminRechargeOrdersParams) (string, []any) {
	where := "WHERE 1=1"
	args := make([]any, 0, 7)
	add := func(expression string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(" AND "+expression, len(args))
	}
	if p.Keyword != "" {
		add(`concat_ws(' ', order_id, balance_order_id, out_trade_no, transaction_id, tenant_name, username, tenant_id, user_id) ILIKE '%' || $%d || '%'`, p.Keyword)
	}
	if p.Method != "" {
		add("method = $%d", p.Method)
	}
	if p.TargetType != "" {
		add("target_type = $%d", p.TargetType)
	}
	if p.PaymentStatus != "" {
		add("payment_status = $%d", p.PaymentStatus)
	}
	if p.FulfillmentStatus != "" {
		add("fulfillment_status = $%d", p.FulfillmentStatus)
	}
	if p.RefundStatus != "" {
		add("refund_status = $%d", p.RefundStatus)
	}
	if p.TimeFrom != nil {
		add("created_at >= $%d", *p.TimeFrom)
	}
	if p.TimeTo != nil {
		add("created_at < $%d", *p.TimeTo)
	}
	return where, args
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAdminRechargeOrder(row rowScanner) (payment.AdminRechargeOrder, error) {
	var item payment.AdminRechargeOrder
	var createdAt time.Time
	var paidAt, paymentExpiresAt, balanceExpiresAt, reversedAt *time.Time
	err := row.Scan(
		&item.OrderID, &item.BalanceOrderID, &item.Method, &item.TargetType, &item.OrderType,
		&item.TenantID, &item.TenantName, &item.UserID, &item.Username,
		&item.PaidAmountMinor, &item.GrossAmountMicroUSD, &item.FeeAmountMicroUSD,
		&item.GiftAmountMicroUSD, &item.CreditedAmountMicroUSD, &item.TenantIncomeMicroUSD,
		&item.PaymentStatus, &item.FulfillmentStatus, &item.RefundStatus, &item.OutTradeNo, &item.TransactionID,
		&item.TopupMode, &item.PackageName, &item.Channel, &item.Note, &item.FailNote,
		&createdAt, &paidAt, &paymentExpiresAt, &balanceExpiresAt,
		&reversedAt, &item.ReversedBy, &item.ReversalReason,
	)
	if err != nil {
		return item, err
	}
	item.CreatedAt = createdAt.UnixMilli()
	item.PaidAt = timeMillis(paidAt)
	item.PaymentExpiresAt = timeMillis(paymentExpiresAt)
	item.BalanceExpiresAt = timeMillis(balanceExpiresAt)
	item.ReversedAt = timeMillis(reversedAt)
	return item, nil
}

func listRechargeCredits(ctx context.Context, pool *pgxpool.Pool, orderID, balanceOrderID string, online bool) ([]payment.RechargeCreditDetail, error) {
	query := `
		SELECT r.order_id, r.order_type, r.order_id = $2, r.credit_amount, r.status,
		       COALESCE(r.note, ''), r.expires_at, r.reversed_at,
		       COALESCE(r.reversed_by, ''), COALESCE(r.reversal_reason, ''),
		       r.reversed_amount_micro, r.lost_amount_micro,
		       COALESCE(l.lot_id, ''), COALESCE(l.granted_micro, 0), COALESCE(l.consumed_micro, 0),
		       COALESCE(GREATEST(l.granted_micro - l.consumed_micro, 0), 0),
		       CASE
		         WHEN l.lot_id IS NULL THEN 'no_lot'
		         WHEN l.revoked_at IS NOT NULL THEN 'revoked'
		         WHEN l.expired_at IS NOT NULL THEN 'expired'
		         WHEN l.consumed_micro >= l.granted_micro THEN 'depleted'
		         ELSE 'available'
		       END,
		       COALESCE(e.refund_id, ''), COALESCE(e.available_reclaimed_micro, 0),
		       COALESCE(e.non_available_debit_micro, 0), COALESCE(e.expired_amount_micro, 0),
		       COALESCE(e.account_debit_micro, 0), COALESCE(e.balance_after_micro, 0)
		FROM bill_recharge_orders r
		LEFT JOIN bill_credit_lots l ON l.recharge_order_id = r.order_id
		LEFT JOIN bill_refund_reversal_effects e ON e.recharge_order_id = r.order_id
		WHERE `
	args := []any{orderID, balanceOrderID}
	if online {
		query += `r.payment_order_id = $1`
	} else {
		query += `r.order_id = $1`
	}
	query += ` ORDER BY (r.order_id = $2) DESC, r.created_at, l.created_at`
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询关联额度失败: %w", err)
	}
	defer rows.Close()
	credits := make([]payment.RechargeCreditDetail, 0)
	for rows.Next() {
		var credit payment.RechargeCreditDetail
		var expiresAt, reversedAt *time.Time
		if err := rows.Scan(
			&credit.BalanceOrderID, &credit.OrderType, &credit.Primary, &credit.CreditAmountMicroUSD,
			&credit.Status, &credit.Note, &expiresAt, &reversedAt, &credit.ReversedBy,
			&credit.ReversalReason, &credit.ReversedAmountMicroUSD, &credit.LostAmountMicroUSD,
			&credit.LotID, &credit.GrantedAmountMicroUSD,
			&credit.ConsumedAmountMicroUSD, &credit.RemainingAmountMicroUSD, &credit.LotStatus,
			&credit.RefundID, &credit.RefundAvailableMicroUSD, &credit.RefundNonAvailableMicroUSD,
			&credit.RefundExpiredMicroUSD, &credit.RefundAccountDebitMicroUSD, &credit.RefundBalanceAfterMicroUSD,
		); err != nil {
			return nil, err
		}
		credit.BalanceExpiresAt = timeMillis(expiresAt)
		credit.ReversedAt = timeMillis(reversedAt)
		credits = append(credits, credit)
	}
	return credits, rows.Err()
}

func getAdminRefund(ctx context.Context, pool *pgxpool.Pool, paymentOrderID string) (*payment.AdminRefundRecord, error) {
	var refund payment.AdminRefundRecord
	var refundedAt, createdAt time.Time
	err := pool.QueryRow(ctx, `
		SELECT refund_id, refund_method, refund_reference, COALESCE(channel_refund_id, ''),
		       refund_amount_minor, status, refunded_at, reason, COALESCE(note, ''), operator_id, created_at
		FROM pay_refunds WHERE payment_order_id = $1
	`, paymentOrderID).Scan(
		&refund.RefundID, &refund.Method, &refund.RefundReference, &refund.ChannelRefundID,
		&refund.RefundAmountMinor, &refund.Status, &refundedAt, &refund.Reason, &refund.Note,
		&refund.OperatorID, &createdAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询退款记录失败: %w", err)
	}
	refund.RefundedAt = refundedAt.UnixMilli()
	refund.CreatedAt = createdAt.UnixMilli()
	return &refund, nil
}

func timeMillis(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	millis := value.UnixMilli()
	return &millis
}
