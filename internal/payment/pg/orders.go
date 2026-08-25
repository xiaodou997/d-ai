// Package pg 是 payment 域的手写 SQL 仓储层（无 ORM，模式同 internal/billing/pg）。
package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/payment"
)

// InsertOrder 写入新建的支付订单（created 状态）。
func InsertOrder(ctx context.Context, pool *pgxpool.Pool, o *payment.Order) error {
	var userIDVal any
	if o.UserID != "" {
		userIDVal = o.UserID
	}
	fulfillmentStatus := o.FulfillmentStatus
	if fulfillmentStatus == "" {
		fulfillmentStatus = payment.FulfillmentStatusPending
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO pay_orders
		(order_id, out_trade_no, scene, tenant_id, user_id, topup_mode, package_id, package_name, package_badge,
		 payment_currency, payment_amount_minor, ledger_currency, gross_amount_micro_usd, fee_rate_bp,
		 fee_amount_micro_usd, gift_amount_micro_usd, credited_amount_micro_usd, tenant_income_micro_usd,
		 balance_expires_at, channel, status, fulfillment_status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7,''), NULLIF($8,''), NULLIF($9,''),
		 $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, now(), now())
	`, o.OrderID, o.OutTradeNo, o.Scene, o.TenantID, userIDVal,
		o.TopupMode, o.PackageID, o.PackageName, o.PackageBadge,
		o.PaymentCurrency, o.PaymentAmountMinor, o.LedgerCurrency, o.GrossAmountMicroUSD,
		o.FeeRateBp, o.FeeAmountMicroUSD, o.GiftAmountMicroUSD, o.CreditedAmountMicroUSD,
		o.TenantIncomeMicroUSD, o.BalanceExpiresAt, o.Channel, o.Status, fulfillmentStatus, o.ExpiresAt)
	if err != nil {
		return fmt.Errorf("创建支付订单失败: %w", err)
	}
	return nil
}

// SetCodeURL 下单成功后回填微信返回的二维码链接。
func SetCodeURL(ctx context.Context, pool *pgxpool.Pool, orderID, codeURL string) error {
	_, err := pool.Exec(ctx, `UPDATE pay_orders SET code_url = $1, updated_at = now() WHERE order_id = $2`, codeURL, orderID)
	return err
}

// MarkOrderFailed 下单调用微信失败时把订单标记为 closed，附失败备注。
func MarkOrderFailed(ctx context.Context, pool *pgxpool.Pool, orderID, note string) error {
	_, err := pool.Exec(ctx, `
		UPDATE pay_orders SET status = $1, fail_note = $2,
		       sweep_attempts = 0, sweep_next_attempt_at = NULL,
		       sweep_last_attempt_at = NULL, sweep_last_error = NULL,
		       updated_at = now() WHERE order_id = $3
	`, payment.OrderStatusClosed, note, orderID)
	return err
}

func scanOrder(row pgx.Row) (*payment.Order, error) {
	var o payment.Order
	var userID, packageID, packageName, packageBadge, codeURL, transactionID, balanceOrderID, failNote, sweepLastError *string
	var paidAt, balanceExpiresAt, sweepNextAttemptAt, sweepLastAttemptAt *time.Time
	err := row.Scan(
		&o.ID, &o.OrderID, &o.OutTradeNo, &o.Scene, &o.TenantID, &userID,
		&o.TopupMode, &packageID, &packageName, &packageBadge,
		&o.PaymentCurrency, &o.PaymentAmountMinor, &o.LedgerCurrency, &o.GrossAmountMicroUSD,
		&o.FeeRateBp, &o.FeeAmountMicroUSD, &o.GiftAmountMicroUSD, &o.CreditedAmountMicroUSD,
		&o.TenantIncomeMicroUSD, &balanceExpiresAt,
		&o.Channel, &codeURL, &transactionID, &o.Status, &o.FulfillmentStatus, &o.RefundStatus, &paidAt, &o.ExpiresAt,
		&balanceOrderID, &failNote, &o.SweepAttempts, &sweepNextAttemptAt, &sweepLastAttemptAt, &sweepLastError,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if userID != nil {
		o.UserID = *userID
	}
	if packageID != nil {
		o.PackageID = *packageID
	}
	if packageName != nil {
		o.PackageName = *packageName
	}
	if packageBadge != nil {
		o.PackageBadge = *packageBadge
	}
	if codeURL != nil {
		o.CodeURL = *codeURL
	}
	if transactionID != nil {
		o.TransactionID = *transactionID
	}
	if balanceOrderID != nil {
		o.BalanceOrderID = *balanceOrderID
	}
	if failNote != nil {
		o.FailNote = *failNote
	}
	if sweepLastError != nil {
		o.SweepLastError = *sweepLastError
	}
	o.PaidAt = paidAt
	o.BalanceExpiresAt = balanceExpiresAt
	o.SweepNextAttemptAt = sweepNextAttemptAt
	o.SweepLastAttemptAt = sweepLastAttemptAt
	return &o, nil
}

const orderColumns = `
	id, order_id, out_trade_no, scene, tenant_id, user_id,
	topup_mode, package_id, package_name, package_badge,
	payment_currency, payment_amount_minor, ledger_currency, gross_amount_micro_usd, fee_rate_bp,
	fee_amount_micro_usd, gift_amount_micro_usd, credited_amount_micro_usd, tenant_income_micro_usd, balance_expires_at,
	channel, code_url, transaction_id, status, fulfillment_status, refund_status, paid_at, expires_at,
	balance_order_id, fail_note, sweep_attempts, sweep_next_attempt_at, sweep_last_attempt_at, sweep_last_error,
	created_at, updated_at`

// GetOrderByID 按 order_id 查询（不加锁，供只读端点使用）。
func GetOrderByID(ctx context.Context, pool *pgxpool.Pool, orderID string) (*payment.Order, error) {
	row := pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM pay_orders WHERE order_id = $1`, orderID)
	return scanOrder(row)
}

