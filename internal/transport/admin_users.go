package transport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	billingdomain "xiaodou/dai/internal/billing"
	"xiaodou/dai/libs/go/httpx"
)

// ---- DTO ----

type adminUserItem struct {
	UserID      string `json:"userId"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	Status      int    `json:"status"`
	StatusText  string `json:"statusText"`
	CreatedTime int64  `json:"createdTime"`
}

type listSystemAdminsInput struct {
	Page    int    `query:"page" default:"1"`
	Size    int    `query:"size" default:"20"`
	Keyword string `query:"keyword" required:"false"`
}

type adminUserListOutput struct {
	Body httpx.Page[adminUserItem]
}

type createSystemAdminInput struct {
	Body struct {
		Username string `json:"username"`
		Email    string `json:"email" required:"false"`
		Password string `json:"password" required:"false"`
	}
}

type createUserOutput struct {
	Body struct {
		UserID          string `json:"userId"`
		Username        string `json:"username"`
		DefaultPassword bool   `json:"defaultPassword"`
	}
}

type updateSystemAdminInput struct {
	ID   string `path:"id"`
	Body struct {
		Email    string `json:"email" required:"false"`
		Status   int    `json:"status" required:"false"`
		Password string `json:"password" required:"false"`
	}
}

type listTenantUsersInput struct {
	TenantID string `query:"tenantId" doc:"目标租户 ID"`
	Page     int    `query:"page" default:"1"`
	Size     int    `query:"size" default:"20"`
	Keyword  string `query:"keyword" required:"false"`
}

type createTenantUserInput struct {
	Body struct {
		TenantID string `json:"tenantId" doc:"目标租户 ID"`
		Username string `json:"username"`
		Email    string `json:"email" required:"false"`
	}
}

type statusPathInput struct {
	ID   string `path:"id"`
	Body struct {
		Status string `json:"status" enum:"active,disabled"`
	}
}

type updateTenantUserInput struct {
	ID   string `path:"id"`
	Body struct {
		Email    string `json:"email" required:"false"`
		Status   int    `json:"status" required:"false"`
		Password string `json:"password" required:"false"`
	}
}

func millisFromTime(t time.Time) int64 { return t.UTC().UnixMilli() }

func adminUserStatusToInt(status string) int {
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

// adminUserStatusFromInt 是 adminUserStatusToInt 的逆映射，用于按状态筛选时把前端的
// 整型状态（1正常/2禁用/3锁定/4级联停用）转回数据库存储的字符串值。
func adminUserStatusFromInt(status int) string {
	switch status {
	case 2:
		return "disabled"
	case 3:
		return "locked"
	case 4:
		return "inherited_disabled"
	default:
		return "active"
	}
}

// registerAdminUsers 注册系统管理员（仅超管）与租户组织用户（系统管理员）端点。
func registerAdminUsers(api huma.API, d Deps) {
	h := newAdminHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	superAdmin := huma.Middlewares{ua, requireUserType(api, 1)}
	sysUser := huma.Middlewares{ua, requireUserType(api, 1, 2)}

	// 系统管理员
	huma.Register(api, huma.Operation{OperationID: "admin-list-system-admins", Method: http.MethodGet, Path: "/api/v1/system-admins",
		Summary: "系统管理员列表", Tags: []string{"admin-system-admins"}, Middlewares: superAdmin}, h.listSystemAdmins)
	huma.Register(api, huma.Operation{OperationID: "admin-create-system-admin", Method: http.MethodPost, Path: "/api/v1/system-admins",
		Summary: "创建平台管理员", Tags: []string{"admin-system-admins"}, Middlewares: superAdmin, DefaultStatus: http.StatusCreated}, h.createSystemAdmin)
	huma.Register(api, huma.Operation{OperationID: "admin-update-system-admin", Method: http.MethodPut, Path: "/api/v1/system-admins/{id}",
		Summary: "更新平台管理员", Tags: []string{"admin-system-admins"}, Middlewares: superAdmin}, h.updateSystemAdmin)
	huma.Register(api, huma.Operation{OperationID: "admin-delete-system-admin", Method: http.MethodDelete, Path: "/api/v1/system-admins/{id}",
		Summary: "删除平台管理员", Tags: []string{"admin-system-admins"}, Middlewares: superAdmin}, h.deleteSystemAdmin)

	// 租户组织用户
	huma.Register(api, huma.Operation{OperationID: "admin-list-tenant-users", Method: http.MethodGet, Path: "/api/v1/tenant-users",
		Summary: "租户用户列表", Tags: []string{"admin-tenant-users"}, Middlewares: sysUser}, h.listTenantUsers)
	huma.Register(api, huma.Operation{OperationID: "admin-create-tenant-user", Method: http.MethodPost, Path: "/api/v1/tenant-users",
		Summary: "创建租户用户", Tags: []string{"admin-tenant-users"}, Middlewares: sysUser, DefaultStatus: http.StatusCreated}, h.createTenantUser)
	huma.Register(api, huma.Operation{OperationID: "admin-update-tenant-user-status", Method: http.MethodPatch, Path: "/api/v1/tenant-users/{id}/status",
		Summary: "启用/停用租户用户", Tags: []string{"admin-tenant-users"}, Middlewares: sysUser}, h.updateTenantUserStatus)
	huma.Register(api, huma.Operation{OperationID: "admin-update-tenant-user", Method: http.MethodPut, Path: "/api/v1/tenant-users/{id}",
		Summary: "更新租户用户", Tags: []string{"admin-tenant-users"}, Middlewares: sysUser}, h.updateTenantUser)
	huma.Register(api, huma.Operation{OperationID: "admin-reset-tenant-user-password", Method: http.MethodPost, Path: "/api/v1/tenant-users/{id}/reset-password",
		Summary: "重置租户用户密码", Tags: []string{"admin-tenant-users"}, Middlewares: sysUser}, h.resetTenantUserPassword)
}

func (h *adminHandlers) listSystemAdmins(ctx context.Context, in *listSystemAdminsInput) (*adminUserListOutput, error) {
	where := "user_type = 2"
	params := []any{}
	idx := 1
	if in.Keyword != "" {
		where += fmt.Sprintf(" AND (username LIKE $%d OR email LIKE $%d)", idx, idx+1)
		p := "%" + in.Keyword + "%"
		params = append(params, p, p)
		idx += 2
	}
	var total int64
	_ = h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM iam_accounts WHERE "+where, params...).Scan(&total)

	offset := (in.Page - 1) * in.Size
	qp := append(append([]any{}, params...), in.Size, offset)
	rows, err := h.pool.Query(ctx,
		fmt.Sprintf("SELECT user_id, username, email, status, created_at FROM iam_accounts WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", where, idx, idx+1),
		qp...)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	defer rows.Close()

	items := make([]adminUserItem, 0)
	for rows.Next() {
		it, err := scanAdminUserRow(rows, func(s string) int {
			if s == "disabled" {
				return 2
			}
			return 1
		})
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		items = append(items, it)
	}
	return &adminUserListOutput{Body: httpx.NewPage(items, total, in.Page, in.Size)}, nil
}

func (h *adminHandlers) createSystemAdmin(ctx context.Context, in *createSystemAdminInput) (*createUserOutput, error) {
	username := auth.NormalizeUsername(in.Body.Username)
	if username == "" {
		return nil, httpx.ErrBadRequest.WithDetail("用户名不能为空")
	}
	var count int
	_ = h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM iam_accounts WHERE lower(username) = lower($1)", username).Scan(&count)
	if count > 0 {
		return nil, httpx.ErrConflict.WithDetail("用户名已存在")
	}
	pass := in.Body.Password
	if pass == "" {
		pass = "123456"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	userID := "SA_" + uuid.New().String()[:24]
	now := billingdomain.NowUTC()
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, email, user_type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 2, 'active', $5, $5)
	`, userID, username, string(hash), in.Body.Email, now); err != nil {
		if authpg.IsUsernameTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("用户名已存在")
		}
		if authpg.IsEmailTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("邮箱已被使用")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	out := &createUserOutput{}
	out.Body.UserID = userID
	out.Body.Username = username
	out.Body.DefaultPassword = pass == "123456"
	return out, nil
}

