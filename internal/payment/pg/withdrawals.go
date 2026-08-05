package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/payment"
)

// InsertWithdrawalTx 事务内写入提现申请（pending 状态）。与冻结现金余额必须同一事务，
// 因此只提供 tx 版本，不提供 pool 版本。
func InsertWithdrawalTx(ctx context.Context, tx pgx.Tx, w *payment.Withdrawal) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO pay_withdrawals
		(withdrawal_id, tenant_id, amount, fee_amount, payout_amount, account_name, bank_name, account_no, apply_note, status, applied_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())
	`, w.WithdrawalID, w.TenantID, w.Amount, w.FeeAmount, w.PayoutAmount, w.AccountName, w.BankName, w.AccountNo, w.ApplyNote, payment.WithdrawalStatusPending, w.AppliedBy)
	if err != nil {
		return fmt.Errorf("创建提现申请失败: %w", err)
	}
	return nil
}

const withdrawalColumns = `
	id, withdrawal_id, tenant_id, amount, fee_amount, payout_amount, account_name, bank_name, account_no,
	COALESCE(apply_note,''), status, applied_by, COALESCE(reviewed_by,''), reviewed_at, COALESCE(review_note,''),
	COALESCE(paid_by,''), paid_at, COALESCE(payment_ref,''), created_at, updated_at`

func scanWithdrawal(row pgx.Row) (*payment.Withdrawal, error) {
	var w payment.Withdrawal
	if err := row.Scan(
		&w.ID, &w.WithdrawalID, &w.TenantID, &w.Amount, &w.FeeAmount, &w.PayoutAmount, &w.AccountName, &w.BankName, &w.AccountNo,
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

// GetWithdrawalForUpdateTx 事务内查询并加行锁——审核/核销/取消前必须先拿到锁，防并发双审。
func GetWithdrawalForUpdateTx(ctx context.Context, tx pgx.Tx, withdrawalID string) (*payment.Withdrawal, error) {
	row := tx.QueryRow(ctx, `SELECT `+withdrawalColumns+` FROM pay_withdrawals WHERE withdrawal_id = $1 FOR UPDATE`, withdrawalID)
	return scanWithdrawal(row)
}

// UpdateWithdrawalReviewTx 审核（approved/rejected），要求调用方已在同一事务内 FOR UPDATE 锁住该行
// 并自行校验前态；这里只做 CAS 兜底 + 落库。
func UpdateWithdrawalReviewTx(ctx context.Context, tx pgx.Tx, withdrawalID, toStatus, reviewerID, note string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE pay_withdrawals SET status = $1, reviewed_by = $2, reviewed_at = now(), review_note = $3, updated_at = now()
		WHERE withdrawal_id = $4 AND status = $5
	`, toStatus, reviewerID, note, withdrawalID, payment.WithdrawalStatusPending)
	if err != nil {
		return fmt.Errorf("审核提现失败: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("提现申请状态已变化")
	}
	return nil
}

// UpdateWithdrawalSettleTx 线下打款核销（approved -> paid）。
func UpdateWithdrawalSettleTx(ctx context.Context, tx pgx.Tx, withdrawalID, paidBy, paymentRef string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE pay_withdrawals SET status = $1, paid_by = $2, paid_at = now(), payment_ref = $3, updated_at = now()
		WHERE withdrawal_id = $4 AND status = $5
	`, payment.WithdrawalStatusPaid, paidBy, paymentRef, withdrawalID, payment.WithdrawalStatusApproved)
	if err != nil {
		return fmt.Errorf("核销提现失败: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("提现申请状态已变化")
	}
	return nil
}

// UpdateWithdrawalCancelTx 申请人自取消（pending -> cancelled）。
func UpdateWithdrawalCancelTx(ctx context.Context, tx pgx.Tx, withdrawalID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE pay_withdrawals SET status = $1, updated_at = now()
		WHERE withdrawal_id = $2 AND status = $3
	`, payment.WithdrawalStatusCancelled, withdrawalID, payment.WithdrawalStatusPending)
	if err != nil {
		return fmt.Errorf("取消提现失败: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("提现申请状态已变化")
	}
	return nil
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
