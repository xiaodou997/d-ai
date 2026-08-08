package pg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/billing"
)

// TenantUser 租户用户实体
type TenantUser struct {
	UserID        string
	TenantID      string
	Username      string
	Email         string
	Phone         string
	Status        int
	LastLoginTime *int64
	CreatedTime   int64
}

// EndUserItem 终端用户列表条目
type EndUserItem struct {
	UserID      string `json:"userId"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Nickname    string `json:"nickname"`
	Status      int    `json:"status"`
	CreatedTime int64  `json:"createdTime"`
}

// InviteCodeItem 邀请码列表条目
type InviteCodeItem struct {
	ID              int64  `json:"id"`
	Code            string `json:"code"`
	RegistrationURL string `json:"registrationUrl,omitempty"`
	TenantID        string `json:"tenantId"`
	CreatedBy       string `json:"createdBy"`
	Description     string `json:"description"`
	MaxUses         int    `json:"maxUses"`
	UsedCount       int    `json:"usedCount"`
	Status          int    `json:"status"`
	ExpireTime      *int64 `json:"expireTime,omitempty"`
	CreatedTime     int64  `json:"createdTime"`
	UpdatedTime     int64  `json:"updatedTime"`
}

// TenantStats 租户统计数据
type TenantStats struct {
	EndUserCount     int64   `json:"endUserCount"`
	InviteCodeCount  int64   `json:"inviteCodeCount"`
	UserDeductionUSD float64 `json:"userDeductionUsd"`
}

// EventItem 消费流水条目
type EventItem struct {
	ID              int64   `json:"id"`
	EventID         string  `json:"eventId"`
	TenantID        string  `json:"tenantId"`
	UserID          string  `json:"userId,omitempty"`
	ClientID        string  `json:"clientId,omitempty"`
	Description     string  `json:"description"`
	TenantAmountUSD float64 `json:"tenantAmountUsd,omitempty"`
	UserAmountUSD   float64 `json:"userAmountUsd,omitempty"`
	Username        string  `json:"username,omitempty"`
	TenantName      string  `json:"tenantName,omitempty"`
	Status          int     `json:"status"`
	CreatedTime     int64   `json:"createdTime"`
}

// RechargeItem 充值记录条目
type RechargeItem struct {
	ID              int64   `json:"id"`
	OrderID         string  `json:"orderId"`
	OrderType       string  `json:"orderType"`
	TenantID        string  `json:"tenantId"`
	UserID          string  `json:"userId"`
	PaidAmountMinor int64   `json:"paidAmountMinor"`
	AmountUSD       float64 `json:"amountUsd"`
	Status          string  `json:"status"`
	Note            string  `json:"note"`
	CreatedTime     int64   `json:"createdTime"`
}

// TenantRepo 租户数据访问层
type TenantRepo struct {
	pool *pgxpool.Pool
}

// NewTenantRepo 创建 TenantRepo 实例
func NewTenantRepo(pool *pgxpool.Pool) *TenantRepo {
	return &TenantRepo{pool: pool}
}

// GetByTenantAndUsername 查询租户用户（用于登录），返回 (user, passwordHash, error)
func (r *TenantRepo) GetByTenantAndUsername(ctx context.Context, tenantID, username string) (*TenantUser, string, error) {
	u := &TenantUser{}
	var passwordHash string
	var status string
	var lastLoginAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, tenant_id, username, password_hash, email, phone, status, last_login_at
			FROM iam_accounts
			WHERE tenant_id = $1 AND username = $2 AND user_type = 3
	`, tenantID, username).Scan(
		&u.UserID, &u.TenantID, &u.Username, &passwordHash,
		&u.Email, &u.Phone, &status, &lastLoginAt,
	)
	if err == pgx.ErrNoRows {
		return nil, "", fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, "", err
	}
	u.Status = tenantUserStatusToInt(status)
	u.LastLoginTime = millisPtr(lastLoginAt)
	return u, passwordHash, nil
}

