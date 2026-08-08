package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/payment"
)

// InsertWithdrawalTx 事务内写入提现记录。旧调用方若不指定状态仍写入
// pending；新的管理员直扣调用方指定 paid，并在同一事务写入打款信息。
func InsertWithdrawalTx(ctx context.Context, tx pgx.Tx, w *payment.Withdrawal) error {
	status := w.Status
	if status == "" {
		status = payment.WithdrawalStatusPending
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO pay_withdrawals
		(withdrawal_id, tenant_id, amount_micro_usd, fee_amount_micro_usd, payout_amount_micro_usd,
		 account_name, bank_name, account_no, apply_note, status, applied_by, paid_by, paid_at, payment_ref, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
		        NULLIF($12, ''), CASE WHEN $10 = 'paid' THEN now() ELSE NULL END, NULLIF($13, ''), now(), now())
	`, w.WithdrawalID, w.TenantID, w.AmountMicroUSD, w.FeeAmountMicroUSD, w.PayoutAmountMicroUSD,
		w.AccountName, w.BankName, w.AccountNo, w.ApplyNote, status, w.AppliedBy, w.PaidBy, w.PaymentRef)
	if err != nil {
		return fmt.Errorf("创建提现申请失败: %w", err)
	}
	return nil
}

const withdrawalColumns = `
	id, withdrawal_id, tenant_id, amount_micro_usd, fee_amount_micro_usd, payout_amount_micro_usd, account_name, bank_name, account_no,
	COALESCE(apply_note,''), status, applied_by, COALESCE(reviewed_by,''), reviewed_at, COALESCE(review_note,''),
	COALESCE(paid_by,''), paid_at, COALESCE(payment_ref,''), created_at, updated_at`

func scanWithdrawal(row pgx.Row) (*payment.Withdrawal, error) {
	var w payment.Withdrawal
	if err := row.Scan(
		&w.ID, &w.WithdrawalID, &w.TenantID, &w.AmountMicroUSD, &w.FeeAmountMicroUSD, &w.PayoutAmountMicroUSD, &w.AccountName, &w.BankName, &w.AccountNo,
		&w.ApplyNote, &w.Status, &w.AppliedBy, &w.ReviewedBy, &w.ReviewedAt, &w.ReviewNote,
		&w.PaidBy, &w.PaidAt, &w.PaymentRef, &w.CreatedAt, &w.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &w, nil
}

// GetWithdrawal 按 withdrawal_id 查询（只读）。
func GetWithdrawal(ctx context.Context, pool *pgxpool.Pool, withdrawalID string) (*payment.Withdrawal, error) {
	row := pool.QueryRow(ctx, `SELECT `+withdrawalColumns+` FROM pay_withdrawals WHERE withdrawal_id = $1`, withdrawalID)
	return scanWithdrawal(row)
}

// ListWithdrawals 分页查询（管理端可按 status 筛选，租户端固定 tenantID）。
func ListWithdrawals(ctx context.Context, pool *pgxpool.Pool, tenantID, status string, page, size int) ([]*payment.Withdrawal, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	idx := 1
	if tenantID != "" {
		where += fmt.Sprintf(" AND tenant_id = $%d", idx)
		args = append(args, tenantID)
		idx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	var total int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM pay_withdrawals "+where, args...).Scan(&total); err != nil {
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
		SELECT %s FROM pay_withdrawals %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, withdrawalColumns, where, idx, idx+1), qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []*payment.Withdrawal
	for rows.Next() {
		w, err := scanWithdrawal(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, w)
	}
	return list, total, rows.Err()
}
