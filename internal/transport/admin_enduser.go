package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	billingdomain "xiaodou/dai/internal/billing"
	"xiaodou/dai/libs/go/httpx"
)

// ---- DTO ----

type endUserItem struct {
	UserID          string  `json:"userId"`
	TenantID        string  `json:"tenantId"`
	Username        string  `json:"username"`
	TenantName      string  `json:"tenantName,omitempty"`
	Email           string  `json:"email,omitempty"`
	Phone           string  `json:"phone,omitempty"`
	InternalNote    string  `json:"internalNote,omitempty"`
	Nickname        string  `json:"nickname,omitempty"`
	Avatar          string  `json:"avatar,omitempty"`
	Status          int     `json:"status"`
	CredentialState string  `json:"credentialState"`
	BalanceUSD      float64 `json:"balanceUsd"`
	LastLoginTime   *int64  `json:"lastLoginTime,omitempty"`
	CreatedTime     int64   `json:"createdTime"`
}

type listEndUsersInput struct {
	Keyword    string `query:"keyword" required:"false"`
	TenantID   string `query:"tenantId" required:"false"`
	TenantName string `query:"tenantName" required:"false"`
	Username   string `query:"username" required:"false"`
	Status     int    `query:"status" required:"false"`
	Page       int    `query:"page" default:"1"`
	Size       int    `query:"size" default:"20"`
}

type endUserListOutput struct {
	Body httpx.Page[endUserItem]
}

type createEndUserInput struct {
	Body struct {
		Username     string  `json:"username"`
		Email        *string `json:"email" required:"false"`
		Phone        *string `json:"phone" required:"false"`
		InternalNote string  `json:"internalNote,omitempty" maxLength:"500"`
	}
}

type updateEndUserInput struct {
	ID   string `path:"id"`
	Body struct {
		Email        *string `json:"email" required:"false" maxLength:"254"`
		Phone        *string `json:"phone" required:"false" maxLength:"32"`
		InternalNote *string `json:"internalNote" required:"false" maxLength:"500"`
	}
}

type createEndUserOutput struct {
	Body struct {
		UserID              string `json:"userId"`
		TenantID            string `json:"tenantId"`
		Username            string `json:"username"`
		ActivationToken     string `json:"activationToken"`
		ActivationExpiresIn int64  `json:"activationExpiresIn"`
	}
}

// registerAdminEndUsers 注册终端用户管理（系统管理员跨租户 / 租户用户限本租户）。
func registerAdminEndUsers(api huma.API, d Deps) {
	h := newAdminHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysOrTenant := huma.Middlewares{ua, requireUserType(api, 1, 2, 3)}
	sysOrTenantSensitive := huma.Middlewares{ua, requireUserType(api, 1, 2, 3), requireRecentAuth(api, d.RecentAuth)}
	tenantOnlySensitive := huma.Middlewares{ua, requireUserType(api, 3), requireRecentAuth(api, d.RecentAuth)}

	huma.Register(api, huma.Operation{OperationID: "admin-list-end-users", Method: http.MethodGet, Path: "/api/v1/users",
		Summary: "终端用户列表", Tags: []string{"admin-end-users"}, Middlewares: sysOrTenant}, h.listEndUsers)
	huma.Register(api, huma.Operation{OperationID: "admin-create-end-user", Method: http.MethodPost, Path: "/api/v1/users",
		Summary: "创建终端用户（租户）", Tags: []string{"admin-end-users"}, Middlewares: tenantOnlySensitive, DefaultStatus: http.StatusCreated}, h.createEndUser)
	huma.Register(api, huma.Operation{OperationID: "admin-update-end-user", Method: http.MethodPatch, Path: "/api/v1/users/{id}",
		Summary: "更新终端用户资料（租户）", Tags: []string{"admin-end-users"}, Middlewares: tenantOnlySensitive}, h.updateEndUser)
	huma.Register(api, huma.Operation{OperationID: "admin-update-end-user-status", Method: http.MethodPatch, Path: "/api/v1/users/{id}/status",
		Summary: "启用/停用终端用户", Tags: []string{"admin-end-users"}, Middlewares: sysOrTenantSensitive}, h.updateEndUserStatus)
	huma.Register(api, huma.Operation{OperationID: "admin-reset-end-user-password", Method: http.MethodPost, Path: "/api/v1/users/{id}/reset-password",
		Summary: "重置终端用户密码", Tags: []string{"admin-end-users"}, Middlewares: sysOrTenantSensitive}, h.resetEndUserPassword)
	huma.Register(api, huma.Operation{OperationID: "admin-delete-end-user", Method: http.MethodDelete, Path: "/api/v1/users/{id}",
		Summary: "删除终端用户", Tags: []string{"admin-end-users"}, Middlewares: sysOrTenantSensitive}, h.deleteEndUser)
}