// GetByUserID 根据 user_id 查询租户用户
func (r *TenantRepo) GetByUserID(ctx context.Context, userID string) (*TenantUser, error) {
	u := &TenantUser{}
	var status string
	var lastLoginAt *time.Time
	var createdAt time.Time
	// email/phone 可空，COALESCE 为空串以适配非指针 string 字段（直接 Scan NULL 会失败）
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, tenant_id, username, COALESCE(email, ''), COALESCE(phone, ''), status, last_login_at, created_at
			FROM iam_accounts
			WHERE user_id = $1 AND user_type = 3
	`, userID).Scan(
		&u.UserID, &u.TenantID, &u.Username,
		&u.Email, &u.Phone, &status, &lastLoginAt, &createdAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}
	u.Status = tenantUserStatusToInt(status)
	u.LastLoginTime = millisPtr(lastLoginAt)
	u.CreatedTime = createdAt.UnixMilli()
	return u, nil
}

// UpdateLastLogin 更新最后登录时间
func (r *TenantRepo) UpdateLastLogin(ctx context.Context, userID string, t int64) error {
	_, err := r.pool.Exec(ctx, `
			UPDATE iam_accounts SET last_login_at = $1 WHERE user_id = $2 AND user_type = 3
	`, time.UnixMilli(t).UTC(), userID)
	return err
}

// ListEndUsers 分页查询本租户的终端用户
func (r *TenantRepo) ListEndUsers(ctx context.Context, tenantID, keyword string, page, size int) ([]EndUserItem, int64, error) {
	var total int64
	baseWhere := "WHERE tenant_id = $1 AND user_type = 4 AND status <> 'deleted'"
	args := []any{tenantID}
	argIdx := 2

	if keyword != "" {
		baseWhere += fmt.Sprintf(" AND (username LIKE $%d OR nickname LIKE $%d)", argIdx, argIdx+1)
		like := "%" + keyword + "%"
		args = append(args, like, like)
		argIdx += 2
	}

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM iam_accounts %s", baseWhere)
	err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	queryArgs := append(args, size, offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT user_id, username, email, phone, nickname, status, created_at
			FROM iam_accounts %s
		ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, baseWhere, argIdx, argIdx+1), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []EndUserItem
	for rows.Next() {
		var item EndUserItem
		var email, phone, nickname *string
		var status string
		var createdAt time.Time
		if err := rows.Scan(
			&item.UserID, &item.Username, &email, &phone, &nickname,
			&status, &createdAt,
		); err != nil {
			continue
		}
		if email != nil {
			item.Email = *email
		}
		if phone != nil {
			item.Phone = *phone
		}
		if nickname != nil {
			item.Nickname = *nickname
		}
		item.Status = endUserStatusToInt(status)
		item.CreatedTime = createdAt.UnixMilli()
		list = append(list, item)
	}
	if list == nil {
		list = []EndUserItem{}
	}
	return list, total, rows.Err()
}

// ListInvitationCodes 查询本租户的邀请码（分页）
func (r *TenantRepo) ListInvitationCodes(ctx context.Context, tenantID string, page, size int) ([]InviteCodeItem, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_invitation_codes WHERE tenant_id = $1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	rows, err := r.pool.Query(ctx, `
		SELECT id, code, tenant_id, created_by, description, max_uses, used_count, status, expires_at, created_at, updated_at
		FROM iam_invitation_codes
		WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, tenantID, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []InviteCodeItem
	for rows.Next() {
		var item InviteCodeItem
		var status string
		var description *string
		var expiresAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&item.ID, &item.Code, &item.TenantID, &item.CreatedBy,
			&description, &item.MaxUses, &item.UsedCount,
			&status, &expiresAt, &createdAt, &updatedAt,
		); err != nil {
			continue
		}
		if description != nil {
			item.Description = *description
		}
		item.Status = inviteStatusToInt(status)
		item.ExpireTime = millisPtr(expiresAt)
		item.CreatedTime = createdAt.UnixMilli()
		item.UpdatedTime = updatedAt.UnixMilli()
		list = append(list, item)
	}
	if list == nil {
		list = []InviteCodeItem{}
	}
	return list, total, rows.Err()
}

