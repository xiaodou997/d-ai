package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"xiaodou/dai/internal/billing"
)

// GetTenantAvailableBalance 查询租户可用余额（事务内）
// 返回: available 可用积分, frozen 冻结积分
func GetTenantAvailableBalance(ctx context.Context, tx pgx.Tx, tenantID string, now time.Time) (available, frozen int64, err error) {
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(frozen_credits, 0) FROM iam_tenants WHERE tenant_id = $1 FOR UPDATE
	`, tenantID).Scan(&frozen)
	if err != nil {
		return 0, 0, fmt.Errorf("查询租户冻结积分失败: %w", err)
	}

	var remaining int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM (
			SELECT remaining_credits
			FROM bill_credit_packages
			WHERE package_type = 'tenant' AND tenant_id = $1 AND status = 'available'
			  AND (expires_at IS NULL OR expires_at > $2)
			FOR UPDATE
		) sub
	`, tenantID, now).Scan(&remaining)
	if err != nil {
		return 0, 0, fmt.Errorf("查询租户剩余积分失败: %w", err)
	}

	available = remaining - frozen
	if available < 0 {
		available = 0
	}
	return available, frozen, nil
}

// GetUserAvailableBalance 查询用户可用余额（事务内）
// 返回: available 可用积分, frozen 冻结积分
func GetUserAvailableBalance(ctx context.Context, tx pgx.Tx, userID string, now time.Time) (available, frozen int64, err error) {
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(frozen_credits, 0) FROM iam_accounts WHERE user_id = $1 AND user_type = 4 FOR UPDATE
	`, userID).Scan(&frozen)
	if err != nil {
		return 0, 0, fmt.Errorf("查询用户冻结积分失败: %w", err)
	}

	var remaining int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(remaining_credits), 0)
		FROM (
			SELECT remaining_credits
			FROM bill_credit_packages
			WHERE package_type = 'user' AND user_id = $1 AND status = 'available'
			  AND (expires_at IS NULL OR expires_at > $2)
			FOR UPDATE
		) sub
	`, userID, now).Scan(&remaining)
	if err != nil {
		return 0, 0, fmt.Errorf("查询用户剩余积分失败: %w", err)
	}

	available = remaining - frozen
	if available < 0 {
		available = 0
	}
	return available, frozen, nil
}