// checkUserBelongsToTenant 校验 userID 归属 callerTenantID（空=管理员跳过）。
func (h *adminHandlers) checkUserBelongsToTenant(ctx context.Context, userID, callerTenantID string) error {
	tenantID, err := h.tenantRepo.GetEndUserTenantID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	if callerTenantID != "" && tenantID != callerTenantID {
		return httpx.ErrForbidden.WithDetail("无权操作")
	}
	return nil
}

func (h *adminHandlers) listEndUsers(ctx context.Context, in *listEndUsersInput) (*endUserListOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	// 租户用户强制本租户；管理员用查询参数
	tenantID := in.TenantID
	if !isAdminClaims(claims) {
		tenantID = claims.TenantID
	}

	where := "WHERE eu.user_type = 4 AND eu.status <> 'deleted'"
	args := []any{}
	idx := 1
	if tenantID != "" {
		where += fmt.Sprintf(" AND eu.tenant_id = $%d", idx)
		args = append(args, tenantID)
		idx++
	}
	if in.Keyword != "" {
		where += fmt.Sprintf(" AND (eu.username LIKE $%d OR eu.email LIKE $%d OR eu.phone LIKE $%d OR eu.internal_note LIKE $%d)", idx, idx, idx, idx)
		p := "%" + in.Keyword + "%"
		args = append(args, p)
		idx++
	}
	// V1 终端用户独立搜索条件：租户名 / 用户名 / 状态
	if in.TenantName != "" {
		where += fmt.Sprintf(" AND t.tenant_name LIKE $%d", idx)
		args = append(args, "%"+in.TenantName+"%")
		idx++
	}
	if in.Username != "" {
		where += fmt.Sprintf(" AND eu.username LIKE $%d", idx)
		args = append(args, "%"+in.Username+"%")
		idx++
	}
	if in.Status != 0 {
		where += fmt.Sprintf(" AND eu.status = $%d", idx)
		args = append(args, adminUserStatusFromInt(in.Status))
		idx++
	}

	// COUNT 与数据查询统一带上 iam_tenants 连接，使 tenant_name 过滤可用。
	from := "FROM iam_accounts eu LEFT JOIN iam_tenants t ON eu.tenant_id = t.tenant_id"

	var total int64
	_ = h.pool.QueryRow(ctx, "SELECT COUNT(*) "+from+" "+where, args...).Scan(&total)

	size := in.Size
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (in.Page - 1) * size
	qargs := append(append([]any{}, args...), size, offset)
	rows, err := h.pool.Query(ctx, fmt.Sprintf(`
		SELECT eu.user_id, eu.tenant_id, eu.username, eu.email, eu.phone, eu.internal_note, eu.nickname, eu.avatar,
		       eu.status, eu.credential_state, eu.last_login_at, eu.created_at,
		       COALESCE(t.tenant_name, '') AS tenant_name,
		       COALESCE((SELECT b.balance_micro FROM bill_accounts b WHERE b.account_id = eu.user_id), 0) AS credits
		%s
		%s ORDER BY eu.created_at DESC LIMIT $%d OFFSET $%d
	`, from, where, idx, idx+1), qargs...)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	defer rows.Close()

	items := make([]endUserItem, 0)
	for rows.Next() {
		var it endUserItem
		var creditsMicro int64
		var email, phone, nickname, avatar, tenantName *string
		var internalNote string
		var status, credentialState string
		var lastLogin *time.Time
		var createdAt time.Time
		if err := rows.Scan(&it.UserID, &it.TenantID, &it.Username, &email, &phone, &internalNote, &nickname, &avatar,
			&status, &credentialState, &lastLogin, &createdAt, &tenantName, &creditsMicro); err != nil {
			continue
		}
		it.BalanceUSD = billingdomain.MicroToUSD(creditsMicro)
		it.Status = adminUserStatusToInt(status)
		it.CredentialState = credentialState
		it.CreatedTime = millisFromTime(createdAt)
		if tenantName != nil {
			it.TenantName = *tenantName
		}
		if email != nil {
			it.Email = *email
		}
		if phone != nil {
			it.Phone = *phone
		}
		it.InternalNote = internalNote
		if nickname != nil {
			it.Nickname = *nickname
		}
		if avatar != nil {
			it.Avatar = *avatar
		}
		if lastLogin != nil {
			v := millisFromTime(*lastLogin)
			it.LastLoginTime = &v
		}
		items = append(items, it)
	}
	return &endUserListOutput{Body: httpx.NewPage(items, total, in.Page, size)}, nil
}

