package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/billing"
	billingpg "xiaodou/dai/internal/billing/pg"
	"xiaodou/dai/internal/payment"
)

// GetBalanceAccount returns the tenant's one USD balance, projected from all
// live balance lots. Historical frozen fields are intentionally not exposed or
// used as an active restriction in the direct-charge model.
func GetBalanceAccount(ctx context.Context, pool *pgxpool.Pool, tenantID string) (*payment.BalanceAccount, error) {
	a := &payment.BalanceAccount{TenantID: tenantID}
	err := pool.QueryRow(ctx, `
		SELECT
		  COALESCE(SUM(p.remaining_credits) FILTER (
		    WHERE p.status = 'available' AND (p.expires_at IS NULL OR p.expires_at > now())
		  ), 0),
		  GREATEST(t.updated_at, COALESCE(MAX(p.updated_at), t.updated_at))
		FROM iam_tenants t
		LEFT JOIN bill_credit_packages p
		  ON p.package_type = 'tenant' AND p.tenant_id = t.tenant_id
		WHERE t.tenant_id = $1
		GROUP BY t.tenant_id, t.updated_at
	`, tenantID).Scan(&a.BalanceMicroUSD, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("查询租户 USD 余额失败: %w", err)
	}
	return a, nil
}

func GetBalanceAccountForUpdate(ctx context.Context, tx pgx.Tx, tenantID string) (*payment.BalanceAccount, error) {
	balance, err := billingpg.GetTenantBalanceForUpdate(ctx, tx, tenantID, billing.NowUTC())
	if err != nil {
		return nil, err
	}
	return &payment.BalanceAccount{
		TenantID: tenantID, BalanceMicroUSD: balance, UpdatedAt: billing.NowUTC(),
	}, nil
}

func DeductTenantBalanceTx(ctx context.Context, tx pgx.Tx, tenantID string, amount int64) error {
	return billingpg.DeductFIFO(ctx, tx, billing.PackageTypeTenant, tenantID, "", amount, billing.NowUTC())
}

func TenantBalanceAfterTx(ctx context.Context, tx pgx.Tx, tenantID string) (int64, error) {
	var balance int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM bill_credit_packages
		WHERE package_type = 'tenant' AND tenant_id = $1 AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
	`, tenantID, billing.NowUTC()).Scan(&balance)
	return balance, err
}

// InsertCashLedgerTx writes a unified USD balance ledger entry.
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
		(txn_id, tenant_id, txn_type, amount_micro_usd, balance_after_micro_usd, ref_type, ref_id, operator_id, note, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
	`, e.TxnID, e.TenantID, e.TxnType, e.AmountMicroUSD, e.BalanceAfterMicroUSD, refTypeVal, refIDVal, operatorVal, noteVal, idemVal)
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
		SELECT id, txn_id, tenant_id, txn_type, amount_micro_usd, balance_after_micro_usd,
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
		if err := rows.Scan(&e.ID, &e.TxnID, &e.TenantID, &e.TxnType, &e.AmountMicroUSD, &e.BalanceAfterMicroUSD,
			&e.RefType, &e.RefID, &e.OperatorID, &e.Note, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, &e)
	}
	return list, total, rows.Err()
}

// ListCashAccounts returns the same unified balance projection for admins.
type CashAccountRow struct {
	payment.BalanceAccount
	TenantName string
}

func ListCashAccounts(ctx context.Context, pool *pgxpool.Pool, page, size int) ([]*CashAccountRow, int64, error) {
	var total int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_tenants`).Scan(&total); err != nil {
		return nil, 0, err
	}
	if size < 1 || size > 100 {
		size = 20
	}
	if page < 1 {
		page = 1
	}
	rows, err := pool.Query(ctx, `
		SELECT t.tenant_id,
		       COALESCE(SUM(p.remaining_credits) FILTER (
		         WHERE p.status = 'available' AND (p.expires_at IS NULL OR p.expires_at > now())
		       ), 0) AS balance_micro_usd,
		       GREATEST(t.updated_at, COALESCE(MAX(p.updated_at), t.updated_at)),
		       COALESCE(t.tenant_name, '')
		FROM iam_tenants t
		LEFT JOIN bill_credit_packages p
		  ON p.package_type = 'tenant' AND p.tenant_id = t.tenant_id
		GROUP BY t.tenant_id, t.updated_at, t.tenant_name
		ORDER BY GREATEST(t.updated_at, COALESCE(MAX(p.updated_at), t.updated_at)) DESC
		LIMIT $1 OFFSET $2
	`, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []*CashAccountRow
	for rows.Next() {
		var r CashAccountRow
		if err := rows.Scan(&r.TenantID, &r.BalanceMicroUSD, &r.UpdatedAt, &r.TenantName); err != nil {
			return nil, 0, err
		}
		list = append(list, &r)
	}
	return list, total, rows.Err()
}