func GetOrderByIDForUpdate(ctx context.Context, tx pgx.Tx, orderID string) (*payment.Order, error) {
	row := tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM pay_orders WHERE order_id = $1 FOR UPDATE`, orderID)
	return scanOrder(row)
}

// GetOrderByOutTradeNoForUpdate 事务内按 out_trade_no 查询并加行锁（回调核销/sweep 用）。
func GetOrderByOutTradeNoForUpdate(ctx context.Context, tx pgx.Tx, outTradeNo string) (*payment.Order, error) {
	row := tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM pay_orders WHERE out_trade_no = $1 FOR UPDATE`, outTradeNo)
	return scanOrder(row)
}

// MarkPaidTx atomically marks the payment as paid and links the balance grant.
func MarkPaidTx(ctx context.Context, tx pgx.Tx, orderID, transactionID, balanceOrderID string, notifyRaw []byte) error {
	_, err := tx.Exec(ctx, `
		UPDATE pay_orders SET status = $1, fulfillment_status = $2, transaction_id = $3, balance_order_id = $4,
		       paid_at = now(), notify_raw = $5,
		       sweep_attempts = 0, sweep_next_attempt_at = NULL,
		       sweep_last_attempt_at = NULL, sweep_last_error = NULL,
		       updated_at = now()
		WHERE order_id = $6
	`, payment.OrderStatusPaid, payment.FulfillmentStatusCredited, transactionID, balanceOrderID, notifyRaw, orderID)
	return err
}

// UpdateStatusTx 事务内更新订单状态（paying/closed/expired），可选写入失败备注。
func UpdateStatusTx(ctx context.Context, tx pgx.Tx, orderID, status, failNote string, notifyRaw []byte) error {
	_, err := tx.Exec(ctx, `
		UPDATE pay_orders SET status = $1, fail_note = NULLIF($2,''), notify_raw = COALESCE($3, notify_raw),
		       sweep_attempts = CASE WHEN $1 IN ('closed', 'paid') THEN 0 ELSE sweep_attempts END,
		       sweep_next_attempt_at = CASE WHEN $1 IN ('closed', 'paid') THEN NULL ELSE sweep_next_attempt_at END,
		       sweep_last_attempt_at = CASE WHEN $1 IN ('closed', 'paid') THEN NULL ELSE sweep_last_attempt_at END,
		       sweep_last_error = CASE WHEN $1 IN ('closed', 'paid') THEN NULL ELSE sweep_last_error END,
		       updated_at = now()
		WHERE order_id = $4
	`, status, failNote, notifyRaw, orderID)
	return err
}