func (h *adminHandlers) createEndUser(ctx context.Context, in *createEndUserInput) (*createEndUserOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil || claims.TenantID == "" {
		return nil, httpx.ErrForbidden.WithDetail("需要租户用户身份")
	}
	username := auth.NormalizeUsername(in.Body.Username)
	if username == "" {
		return nil, httpx.ErrBadRequest.WithDetail("用户名不能为空")
	}
	var exists int
	if err := h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_accounts WHERE lower(username) = lower($1)`, username).Scan(&exists); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if exists > 0 {
		return nil, httpx.ErrConflict.WithDetail("用户名已被占用，请换一个")
	}
	credential, err := h.activations.NewCredential()
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	userID := "U_" + strings.ToUpper(uuid.NewString()[:24])
	now := time.Now().UTC()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, credential_state, email, phone, internal_note, user_type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'pending_activation', $5, $6, $7, 4, 'active', $8, $8)
	`, userID, claims.TenantID, username, credential.PasswordHash, in.Body.Email, in.Body.Phone, strings.TrimSpace(in.Body.InternalNote), now); err != nil {
		if authpg.IsUsernameTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("用户名已被占用，请换一个")
		}
		if authpg.IsEmailTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("邮箱已被使用")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if err := h.activations.Store(ctx, tx, userID, auth.ActivationPurposeAccount, credential); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	out := &createEndUserOutput{}
	out.Body.UserID = userID
	out.Body.TenantID = claims.TenantID
	out.Body.Username = username
	out.Body.ActivationToken = credential.Token
	out.Body.ActivationExpiresIn = int64(time.Until(credential.ExpiresAt).Seconds())
	return out, nil
}

func (h *adminHandlers) updateEndUser(ctx context.Context, in *updateEndUserInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil || claims.TenantID == "" {
		return nil, httpx.ErrForbidden.WithDetail("需要租户用户身份")
	}
	if err := h.checkUserBelongsToTenant(ctx, in.ID, claims.TenantID); err != nil {
		return nil, err
	}

	emailSet, email := normalizedOptionalText(in.Body.Email)
	phoneSet, phone := normalizedOptionalText(in.Body.Phone)
	noteSet, internalNote := normalizedOptionalText(in.Body.InternalNote)
	if !emailSet && !phoneSet && !noteSet {
		return nil, httpx.ErrBadRequest.WithDetail("至少提供一个需要更新的字段")
	}

	result, err := h.pool.Exec(ctx, `
		UPDATE iam_accounts
		SET email = CASE WHEN $1 THEN NULLIF($2, '') ELSE email END,
		    phone = CASE WHEN $3 THEN NULLIF($4, '') ELSE phone END,
		    internal_note = CASE WHEN $5 THEN $6 ELSE internal_note END,
		    updated_at = $7
		WHERE user_id = $8 AND tenant_id = $9 AND user_type = 4 AND status <> 'deleted'
	`, emailSet, email, phoneSet, phone, noteSet, internalNote, time.Now().UTC(), in.ID, claims.TenantID)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if result.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	out := &messageOutput{}
	out.Body.Message = "用户资料已更新"
	return out, nil
}