// CreateInvitationCode 创建邀请码
func (r *TenantRepo) CreateInvitationCode(ctx context.Context, code, tenantID, createdBy, description string, maxUses int, expireTime *int64) error {
	now := time.Now().UTC()
	var expiresAt *time.Time
	if expireTime != nil {
		value := time.UnixMilli(*expireTime).UTC()
		expiresAt = &value
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO iam_invitation_codes (code, tenant_id, created_by, description, max_uses, used_count, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 0, 'active', $6, $7, $8)
	`, code, tenantID, createdBy, description, maxUses, expiresAt, now, now)
	return err
}

// UpdateInvitationCode 更新邀请码状态和描述（需验证 tenant_id）
func (r *TenantRepo) UpdateInvitationCode(ctx context.Context, id int64, tenantID string, status int, description string) error {
	parts := []string{}
	args := []any{}
	argIdx := 1

	if status != 0 {
		parts = append(parts, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, inviteStatusFromInt(status))
		argIdx++
	}
	if description != "" {
		parts = append(parts, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, description)
		argIdx++
	}
	if len(parts) == 0 {
		return nil
	}

	parts = append(parts, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now().UTC())
	argIdx++
	args = append(args, id, tenantID)

	query := fmt.Sprintf("UPDATE iam_invitation_codes SET %s WHERE id = $%d AND tenant_id = $%d", strings.Join(parts, ", "), argIdx, argIdx+1)
	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

// DeleteInvitationCode 删除邀请码（需验证 tenant_id）
func (r *TenantRepo) DeleteInvitationCode(ctx context.Context, id int64, tenantID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM iam_invitation_codes WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// GetStats 获取租户统计数据
func (r *TenantRepo) GetStats(ctx context.Context, tenantID string) (*TenantStats, error) {
	stats := &TenantStats{}
	var deductionMicro int64
	r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_accounts WHERE tenant_id = $1 AND user_type = 4 AND status <> 'deleted'`, tenantID).Scan(&stats.EndUserCount)
	r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_invitation_codes WHERE tenant_id = $1`, tenantID).Scan(&stats.InviteCodeCount)
	r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(user_credits), 0) FROM bill_events WHERE tenant_id = $1 AND status = 'succeeded' AND event_type = 'charge'`, tenantID).Scan(&deductionMicro)
	stats.UserDeductionUSD = billing.MicroToUSD(deductionMicro)
	return stats, nil
}