// AddTenantFrozen 增加租户冻结积分（事务内）
func AddTenantFrozen(ctx context.Context, tx pgx.Tx, tenantID string, amount int64) error {
	result, err := tx.Exec(ctx, `
		UPDATE iam_tenants
		SET frozen_credits = frozen_credits + $1
		WHERE tenant_id = $2
	`, amount, tenantID)
	if err != nil {
		return fmt.Errorf("增加租户冻结积分失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("租户账户不存在: %s", tenantID)
	}
	return nil
}

// AddUserFrozen 增加用户冻结积分（事务内）
func AddUserFrozen(ctx context.Context, tx pgx.Tx, userID string, amount int64) error {
	result, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET frozen_credits = frozen_credits + $1
		WHERE user_id = $2 AND user_type = 4
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("增加用户冻结积分失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("用户账户不存在: %s", userID)
	}
	return nil
}

// ReduceTenantFrozen 减少租户冻结积分（事务内）
func ReduceTenantFrozen(ctx context.Context, tx pgx.Tx, tenantID string, amount int64) error {
	result, err := tx.Exec(ctx, `
		UPDATE iam_tenants
		SET frozen_credits = frozen_credits - $1, updated_at = now()
		WHERE tenant_id = $2 AND frozen_credits >= $1
	`, amount, tenantID)
	if err != nil {
		return fmt.Errorf("减少租户冻结积分失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("租户账户不存在或冻结积分不足: %s", tenantID)
	}
	return nil
}

// ReduceUserFrozen 减少用户冻结积分（事务内）
func ReduceUserFrozen(ctx context.Context, tx pgx.Tx, userID string, amount int64) error {
	result, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET frozen_credits = frozen_credits - $1, updated_at = now()
		WHERE user_id = $2 AND user_type = 4 AND frozen_credits >= $1
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("减少用户冻结积分失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("用户账户不存在或冻结积分不足: %s", userID)
	}
	return nil
}

// DeductFIFO FIFO 扣减积分（事务内）
// 按过期时间升序消耗，永久积分（expires_at IS NULL）排最后
func DeductFIFO(ctx context.Context, tx pgx.Tx, packageType string, tenantID, userID string, amount int64, now time.Time) error {
	remaining := amount

	var rows pgx.Rows
	var err error

	if packageType == billing.PackageTypeTenant {
		rows, err = tx.Query(ctx, `
			SELECT package_id, remaining_credits
			FROM bill_credit_packages
			WHERE package_type = $1 AND tenant_id = $2 AND status = 'available'
			  AND remaining_credits > 0
			  AND (expires_at IS NULL OR expires_at > $3)
			ORDER BY
			  CASE WHEN expires_at IS NULL THEN 1 ELSE 0 END,
			  expires_at ASC,
			  created_at ASC
			FOR UPDATE
		`, packageType, tenantID, now)
	} else {
		rows, err = tx.Query(ctx, `
			SELECT package_id, remaining_credits
			FROM bill_credit_packages
			WHERE package_type = $1 AND user_id = $2 AND status = 'available'
			  AND remaining_credits > 0
			  AND (expires_at IS NULL OR expires_at > $3)
			ORDER BY
			  CASE WHEN expires_at IS NULL THEN 1 ELSE 0 END,
			  expires_at ASC,
			  created_at ASC
			FOR UPDATE
		`, packageType, userID, now)
	}
	if err != nil {
		return fmt.Errorf("查询积分包失败: %w", err)
	}

	// 先把所有行读进内存再关闭游标，避免在 rows 未关闭时复用同一连接执行 UPDATE
	// （pgx v5 同一连接只能有一个在途操作，否则报 conn busy）
	type pkgRow struct {
		id        string
		remaining int64
	}
	var packages []pkgRow
	for rows.Next() {
		var r pkgRow
		if err := rows.Scan(&r.id, &r.remaining); err != nil {
			rows.Close()
			return fmt.Errorf("扫描积分包失败: %w", err)
		}
		packages = append(packages, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取积分包失败: %w", err)
	}

	for _, pkg := range packages {
		if remaining <= 0 {
			break
		}
		deduct := remaining
		if deduct > pkg.remaining {
			deduct = pkg.remaining
		}
		_, err := tx.Exec(ctx, `
			UPDATE bill_credit_packages
			SET remaining_credits = remaining_credits - $1, updated_at = now()
			WHERE package_id = $2
		`, deduct, pkg.id)
		if err != nil {
			return fmt.Errorf("扣减积分包失败: %w", err)
		}
		remaining -= deduct
	}

	if remaining > 0 {
		return fmt.Errorf("积分不足，缺少 %d 积分", remaining)
	}

	return nil
}

// ============================================================================
// 欠费状态 helpers。overdraft_limit 仅为兼容旧表结构保留，V2 不将其作为信用额度。
// ============================================================================

// GetTenantOverdraft 读取租户透支额度上限与当前已用额度（事务内，FOR UPDATE 行锁）
func GetTenantOverdraft(ctx context.Context, tx pgx.Tx, tenantID string) (limit, current int64, err error) {
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(overdraft_limit, 0), COALESCE(current_overdraft, 0)
		FROM iam_tenants WHERE tenant_id = $1 FOR UPDATE
	`, tenantID).Scan(&limit, &current)
	if err != nil {
		return 0, 0, fmt.Errorf("查询租户透支额度失败: %w", err)
	}
	return limit, current, nil
}

// GetUserOverdraft 读取用户透支额度上限与当前已用额度（事务内，FOR UPDATE 行锁）
func GetUserOverdraft(ctx context.Context, tx pgx.Tx, userID string) (limit, current int64, err error) {
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(overdraft_limit, 0), COALESCE(current_overdraft, 0)
		FROM iam_accounts WHERE user_id = $1 AND user_type = 4 FOR UPDATE
	`, userID).Scan(&limit, &current)
	if err != nil {
		return 0, 0, fmt.Errorf("查询用户透支额度失败: %w", err)
	}
	return limit, current, nil
}

// IncreaseTenantOverdraft 增加租户在途尾差形成的欠费（事务内）。
func IncreaseTenantOverdraft(ctx context.Context, tx pgx.Tx, tenantID string, amount int64) error {
	if amount <= 0 {
		return nil
	}
	result, err := tx.Exec(ctx, `
		UPDATE iam_tenants
		SET current_overdraft = current_overdraft + $1, updated_at = now()
		WHERE tenant_id = $2
	`, amount, tenantID)
	if err != nil {
		return fmt.Errorf("增加租户透支额度失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("租户账户不存在: %s", tenantID)
	}
	return nil
}

// IncreaseUserOverdraft 增加用户在途尾差形成的欠费（事务内）。
func IncreaseUserOverdraft(ctx context.Context, tx pgx.Tx, userID string, amount int64) error {
	if amount <= 0 {
		return nil
	}
	result, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET current_overdraft = current_overdraft + $1, updated_at = now()
		WHERE user_id = $2 AND user_type = 4
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("增加用户透支额度失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("用户账户不存在: %s", userID)
	}
	return nil
}

// DecreaseTenantOverdraft 抵扣租户透支额度（事务内）。
// 返回实际抵扣量 min(amount, current_overdraft_before)。
func DecreaseTenantOverdraft(ctx context.Context, tx pgx.Tx, tenantID string, amount int64) (deducted int64, err error) {
	if amount <= 0 {
		return 0, nil
	}
	// 先 FOR UPDATE 锁住 + 读当前值，再 UPDATE，两步 SQL 同事务原子。
	var before int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(current_overdraft, 0) FROM iam_tenants
		WHERE tenant_id = $1 FOR UPDATE
	`, tenantID).Scan(&before); err != nil {
		return 0, fmt.Errorf("查询租户透支额度失败: %w", err)
	}
	deducted = amount
	if before < deducted {
		deducted = before
	}
	if deducted == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE iam_tenants
		SET current_overdraft = current_overdraft - $1, updated_at = now()
		WHERE tenant_id = $2
	`, deducted, tenantID); err != nil {
		return 0, fmt.Errorf("抵扣租户透支额度失败: %w", err)
	}
	return deducted, nil
}

// DecreaseUserOverdraft 抵扣用户透支额度（事务内）。
// 返回实际抵扣量 min(amount, current_overdraft_before)。
func DecreaseUserOverdraft(ctx context.Context, tx pgx.Tx, userID string, amount int64) (deducted int64, err error) {
	if amount <= 0 {
		return 0, nil
	}
	var before int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(current_overdraft, 0) FROM iam_accounts
		WHERE user_id = $1 AND user_type = 4 FOR UPDATE
	`, userID).Scan(&before); err != nil {
		return 0, fmt.Errorf("查询用户透支额度失败: %w", err)
	}
	deducted = amount
	if before < deducted {
		deducted = before
	}
	if deducted == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET current_overdraft = current_overdraft - $1, updated_at = now()
		WHERE user_id = $2 AND user_type = 4
	`, deducted, userID); err != nil {
		return 0, fmt.Errorf("抵扣用户透支额度失败: %w", err)
	}
	return deducted, nil
}

// DeductFIFOPartialPreservingFrozen deducts only credits available to this
// operation. ownedFrozen may be non-zero only when this transaction owns that
// amount of the account's current frozen balance, such as a pending
// authorization capture. Other frozen credits are never consumed.
func DeductFIFOPartialPreservingFrozen(
	ctx context.Context,
	tx pgx.Tx,
	packageType, tenantID, userID string,
	amount, ownedFrozen int64,
	now time.Time,
) (shortfall int64, err error) {
	if amount <= 0 {
		return 0, nil
	}
	if ownedFrozen < 0 {
		return 0, fmt.Errorf("owned frozen credits cannot be negative")
	}

	var available, frozen int64
	switch packageType {
	case billing.PackageTypeTenant:
		available, frozen, err = GetTenantAvailableBalance(ctx, tx, tenantID, now)
	case billing.PackageTypeUser:
		available, frozen, err = GetUserAvailableBalance(ctx, tx, userID, now)
	default:
		return 0, fmt.Errorf("unsupported package type %q", packageType)
	}
	if err != nil {
		return 0, err
	}
	if ownedFrozen > frozen {
		return 0, fmt.Errorf("owned frozen credits %d exceed account frozen credits %d", ownedFrozen, frozen)
	}

	deductible := amount
	if deductible > available && deductible-available > ownedFrozen {
		deductible = available + ownedFrozen
	}
	shortfall = amount - deductible
	if deductible == 0 {
		return shortfall, nil
	}
	unexpectedShortfall, err := deductFIFOPartial(
		ctx, tx, packageType, tenantID, userID, deductible, now)
	if err != nil {
		return 0, err
	}
	if unexpectedShortfall != 0 {
		return 0, fmt.Errorf("protected FIFO deduction lost %d locked credits", unexpectedShortfall)
	}
	return shortfall, nil
}

// deductFIFOPartial performs the raw package deduction after the caller has
// reserved the account/package rows and capped the amount around frozen funds.
func deductFIFOPartial(ctx context.Context, tx pgx.Tx, packageType string, tenantID, userID string, amount int64, now time.Time) (shortfall int64, err error) {
	if amount <= 0 {
		return 0, nil
	}

	var rows pgx.Rows
	if packageType == billing.PackageTypeTenant {
		rows, err = tx.Query(ctx, `
			SELECT package_id, remaining_credits
			FROM bill_credit_packages
			WHERE package_type = $1 AND tenant_id = $2 AND status = 'available'
			  AND remaining_credits > 0
			  AND (expires_at IS NULL OR expires_at > $3)
			ORDER BY
			  CASE WHEN expires_at IS NULL THEN 1 ELSE 0 END,
			  expires_at ASC,
			  created_at ASC
			FOR UPDATE
		`, packageType, tenantID, now)
	} else {
		rows, err = tx.Query(ctx, `
			SELECT package_id, remaining_credits
			FROM bill_credit_packages
			WHERE package_type = $1 AND user_id = $2 AND status = 'available'
			  AND remaining_credits > 0
			  AND (expires_at IS NULL OR expires_at > $3)
			ORDER BY
			  CASE WHEN expires_at IS NULL THEN 1 ELSE 0 END,
			  expires_at ASC,
			  created_at ASC
			FOR UPDATE
		`, packageType, userID, now)
	}
	if err != nil {
		return 0, fmt.Errorf("查询积分包失败: %w", err)
	}

	type pkgRow struct {
		id        string
		remaining int64
	}
	var packages []pkgRow
	for rows.Next() {
		var r pkgRow
		if err := rows.Scan(&r.id, &r.remaining); err != nil {
			rows.Close()
			return 0, fmt.Errorf("扫描积分包失败: %w", err)
		}
		packages = append(packages, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("读取积分包失败: %w", err)
	}

	remaining := amount
	for _, pkg := range packages {
		if remaining <= 0 {
			break
		}
		deduct := remaining
		if deduct > pkg.remaining {
			deduct = pkg.remaining
		}
		if _, err := tx.Exec(ctx, `
			UPDATE bill_credit_packages
			SET remaining_credits = remaining_credits - $1, updated_at = now()
			WHERE package_id = $2
		`, deduct, pkg.id); err != nil {
			return 0, fmt.Errorf("扣减积分包失败: %w", err)
		}
		remaining -= deduct
	}

	return remaining, nil
}

// RevokeCreditPackage 撤销积分包（事务内）
// fullRevoke=true → status='revoked'（积分未被消耗，完整撤销）
// fullRevoke=false → status='depleted'（积分已部分消耗，剩余撤销）
func RevokeCreditPackage(ctx context.Context, tx pgx.Tx, packageID string, fullRevoke bool) error {
	newStatus := billing.PackageStatusDepleted
	if fullRevoke {
		newStatus = billing.PackageStatusRevoked
	}
	result, err := tx.Exec(ctx, `
		UPDATE bill_credit_packages
		SET remaining_credits = 0, status = $1, updated_at = now()
		WHERE package_id = $2
	`, newStatus, packageID)
	if err != nil {
		return fmt.Errorf("撤销积分包失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("积分包不存在: %s", packageID)
	}
	return nil
}

// CreateRefundPackage 退款时新建积分包（事务内）
// 退款积分包永久有效（expires_at=NULL），不关联充值订单
func CreateRefundPackage(ctx context.Context, tx pgx.Tx, packageID, packageType, tenantID, userID string, credits int64, now time.Time) error {
	var userIDVal any
	if userID != "" {
		userIDVal = userID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO bill_credit_packages
		(package_id, package_type, tenant_id, user_id,
		 total_credits, remaining_credits, expires_at, status, source,
		 recharge_order_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULL, 'available', 'REFUND', NULL, 0, $7, $7)
	`, packageID, packageType, tenantID, userIDVal, credits, credits, now)
	if err != nil {
		return fmt.Errorf("创建退款积分包失败: %w", err)
	}
	return nil
}