// UpdateStatusIfCurrent performs the non-payment sweep transition only when
// the order still has the status observed before the provider call. This is an
// optimistic state-machine guard: a concurrent callback that already marked
// the order paid cannot be overwritten by a late close/expire result.
func UpdateStatusIfCurrent(ctx context.Context, pool *pgxpool.Pool, orderID, expectedStatus, status, failNote string) (bool, error) {
	result, err := pool.Exec(ctx, `
		UPDATE pay_orders
		SET status = $1, fail_note = NULLIF($2, ''),
		    sweep_attempts = CASE WHEN $1 IN ('closed', 'paid') THEN 0 ELSE sweep_attempts END,
		    sweep_next_attempt_at = CASE WHEN $1 IN ('closed', 'paid') THEN NULL ELSE sweep_next_attempt_at END,
		    sweep_last_attempt_at = CASE WHEN $1 IN ('closed', 'paid') THEN NULL ELSE sweep_last_attempt_at END,
		    sweep_last_error = CASE WHEN $1 IN ('closed', 'paid') THEN NULL ELSE sweep_last_error END,
		    updated_at = now()
		WHERE order_id = $3 AND status = $4
	`, status, failNote, orderID, expectedStatus)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

// RecordSweepFailureIfCurrent persists a provider/settlement failure and
// schedules the next attempt only when the order still has the status observed
// before the external call. The conditional transition prevents a late sweep
// result from overwriting a concurrent payment callback.
func RecordSweepFailureIfCurrent(ctx context.Context, pool *pgxpool.Pool, orderID, expectedStatus, status string, nextAttemptAt time.Time, lastError string) (bool, error) {
	result, err := pool.Exec(ctx, `
		UPDATE pay_orders
		SET status = $1,
		    sweep_attempts = sweep_attempts + 1,
		    sweep_next_attempt_at = $2,
		    sweep_last_attempt_at = now(),
		    sweep_last_error = NULLIF($3, ''),
		    fail_note = CASE WHEN $1 = 'expired' THEN NULLIF($3, '') ELSE fail_note END,
		    updated_at = now()
		WHERE order_id = $4 AND status = $5
	`, status, nextAttemptAt, lastError, orderID, expectedStatus)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

// ScheduleSweepRetryIfCurrent throttles a non-terminal provider response
// (for example USERPAYING) without counting it as a failure.
func ScheduleSweepRetryIfCurrent(ctx context.Context, pool *pgxpool.Pool, orderID, expectedStatus string, nextAttemptAt time.Time) (bool, error) {
	result, err := pool.Exec(ctx, `
		UPDATE pay_orders
		SET sweep_next_attempt_at = $1,
		    sweep_last_attempt_at = now(),
		    sweep_last_error = NULL,
		    updated_at = now()
		WHERE order_id = $2 AND status = $3
	`, nextAttemptAt, orderID, expectedStatus)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

// DeleteStaleClosedOrders removes only closed payment shells that never paid
// and never produced a balance grant. The caller supplies the retention cutoff
// so the cleanup policy stays outside the repository layer.
func DeleteStaleClosedOrders(ctx context.Context, pool *pgxpool.Pool, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := pool.Query(ctx, `
		WITH stale AS (
			SELECT p.order_id
			FROM pay_orders p
			WHERE p.status = $1
			  AND p.fulfillment_status = $2
			  AND p.refund_status = $3
			  AND p.paid_at IS NULL
			  AND p.transaction_id IS NULL
			  AND p.balance_order_id IS NULL
			  AND p.updated_at < $4
			  AND NOT EXISTS (
				  SELECT 1 FROM bill_recharge_orders r
				  WHERE r.payment_order_id = p.order_id
			  )
			  AND NOT EXISTS (
				  SELECT 1 FROM pay_refunds f
				  WHERE f.payment_order_id = p.order_id
			  )
			ORDER BY p.updated_at ASC
			LIMIT $5
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM pay_orders p
		USING stale
		WHERE p.order_id = stale.order_id
		RETURNING p.order_id
	`, payment.OrderStatusClosed, payment.FulfillmentStatusPending, payment.RefundStatusNone, before, limit)
	if err != nil {
		return 0, fmt.Errorf("清理已关闭未支付订单失败: %w", err)
	}
	defer rows.Close()
	deleted := 0
	for rows.Next() {
		deleted++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("读取清理订单结果失败: %w", err)
	}
	return deleted, nil
}

// ListSweepCandidates 返回需要 sweep 处理的订单：created/paying/expired 且已过期。纳入 expired
// 是为了让"Close 失败→标记 expired"之后仍能被下一轮 sweep 重新 Query+Close（设计文档 §4.5
// "失败置 expired，下轮重试关单"）——若不重新纳入候选集，Close 失败会让订单永久停在 expired，
// 即便用户其实已经支付成功（Query 瞬时错误导致误判为未支付）也再无重试机会。
// sweep_next_attempt_at makes provider failures durable across scheduler
// cycles and process restarts instead of retrying every minute forever.
func ListSweepCandidates(ctx context.Context, pool *pgxpool.Pool, now time.Time, limit int) ([]*payment.Order, error) {
	rows, err := pool.Query(ctx, `
		SELECT `+orderColumns+` FROM pay_orders
		WHERE status IN ($1, $2, $3) AND expires_at < $4
		  AND (sweep_next_attempt_at IS NULL OR sweep_next_attempt_at <= $4)
		ORDER BY expires_at ASC LIMIT $5
	`, payment.OrderStatusCreated, payment.OrderStatusPaying, payment.OrderStatusExpired, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*payment.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

// ListInFlightCandidates 返回"在途补偿"候选：created/paying 状态、created_at 已过 5 分钟
// 但仍未到期。同时纳入 paying 是因为回调可能先把订单推进到 paying（USERPAYING）后丢失了
// 最终的 SUCCESS 通知——若只查 created，这类订单要等到整个 order TTL 到期才会被
// ListSweepCandidates 捡回补偿，入账延迟可达数小时。
func ListInFlightCandidates(ctx context.Context, pool *pgxpool.Pool, createdBefore, now time.Time, limit int) ([]*payment.Order, error) {
	rows, err := pool.Query(ctx, `
		SELECT `+orderColumns+` FROM pay_orders
		WHERE status IN ($1, $2) AND created_at < $3 AND expires_at > $4
		  AND (sweep_next_attempt_at IS NULL OR sweep_next_attempt_at <= $4)
		ORDER BY created_at ASC LIMIT $5
	`, payment.OrderStatusCreated, payment.OrderStatusPaying, createdBefore, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*payment.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

// ListOrdersParams 管理端/自助端订单列表筛选。
type ListOrdersParams struct {
	Scene    string
	Status   string
	TenantID string
	UserID   string
	Page     int
	Size     int
}

func ListOrders(ctx context.Context, pool *pgxpool.Pool, p ListOrdersParams) ([]*payment.Order, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	idx := 1
	add := func(cond string, val any) {
		where += fmt.Sprintf(" AND %s $%d", cond, idx)
		args = append(args, val)
		idx++
	}
	if p.Scene != "" {
		add("scene =", p.Scene)
	}
	if p.Status != "" {
		add("status =", p.Status)
	}
	if p.TenantID != "" {
		add("tenant_id =", p.TenantID)
	}
	if p.UserID != "" {
		add("user_id =", p.UserID)
	}

	var total int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM pay_orders "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	size := p.Size
	if size < 1 || size > 100 {
		size = 20
	}
	page := p.Page
	if page < 1 {
		page = 1
	}
	qargs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM pay_orders %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, orderColumns, where, idx, idx+1), qargs...)
	if err != nil {
		return nil, 0, err
	}
	var list []*payment.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		list = append(list, o)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	if err := loadOrderPartyNames(ctx, pool, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// loadOrderPartyNames enriches list projections without denormalizing mutable
// tenant and user names into the immutable payment snapshot.
func loadOrderPartyNames(ctx context.Context, pool *pgxpool.Pool, orders []*payment.Order) error {
	if len(orders) == 0 {
		return nil
	}
	byID := make(map[string]*payment.Order, len(orders))
	orderIDs := make([]string, 0, len(orders))
	for _, order := range orders {
		byID[order.OrderID] = order
		orderIDs = append(orderIDs, order.OrderID)
	}

	rows, err := pool.Query(ctx, `
		SELECT o.order_id, COALESCE(t.tenant_name, ''), COALESCE(u.username, '')
		FROM pay_orders o
		LEFT JOIN iam_tenants t ON t.tenant_id = o.tenant_id
		LEFT JOIN iam_accounts u ON u.user_id = o.user_id AND u.user_type = 4
		WHERE o.order_id = ANY($1)
	`, orderIDs)
	if err != nil {
		return fmt.Errorf("查询支付订单主体名称失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var orderID, tenantName, username string
		if err := rows.Scan(&orderID, &tenantName, &username); err != nil {
			return fmt.Errorf("扫描支付订单主体名称失败: %w", err)
		}
		if order := byID[orderID]; order != nil {
			order.TenantName = tenantName
			order.Username = username
		}
	}
	return rows.Err()
}