// ListTransactions 查询消费流水（分页）
func (r *TenantRepo) ListTransactions(ctx context.Context, tenantID string, page, size int) ([]EventItem, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_events WHERE tenant_id = $1 AND event_type = 'charge'`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	rows, err := r.pool.Query(ctx, `
		SELECT dt.id, dt.event_id, dt.tenant_id, COALESCE(dt.user_id,''), COALESCE(dt.client_id,''),
		       COALESCE(dt.description,''), COALESCE(dt.tenant_credits,0), COALESCE(dt.user_credits,0),
		       dt.status, dt.created_at,
		       COALESCE(eu.username, '') AS username,
		       COALESCE(t.tenant_name, '') AS tenant_name
		FROM bill_events dt
		LEFT JOIN iam_tenants t ON t.tenant_id = dt.tenant_id
			LEFT JOIN iam_accounts eu ON eu.user_id = dt.user_id AND eu.user_type = 4
		WHERE dt.tenant_id = $1 AND dt.event_type = 'charge'
		ORDER BY dt.created_at DESC LIMIT $2 OFFSET $3
	`, tenantID, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []EventItem
	for rows.Next() {
		var item EventItem
		var tenantMicro, userMicro int64
		var status string
		var createdAt time.Time
		if err := rows.Scan(
			&item.ID, &item.EventID, &item.TenantID,
			&item.UserID, &item.ClientID, &item.Description,
			&tenantMicro, &userMicro, &status, &createdAt,
			&item.Username, &item.TenantName,
		); err != nil {
			continue
		}
		item.Status = txStatusToInt(status)
		item.TenantAmountUSD = billing.MicroToUSD(tenantMicro)
		item.UserAmountUSD = billing.MicroToUSD(userMicro)
		item.CreatedTime = createdAt.UnixMilli()
		list = append(list, item)
	}
	if list == nil {
		list = []EventItem{}
	}
	return list, total, rows.Err()
}

// ListRechargeRecords 查询充值记录（分页）—— 租户只查看本租户收到的充值
// （platform_to_tenant 管理员手动充值 + online_tenant_topup 微信在线充值）
func (r *TenantRepo) ListRechargeRecords(ctx context.Context, tenantID string, page, size int) ([]RechargeItem, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM bill_recharge_orders WHERE order_type IN ('platform_to_tenant', 'online_tenant_topup') AND tenant_id = $1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	rows, err := r.pool.Query(ctx, `
		SELECT id, order_id, order_type, tenant_id, COALESCE(user_id,''), paid_amount, credit_amount, status, COALESCE(note,''), created_at
		FROM bill_recharge_orders
		WHERE order_type IN ('platform_to_tenant', 'online_tenant_topup') AND tenant_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, tenantID, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []RechargeItem
	for rows.Next() {
		var item RechargeItem
		var creditMicro int64
		var createdAt time.Time
		if err := rows.Scan(
			&item.ID, &item.OrderID, &item.OrderType, &item.TenantID, &item.UserID,
			&item.PaidAmountMinor, &creditMicro, &item.Status, &item.Note, &createdAt,
		); err != nil {
			continue
		}
		item.CreatedTime = createdAt.UnixMilli()
		item.AmountUSD = billing.MicroToUSD(creditMicro)
		list = append(list, item)
	}
	if list == nil {
		list = []RechargeItem{}
	}
	return list, total, rows.Err()
}

// TenantOverviewStats 扩展的租户统计数据（含用户 USD 余额和活跃用户）
type TenantOverviewStats struct {
	EndUserCount             int64   `json:"endUserCount"`
	InviteCodeCount          int64   `json:"inviteCodeCount"`
	UserDeductionUSD         float64 `json:"userDeductionUsd"`
	UserTotalBalanceUSD      float64 `json:"userTotalBalanceUsd"`
	ActiveUserCount          int64   `json:"activeUserCount"`
	UserConsumptionCount     int64   `json:"userConsumptionCount"`
	SettlementIncomeMicroUSD int64   `json:"settlementIncomeMicroUsd"`
}

// ClientConsumptionItem APP 消耗统计条目
type ClientConsumptionItem struct {
	ClientID   string  `json:"clientId"`
	ClientName string  `json:"clientName"`
	AmountUSD  float64 `json:"amountUsd"`
	Percentage string  `json:"percentage"`
}

// UserConsumptionItem 用户消费贡献排行条目。
type UserConsumptionItem struct {
	UserID           string  `json:"userId"`
	Username         string  `json:"username"`
	AmountUSD        float64 `json:"amountUsd"`
	TransactionCount int64   `json:"transactionCount"`
	Percentage       string  `json:"percentage"`
}

