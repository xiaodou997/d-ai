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
	_, err := pool.Exec(ctx, `
		INSERT INTO pay_orders
		(order_id, out_trade_no, scene, tenant_id, user_id, topup_mode, package_id, package_name, package_badge,
		 amount, exchange_rate, gross_credit_amount, fee_rate_bp, fee_credit_amount, credit_amount,
		 fee_amount, net_amount, channel, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7,''), NULLIF($8,''), NULLIF($9,''),
		 $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, now(), now())
	`, o.OrderID, o.OutTradeNo, o.Scene, o.TenantID, userIDVal,
		o.TopupMode, o.PackageID, o.PackageName, o.PackageBadge, o.Amount, o.ExchangeRate, o.GrossCreditAmount,
		o.FeeRateBp, o.FeeCreditAmount, o.CreditAmount, o.FeeAmount, o.NetAmount, o.Channel, o.Status, o.ExpiresAt)
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
		UPDATE pay_orders SET status = $1, fail_note = $2, updated_at = now() WHERE order_id = $3
	`, payment.OrderStatusClosed, note, orderID)
	return err
}

func scanOrder(row pgx.Row) (*payment.Order, error) {
	var o payment.Order
	var userID, packageID, packageName, packageBadge, codeURL, transactionID, creditOrderID, failNote *string
	var paidAt *time.Time
	err := row.Scan(
		&o.ID, &o.OrderID, &o.OutTradeNo, &o.Scene, &o.TenantID, &userID,
		&o.TopupMode, &packageID, &packageName, &packageBadge,
		&o.Amount, &o.ExchangeRate, &o.GrossCreditAmount, &o.FeeRateBp, &o.FeeCreditAmount, &o.CreditAmount, &o.FeeAmount, &o.NetAmount,
		&o.Channel, &codeURL, &transactionID, &o.Status, &paidAt, &o.ExpiresAt,
		&creditOrderID, &failNote, &o.CreatedAt, &o.UpdatedAt,
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
	if creditOrderID != nil {
		o.CreditOrderID = *creditOrderID
	}
	if failNote != nil {
		o.FailNote = *failNote
	}
	o.PaidAt = paidAt
	return &o, nil
}

const orderColumns = `
	id, order_id, out_trade_no, scene, tenant_id, user_id,
	topup_mode, package_id, package_name, package_badge,
	amount, exchange_rate, gross_credit_amount, fee_rate_bp, fee_credit_amount, credit_amount, fee_amount, net_amount,
	channel, code_url, transaction_id, status, paid_at, expires_at,
	credit_order_id, fail_note, created_at, updated_at`

// GetOrderByID 按 order_id 查询（不加锁，供只读端点使用）。
func GetOrderByID(ctx context.Context, pool *pgxpool.Pool, orderID string) (*payment.Order, error) {
	row := pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM pay_orders WHERE order_id = $1`, orderID)
	return scanOrder(row)
}

// GetOrderByOutTradeNoForUpdate 事务内按 out_trade_no 查询并加行锁（回调核销/sweep 用）。
func GetOrderByOutTradeNoForUpdate(ctx context.Context, tx pgx.Tx, outTradeNo string) (*payment.Order, error) {
	row := tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM pay_orders WHERE out_trade_no = $1 FOR UPDATE`, outTradeNo)
	return scanOrder(row)
}

// MarkPaidTx 事务内把订单标记为 paid 并回填微信交易号/入账后的充值订单号。
func MarkPaidTx(ctx context.Context, tx pgx.Tx, orderID, transactionID, creditOrderID string, notifyRaw []byte) error {
	_, err := tx.Exec(ctx, `
		UPDATE pay_orders SET status = $1, transaction_id = $2, credit_order_id = $3,
		       paid_at = now(), notify_raw = $4, updated_at = now()
		WHERE order_id = $5
	`, payment.OrderStatusPaid, transactionID, creditOrderID, notifyRaw, orderID)
	return err
}

// UpdateStatusTx 事务内更新订单状态（paying/closed/expired），可选写入失败备注。
func UpdateStatusTx(ctx context.Context, tx pgx.Tx, orderID, status, failNote string, notifyRaw []byte) error {
	_, err := tx.Exec(ctx, `
		UPDATE pay_orders SET status = $1, fail_note = NULLIF($2,''), notify_raw = COALESCE($3, notify_raw), updated_at = now()
		WHERE order_id = $4
	`, status, failNote, notifyRaw, orderID)
	return err
}

// UpdateStatus 非事务版本，供 sweep 任务标记 closed/expired（不涉及记账，无需事务）。
func UpdateStatus(ctx context.Context, pool *pgxpool.Pool, orderID, status, failNote string) error {
	_, err := pool.Exec(ctx, `
		UPDATE pay_orders SET status = $1, fail_note = NULLIF($2,''), updated_at = now() WHERE order_id = $3
	`, status, failNote, orderID)
	return err
}

// ListSweepCandidates 返回需要 sweep 处理的订单：created/paying/expired 且已过期。纳入 expired
// 是为了让"Close 失败→标记 expired"之后仍能被下一轮 sweep 重新 Query+Close（设计文档 §4.5
// "失败置 expired，下轮重试关单"）——若不重新纳入候选集，Close 失败会让订单永久停在 expired，
// 即便用户其实已经支付成功（Query 瞬时错误导致误判为未支付）也再无重试机会。
func ListSweepCandidates(ctx context.Context, pool *pgxpool.Pool, now time.Time, limit int) ([]*payment.Order, error) {
	rows, err := pool.Query(ctx, `
		SELECT `+orderColumns+` FROM pay_orders
		WHERE status IN ($1, $2, $3) AND expires_at < $4
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
	defer rows.Close()
	var list []*payment.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, o)
	}
	return list, total, rows.Err()
}