func normalizedOptionalText(value *string) (bool, string) {
	if value == nil {
		return false, ""
	}
	return true, strings.TrimSpace(*value)
}

func (h *adminHandlers) updateEndUserStatus(ctx context.Context, in *statusPathInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	callerTenantID := ""
	if !isAdminClaims(claims) {
		callerTenantID = claims.TenantID
	}
	if err := h.checkUserBelongsToTenant(ctx, in.ID, callerTenantID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result, err := h.pool.Exec(ctx, `UPDATE iam_accounts SET status = $1, updated_at = $2 WHERE user_id = $3 AND user_type = 4`, in.Body.Status, now, in.ID)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if result.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if h.blacklist != nil {
		if in.Body.Status == "disabled" {
			_ = h.blacklist.BanUser(ctx, in.ID)
		} else {
			_ = h.blacklist.UnbanUser(ctx, in.ID)
		}
	}
	out := &messageOutput{}
	out.Body.Message = "用户状态已更新"
	return out, nil
}

func (h *adminHandlers) resetEndUserPassword(ctx context.Context, in *tenantIDInput) (*activationCredentialOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	callerTenantID := ""
	if !isAdminClaims(claims) {
		callerTenantID = claims.TenantID
	}
	if err := h.checkUserBelongsToTenant(ctx, in.ID, callerTenantID); err != nil {
		return nil, err
	}
	result, err := h.activations.Reset(ctx, in.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if h.blacklist != nil {
		_ = h.blacklist.LogoutUser(in.ID)
	}
	out := &activationCredentialOutput{}
	setActivationOutput(out, result)
	return out, nil
}

// deleteEndUser removes an end user from operational views while retaining
// financial and audit history keyed by user_id. A balance or debt must be
// settled first so deletion cannot strand value on an inaccessible account.
func (h *adminHandlers) deleteEndUser(ctx context.Context, in *tenantIDInput) (*successOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	callerTenantID := ""
	if !isAdminClaims(claims) {
		callerTenantID = claims.TenantID
	}
	if err := h.checkUserBelongsToTenant(ctx, in.ID, callerTenantID); err != nil {
		return nil, err
	}
	if h.blacklist == nil || !h.blacklist.IsEnabled() {
		return nil, httpx.ErrUnavailable.WithDetail("删除用户需要可用的会话封禁服务")
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM iam_accounts
		WHERE user_id = $1 AND user_type = 4
		FOR UPDATE
	`, in.ID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) || status == "deleted" {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}

	// 一个数字就能表达两条规则：欠费不能删（会赖掉账），有余额也不能删（会吞掉钱）。
	var balanceMicroUSD int64
	if err := tx.QueryRow(ctx, `
		SELECT balance_micro FROM bill_accounts WHERE account_id = $1 FOR UPDATE
	`, in.ID).Scan(&balanceMicroUSD); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if balanceMicroUSD < 0 {
		return nil, httpx.ErrConflict.WithDetail("用户仍有未结清欠费，不能删除")
	}
	if balanceMicroUSD > 0 {
		return nil, httpx.ErrConflict.WithDetail("用户仍有可用 USD 余额，不能删除")
	}

	now := time.Now().UTC()
	if err := h.blacklist.BanUser(ctx, in.ID); err != nil {
		return nil, httpx.ErrUnavailable.WithCause(err)
	}
	result, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET status = 'deleted', updated_at = $1
		WHERE user_id = $2 AND user_type = 4 AND status <> 'deleted'
	`, now, in.ID)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if result.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return okSuccess(), nil
}