// GetTenantOverviewStats 获取扩展的租户统计数据。
// timeFrom/timeTo 仅作用于行为指标；存量指标保持全量口径。
func (r *TenantRepo) GetTenantOverviewStats(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) (*TenantOverviewStats, error) {
	stats := &TenantOverviewStats{}
	var userTotalMicro, userDeductionMicro int64

	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_accounts WHERE tenant_id = $1 AND user_type = 4 AND status <> 'deleted'`, tenantID).Scan(&stats.EndUserCount); err != nil {
		return nil, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_invitation_codes WHERE tenant_id = $1`, tenantID).Scan(&stats.InviteCodeCount); err != nil {
		return nil, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(remaining_credits), 0) FROM bill_credit_packages WHERE package_type = 'user' AND tenant_id = $1 AND status = 'available'`, tenantID).Scan(&userTotalMicro); err != nil {
		return nil, err
	}

	behaviorBase := `
		FROM bill_events
		WHERE tenant_id = $1
		  AND status = 'succeeded'
		  AND event_type = 'charge'
	`
	behaviorArgs := []any{tenantID}
	argIdx := 2
	if timeFrom != nil {
		behaviorBase += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		behaviorArgs = append(behaviorArgs, *timeFrom)
		argIdx++
	}
	if timeTo != nil {
		behaviorBase += fmt.Sprintf(" AND created_at < $%d", argIdx)
		behaviorArgs = append(behaviorArgs, *timeTo)
		argIdx++
	}

	if err := r.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(COALESCE(user_credits, 0)), 0),
			COUNT(*) FILTER (WHERE user_id IS NOT NULL AND COALESCE(user_credits, 0) > 0),
			COUNT(DISTINCT user_id) FILTER (WHERE user_id IS NOT NULL AND COALESCE(user_credits, 0) > 0)
	`+behaviorBase, behaviorArgs...).Scan(
		&userDeductionMicro,
		&stats.UserConsumptionCount,
		&stats.ActiveUserCount,
	); err != nil {
		return nil, err
	}

	incomeQuery := `
		SELECT COALESCE(SUM(amount_micro_usd), 0)
		FROM pay_cash_ledger
		WHERE tenant_id = $1 AND txn_type = 'topup_income' AND amount_micro_usd > 0
	`
	incomeArgs := []any{tenantID}
	incomeArgIdx := 2
	if timeFrom != nil {
		incomeQuery += fmt.Sprintf(" AND created_at >= $%d", incomeArgIdx)
		incomeArgs = append(incomeArgs, *timeFrom)
		incomeArgIdx++
	}
	if timeTo != nil {
		incomeQuery += fmt.Sprintf(" AND created_at < $%d", incomeArgIdx)
		incomeArgs = append(incomeArgs, *timeTo)
	}
	if err := r.pool.QueryRow(ctx, incomeQuery, incomeArgs...).Scan(&stats.SettlementIncomeMicroUSD); err != nil {
		return nil, err
	}
	stats.UserTotalBalanceUSD = billing.MicroToUSD(userTotalMicro)
	stats.UserDeductionUSD = billing.MicroToUSD(userDeductionMicro)

	return stats, nil
}

