package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/payment"
)

// GetOrCreateCashAccountForUpdate 事务内取得（不存在则创建）租户现金账户并加行锁。
func GetOrCreateCashAccountForUpdate(ctx context.Context, tx pgx.Tx, tenantID string) (*payment.CashAccount, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO pay_cash_accounts (tenant_id) VALUES ($1) ON CONFLICT DO NOTHING`, tenantID); err != nil {
		return nil, fmt.Errorf("创建租户现金账户失败: %w", err)
	}
	var a payment.CashAccount
	a.TenantID = tenantID
	if err := tx.QueryRow(ctx, `
		SELECT balance, frozen, updated_at FROM pay_cash_accounts WHERE tenant_id = $1 FOR UPDATE
	`, tenantID).Scan(&a.Balance, &a.Frozen, &a.UpdatedAt); err != nil {
		return nil, fmt.Errorf("查询租户现金账户失败: %w", err)
	}
	return &a, nil
}

// GetCashAccount 只读查询（不存在时返回零值账户，不报错——尚未产生过在线收入的租户很正常）。
func GetCashAccount(ctx context.Context, pool *pgxpool.Pool, tenantID string) (*payment.CashAccount, error) {
	a := &payment.CashAccount{TenantID: tenantID}
	err := pool.QueryRow(ctx, `
		SELECT balance, frozen, updated_at FROM pay_cash_accounts WHERE tenant_id = $1
	`, tenantID).Scan(&a.Balance, &a.Frozen, &a.UpdatedAt)
	if err != nil {
		return a, nil //nolint:nilerr // 未开户视为余额为零
	}
	return a, nil
}

// AddCashBalanceTx 事务内增减余额（amount 可为负），返回变动后余额。
func AddCashBalanceTx(ctx context.Context, tx pgx.Tx, tenantID string, amount int64) (balanceAfter int64, err error) {
	err = tx.QueryRow(ctx, `
		UPDATE pay_cash_accounts SET balance = balance + $1, updated_at = now()
		WHERE tenant_id = $2 RETURNING balance
	`, amount, tenantID).Scan(&balanceAfter)
	if err != nil {
		return 0, fmt.Errorf("更新租户现金余额失败: %w", err)
	}
	return balanceAfter, nil
}

// AddCashFrozenTx 事务内增减冻结额（申请提现 +amount，驳回/取消 -amount）。
func AddCashFrozenTx(ctx context.Context, tx pgx.Tx, tenantID string, amount int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE pay_cash_accounts SET frozen = frozen + $1, updated_at = now() WHERE tenant_id = $2
	`, amount, tenantID)
	if err != nil {
		return fmt.Errorf("更新租户冻结余额失败: %w", err)
	}
	return nil
}

// InsertCashLedgerTx 写入现金流水（idempotencyKey 为空表示不需要幂等兜底，如手动购积分场景）。
func InsertCashLedgerTx(ctx context.Context, tx pgx.Tx, e *payment.CashLedgerEntry, idempotencyKey string) error {
	var idemVal any
	if idempotencyKey != "" {
		idemVal = idempotencyKey
	}
	var refTypeVal, refIDVal, operatorVal, noteVal any
	if e.RefType != "" {
		refTypeVal = e.RefType
	}
	if e.RefID != "" {
		refIDVal = e.RefID
	}
	if e.OperatorID != "" {
		operatorVal = e.OperatorID
	}
	if e.Note != "" {
		noteVal = e.Note
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO pay_cash_ledger
		(txn_id, tenant_id, txn_type, amount, balance_after, ref_type, ref_id, operator_id, note, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
	`, e.TxnID, e.TenantID, e.TxnType, e.Amount, e.BalanceAfter, refTypeVal, refIDVal, operatorVal, noteVal, idemVal)
	if err != nil {
		return fmt.Errorf("写入现金流水失败: %w", err)
	}
	return nil
}

// ExistsCashLedgerIdempotencyKey 幂等兜底：idempotency_key 已存在则说明本笔已入账过。
func ExistsCashLedgerIdempotencyKey(ctx context.Context, tx pgx.Tx, idempotencyKey string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pay_cash_ledger WHERE idempotency_key = $1)`, idempotencyKey).Scan(&exists)
	return exists, err
}

// ListCashLedger 现金流水分页查询。
func ListCashLedger(ctx context.Context, pool *pgxpool.Pool, tenantID string, txnType string, page, size int) ([]*payment.CashLedgerEntry, int64, error) {
	where := "WHERE tenant_id = $1"
	args := []any{tenantID}
	idx := 2
	if txnType != "" {
		where += fmt.Sprintf(" AND txn_type = $%d", idx)
		args = append(args, txnType)
		idx++
	}
	var total int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM pay_cash_ledger "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if size < 1 || size > 100 {
		size = 20
	}
	if page < 1 {
		page = 1
	}
	qargs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT id, txn_id, tenant_id, txn_type, amount, balance_after,
		       COALESCE(ref_type,''), COALESCE(ref_id,''), COALESCE(operator_id,''), COALESCE(note,''), created_at
		FROM pay_cash_ledger %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []*payment.CashLedgerEntry
	for rows.Next() {
		var e payment.CashLedgerEntry
		if err := rows.Scan(&e.ID, &e.TxnID, &e.TenantID, &e.TxnType, &e.Amount, &e.BalanceAfter,
			&e.RefType, &e.RefID, &e.OperatorID, &e.Note, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, &e)
	}
	return list, total, rows.Err()
}

// ListCashAccounts 管理端现金账户总览（联 iam_tenants 名称），分页。
type CashAccountRow struct {
	payment.CashAccount
	TenantName string
}

func ListCashAccounts(ctx context.Context, pool *pgxpool.Pool, page, size int) ([]*CashAccountRow, int64, error) {
	var total int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM pay_cash_accounts`).Scan(&total); err != nil {
		return nil, 0, err
	}
	if size < 1 || size > 100 {
		size = 20
	}
	if page < 1 {
		page = 1
	}
	rows, err := pool.Query(ctx, `
		SELECT a.tenant_id, a.balance, a.frozen, a.updated_at, COALESCE(t.tenant_name, '')
		FROM pay_cash_accounts a
		LEFT JOIN iam_tenants t ON t.tenant_id = a.tenant_id
		ORDER BY a.updated_at DESC LIMIT $1 OFFSET $2
	`, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []*CashAccountRow
	for rows.Next() {
		var r CashAccountRow
		if err := rows.Scan(&r.TenantID, &r.Balance, &r.Frozen, &r.UpdatedAt, &r.TenantName); err != nil {
			return nil, 0, err
		}
		list = append(list, &r)
	}
	return list, total, rows.Err()
}