func (h *adminHandlers) updateSystemAdmin(ctx context.Context, in *updateSystemAdminInput) (*successOutput, error) {
	var userType int32
	if err := h.pool.QueryRow(ctx, "SELECT user_type FROM iam_accounts WHERE user_id = $1 AND user_type IN (1, 2)", in.ID).Scan(&userType); err != nil {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if userType != 2 {
		return nil, httpx.ErrForbidden.WithDetail("不允许修改超级管理员")
	}
	now := billingdomain.NowUTC()
	status := "active"
	if in.Body.Status == 2 {
		status = "disabled"
	}
	var err error
	if in.Body.Password != "" {
		var hash []byte
		if hash, err = bcrypt.GenerateFromPassword([]byte(in.Body.Password), bcrypt.DefaultCost); err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		_, err = h.pool.Exec(ctx, "UPDATE iam_accounts SET email = $1, status = $2, password_hash = $3, credential_version = credential_version + 1, updated_at = $4 WHERE user_id = $5 AND user_type = 2",
			in.Body.Email, status, string(hash), now, in.ID)
	} else {
		_, err = h.pool.Exec(ctx, "UPDATE iam_accounts SET email = $1, status = $2, updated_at = $3 WHERE user_id = $4 AND user_type = 2",
			in.Body.Email, status, now, in.ID)
	}
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if h.blacklist != nil {
		if status == "disabled" {
			_ = h.blacklist.BanUser(ctx, in.ID)
		} else {
			_ = h.blacklist.UnbanUser(ctx, in.ID)
			if in.Body.Password != "" {
				_ = h.blacklist.LogoutUser(in.ID)
			}
		}
	}
	return okSuccess(), nil
}

func (h *adminHandlers) deleteSystemAdmin(ctx context.Context, in *tenantIDInput) (*successOutput, error) {
	var userType int32
	if err := h.pool.QueryRow(ctx, "SELECT user_type FROM iam_accounts WHERE user_id = $1 AND user_type IN (1, 2)", in.ID).Scan(&userType); err != nil {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if userType != 2 {
		return nil, httpx.ErrForbidden.WithDetail("不允许删除超级管理员")
	}
	deleted, err := h.pool.Exec(ctx, `DELETE FROM iam_accounts WHERE user_id = $1 AND user_type = 2`, in.ID)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if deleted.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	return okSuccess(), nil
}

func (h *adminHandlers) listTenantUsers(ctx context.Context, in *listTenantUsersInput) (*adminUserListOutput, error) {
	if in.TenantID == "" {
		return nil, httpx.ErrBadRequest.WithDetail("tenantId 必填")
	}
	var total int64
	offset := (in.Page - 1) * in.Size
	countSQL := `SELECT COUNT(*) FROM iam_accounts WHERE tenant_id = $1 AND user_type = 3`
	querySQL := `SELECT user_id, username, email, status, created_at FROM iam_accounts WHERE tenant_id = $1 AND user_type = 3 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	countArgs := []any{in.TenantID}
	queryArgs := []any{in.TenantID, in.Size, offset}
	if in.Keyword != "" {
		kw := "%" + in.Keyword + "%"
		countSQL = `SELECT COUNT(*) FROM iam_accounts WHERE tenant_id = $1 AND user_type = 3 AND (username ILIKE $2 OR email ILIKE $2)`
		querySQL = `SELECT user_id, username, email, status, created_at FROM iam_accounts WHERE tenant_id = $1 AND user_type = 3 AND (username ILIKE $2 OR email ILIKE $2) ORDER BY created_at DESC LIMIT $3 OFFSET $4`
		countArgs = []any{in.TenantID, kw}
		queryArgs = []any{in.TenantID, kw, in.Size, offset}
	}
	_ = h.pool.QueryRow(ctx, countSQL, countArgs...).Scan(&total)
	rows, err := h.pool.Query(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	defer rows.Close()
	items := make([]adminUserItem, 0)
	for rows.Next() {
		it, err := scanAdminUserRow(rows, adminUserStatusToInt)
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		items = append(items, it)
	}
	return &adminUserListOutput{Body: httpx.NewPage(items, total, in.Page, in.Size)}, nil
}

func (h *adminHandlers) createTenantUser(ctx context.Context, in *createTenantUserInput) (*createUserOutput, error) {
	username := auth.NormalizeUsername(in.Body.Username)
	if username == "" {
		return nil, httpx.ErrBadRequest.WithDetail("用户名不能为空")
	}
	var tenantExists int
	_ = h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM iam_tenants WHERE tenant_id = $1", in.Body.TenantID).Scan(&tenantExists)
	if tenantExists == 0 {
		return nil, httpx.ErrBadRequest.WithDetail("目标租户不存在")
	}
	var exists int
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_accounts WHERE lower(username) = lower($1)`, username).Scan(&exists)
	if exists > 0 {
		return nil, httpx.ErrConflict.WithDetail("用户名已被占用，请换一个")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	userID := "TU_" + uuid.New().String()[:24]
	now := billingdomain.NowUTC()
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, user_type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 3, 'active', $6, $6)
	`, userID, in.Body.TenantID, username, string(hash), in.Body.Email, now); err != nil {
		if authpg.IsUsernameTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("用户名已被占用，请换一个")
		}
		if authpg.IsEmailTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("邮箱已被使用")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	out := &createUserOutput{}
	out.Body.UserID = userID
	out.Body.Username = username
	out.Body.DefaultPassword = true
	return out, nil
}

func (h *adminHandlers) updateTenantUserStatus(ctx context.Context, in *statusPathInput) (*successOutput, error) {
	now := billingdomain.NowUTC()
	result, err := h.pool.Exec(ctx, `UPDATE iam_accounts SET status = $1, updated_at = $2 WHERE user_id = $3 AND user_type = 3`, in.Body.Status, now, in.ID)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if result.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	// 停用立即失效该用户所有 token 并标记封禁；启用时清除封禁标记
	if h.blacklist != nil {
		if in.Body.Status == "disabled" {
			_ = h.blacklist.BanUser(ctx, in.ID)
		} else {
			_ = h.blacklist.UnbanUser(ctx, in.ID)
		}
	}
	return okSuccess(), nil
}

func (h *adminHandlers) updateTenantUser(ctx context.Context, in *updateTenantUserInput) (*successOutput, error) {
	now := billingdomain.NowUTC()
	status := "active"
	if in.Body.Status == 2 {
		status = "disabled"
	}
	var err error
	if in.Body.Password != "" {
		var hash []byte
		if hash, err = bcrypt.GenerateFromPassword([]byte(in.Body.Password), bcrypt.DefaultCost); err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		_, err = h.pool.Exec(ctx, "UPDATE iam_accounts SET email = $1, status = $2, password_hash = $3, credential_version = credential_version + 1, updated_at = $4 WHERE user_id = $5 AND user_type = 3",
			in.Body.Email, status, string(hash), now, in.ID)
	} else {
		_, err = h.pool.Exec(ctx, "UPDATE iam_accounts SET email = $1, status = $2, updated_at = $3 WHERE user_id = $4 AND user_type = 3",
			in.Body.Email, status, now, in.ID)
	}
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	// 停用时强制下线并标记封禁；启用时清除封禁标记
	if h.blacklist != nil {
		if status == "disabled" {
			_ = h.blacklist.BanUser(ctx, in.ID)
		} else {
			_ = h.blacklist.UnbanUser(ctx, in.ID)
			if in.Body.Password != "" {
				_ = h.blacklist.LogoutUser(in.ID)
			}
		}
	}
	return okSuccess(), nil
}

func (h *adminHandlers) resetTenantUserPassword(ctx context.Context, in *tenantIDInput) (*successOutput, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	now := billingdomain.NowUTC()
	result, err := h.pool.Exec(ctx, `UPDATE iam_accounts SET password_hash = $1, credential_version = credential_version + 1, updated_at = $2 WHERE user_id = $3 AND user_type = 3`, string(hash), now, in.ID)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if result.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if h.blacklist != nil {
		_ = h.blacklist.LogoutUser(in.ID)
	}
	return okSuccess(), nil
}

// scanAdminUserRow 扫描 (user_id, username, email, status, created_at) 一行，
// statusToInt 把存储状态映射为前端整型。
func scanAdminUserRow(rows interface {
	Scan(...any) error
}, statusToInt func(string) int) (adminUserItem, error) {
	var userID, username, status string
	var email *string
	var createdAt time.Time
	if err := rows.Scan(&userID, &username, &email, &status, &createdAt); err != nil {
		return adminUserItem{}, err
	}
	it := adminUserItem{
		UserID: userID, Username: username,
		Status: statusToInt(status), CreatedTime: millisFromTime(createdAt),
	}
	if email != nil {
		it.Email = *email
	}
	it.StatusText = "正常"
	if it.Status != 1 {
		it.StatusText = "停用"
	}
	return it, nil
}