// GetUserConsumptionRanking 返回 USD 消费最高的终端用户。
func (r *TenantRepo) GetUserConsumptionRanking(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time, limit int) ([]UserConsumptionItem, error) {
	if limit < 1 || limit > 20 {
		limit = 10
	}
	query := `
		SELECT
			e.user_id,
			COALESCE(NULLIF(u.username, ''), '已删除用户') AS username,
			SUM(COALESCE(e.user_credits, 0)) AS credits,
			COUNT(*) AS transaction_count,
			SUM(SUM(COALESCE(e.user_credits, 0))) OVER () AS total_credits
		FROM bill_events e
			LEFT JOIN iam_accounts u ON u.user_id = e.user_id AND u.user_type = 4
		WHERE e.tenant_id = $1
		  AND e.status = 'succeeded'
		  AND e.event_type = 'charge'
		  AND e.user_id IS NOT NULL
		  AND COALESCE(e.user_credits, 0) > 0
	`
	args := []any{tenantID}
	argIdx := 2
	if timeFrom != nil {
		query += fmt.Sprintf(" AND e.created_at >= $%d", argIdx)
		args = append(args, *timeFrom)
		argIdx++
	}
	if timeTo != nil {
		query += fmt.Sprintf(" AND e.created_at < $%d", argIdx)
		args = append(args, *timeTo)
		argIdx++
	}
	query += fmt.Sprintf(`
		GROUP BY e.user_id, u.username
		ORDER BY credits DESC, transaction_count DESC, e.user_id
		LIMIT $%d
	`, argIdx)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]UserConsumptionItem, 0, limit)
	for rows.Next() {
		var item UserConsumptionItem
		var creditsMicro, totalCreditsMicro int64
		if err := rows.Scan(&item.UserID, &item.Username, &creditsMicro, &item.TransactionCount, &totalCreditsMicro); err != nil {
			return nil, err
		}
		item.AmountUSD = billing.MicroToUSD(creditsMicro)
		if totalCreditsMicro > 0 {
			item.Percentage = fmt.Sprintf("%.1f", float64(creditsMicro)*100/float64(totalCreditsMicro))
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// GetClientConsumption 获取按 APP 消耗分布（用于饼状图）
func (r *TenantRepo) GetClientConsumption(ctx context.Context, tenantID string, timeFrom, timeTo *time.Time) ([]ClientConsumptionItem, error) {
	query := `
			SELECT
				COALESCE(dt.client_id, '') AS client_id,
				COALESCE(NULLIF(dt.client_id, ''), 'D-AI') AS client_name,
				SUM(COALESCE(dt.user_credits, 0)) AS credits
			FROM bill_events dt
			WHERE dt.tenant_id = $1 AND dt.status = 'succeeded' AND dt.event_type = 'charge'
	`
	args := []any{tenantID}
	argIdx := 2
	if timeFrom != nil {
		query += fmt.Sprintf(" AND dt.created_at >= $%d", argIdx)
		args = append(args, *timeFrom)
		argIdx++
	}
	if timeTo != nil {
		query += fmt.Sprintf(" AND dt.created_at < $%d", argIdx)
		args = append(args, *timeTo)
		argIdx++
	}
	query += `
				GROUP BY dt.client_id
				HAVING SUM(COALESCE(dt.user_credits, 0)) > 0
				ORDER BY credits DESC
	`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ClientConsumptionItem
	var totalCredits float64

	for rows.Next() {
		var item ClientConsumptionItem
		var creditsMicro int64
		if err := rows.Scan(&item.ClientID, &item.ClientName, &creditsMicro); err != nil {
			continue
		}
		item.AmountUSD = billing.MicroToUSD(creditsMicro)
		totalCredits += item.AmountUSD
		list = append(list, item)
	}

	if totalCredits > 0 {
		for i := range list {
			percentage := list[i].AmountUSD * 100 / totalCredits
			list[i].Percentage = fmt.Sprintf("%.1f", percentage)
		}
	}

	if list == nil {
		list = []ClientConsumptionItem{}
	}
	return list, nil
}

func tenantUserStatusToInt(status string) int {
	switch status {
	case "disabled":
		return 2
	case "inherited_disabled":
		return 3
	default:
		return 1
	}
}

func endUserStatusToInt(status string) int {
	switch status {
	case "disabled":
		return 2
	case "locked":
		return 3
	case "inherited_disabled":
		return 4
	default:
		return 1
	}
}

func inviteStatusToInt(status string) int {
	if status == "disabled" {
		return 2
	}
	return 1
}

func inviteStatusFromInt(status int) string {
	if status == 2 {
		return "disabled"
	}
	return "active"
}

func txStatusToInt(status string) int {
	switch status {
	case "succeeded":
		return 1
	case "cancelled":
		return 2
	case "refunded":
		return 3
	case "released":
		return 4
	default:
		return 0
	}
}

func millisPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	value := t.UnixMilli()
	return &value
}
