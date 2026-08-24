package transport

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	userports "xiaodou/dai/internal/user/ports"
	"xiaodou/dai/libs/go/httpx"
)

// ---- DTO ----

type adminUserItem struct {
	UserID          string `json:"userId"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	Status          int    `json:"status"`
	StatusText      string `json:"statusText"`
	CredentialState string `json:"credentialState"`
	CreatedTime     int64  `json:"createdTime"`
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
	}
}

type createUserOutput struct {
	Body struct {
		UserID              string `json:"userId"`
		Username            string `json:"username"`
		ActivationToken     string `json:"activationToken"`
		ActivationExpiresIn int64  `json:"activationExpiresIn"`
	}
}

type updateSystemAdminInput struct {
	ID   string `path:"id"`
	Body struct {
		Email  string `json:"email" required:"false"`
		Status int    `json:"status" required:"false"`
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
		Email  string `json:"email" required:"false"`
		Status int    `json:"status" required:"false"`
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
	superAdminSensitive := huma.Middlewares{ua, requireUserType(api, 1), requireRecentAuth(api, d.RecentAuth)}
	sysUserSensitive := huma.Middlewares{ua, requireUserType(api, 1, 2), requireRecentAuth(api, d.RecentAuth)}

	// 系统管理员
	huma.Register(api, huma.Operation{OperationID: "admin-list-system-admins", Method: http.MethodGet, Path: "/api/v1/system-admins",
		Summary: "系统管理员列表", Tags: []string{"admin-system-admins"}, Middlewares: superAdmin}, h.listSystemAdmins)
	huma.Register(api, huma.Operation{OperationID: "admin-create-system-admin", Method: http.MethodPost, Path: "/api/v1/system-admins",
		Summary: "创建平台管理员", Tags: []string{"admin-system-admins"}, Middlewares: superAdminSensitive, DefaultStatus: http.StatusCreated}, h.createSystemAdmin)
	huma.Register(api, huma.Operation{OperationID: "admin-update-system-admin", Method: http.MethodPut, Path: "/api/v1/system-admins/{id}",
		Summary: "更新平台管理员", Tags: []string{"admin-system-admins"}, Middlewares: superAdminSensitive}, h.updateSystemAdmin)
	huma.Register(api, huma.Operation{OperationID: "admin-delete-system-admin", Method: http.MethodDelete, Path: "/api/v1/system-admins/{id}",
		Summary: "删除平台管理员", Tags: []string{"admin-system-admins"}, Middlewares: superAdminSensitive}, h.deleteSystemAdmin)
	huma.Register(api, huma.Operation{OperationID: "admin-reset-system-admin-password", Method: http.MethodPost, Path: "/api/v1/system-admins/{id}/reset-password",
		Summary: "签发平台管理员密码重置凭证", Tags: []string{"admin-system-admins"}, Middlewares: superAdminSensitive}, h.resetSystemAdminPassword)

	// 租户组织用户
	huma.Register(api, huma.Operation{OperationID: "admin-list-tenant-users", Method: http.MethodGet, Path: "/api/v1/tenant-users",
		Summary: "租户用户列表", Tags: []string{"admin-tenant-users"}, Middlewares: sysUser}, h.listTenantUsers)
	huma.Register(api, huma.Operation{OperationID: "admin-create-tenant-user", Method: http.MethodPost, Path: "/api/v1/tenant-users",
		Summary: "创建租户用户", Tags: []string{"admin-tenant-users"}, Middlewares: sysUserSensitive, DefaultStatus: http.StatusCreated}, h.createTenantUser)
	huma.Register(api, huma.Operation{OperationID: "admin-update-tenant-user-status", Method: http.MethodPatch, Path: "/api/v1/tenant-users/{id}/status",
		Summary: "启用/停用租户用户", Tags: []string{"admin-tenant-users"}, Middlewares: sysUserSensitive}, h.updateTenantUserStatus)
	huma.Register(api, huma.Operation{OperationID: "admin-update-tenant-user", Method: http.MethodPut, Path: "/api/v1/tenant-users/{id}",
		Summary: "更新租户用户", Tags: []string{"admin-tenant-users"}, Middlewares: sysUserSensitive}, h.updateTenantUser)
	huma.Register(api, huma.Operation{OperationID: "admin-reset-tenant-user-password", Method: http.MethodPost, Path: "/api/v1/tenant-users/{id}/reset-password",
		Summary: "重置租户用户密码", Tags: []string{"admin-tenant-users"}, Middlewares: sysUserSensitive}, h.resetTenantUserPassword)
}

func (h *adminHandlers) listSystemAdmins(ctx context.Context, in *listSystemAdminsInput) (*adminUserListOutput, error) {
	result, err := h.accountRepo.ListSystemAdmins(ctx, in.Keyword, in.Page, in.Size)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	items := make([]adminUserItem, 0, len(result.Records))
	for _, row := range result.Records {
		items = append(items, adminUserItemFromRecord(row, func(status string) int {
			if status == "disabled" {
				return 2
			}
			return 1
		}))
	}
	return &adminUserListOutput{Body: httpx.NewPage(items, result.Total, int(result.Page), int(result.Size))}, nil
}

func (h *adminHandlers) createSystemAdmin(ctx context.Context, in *createSystemAdminInput) (*createUserOutput, error) {
	username := auth.NormalizeUsername(in.Body.Username)
	if username == "" {
		return nil, httpx.ErrBadRequest.WithDetail("用户名不能为空")
	}
	credential, err := h.activations.NewCredential()
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	userID := "SA_" + uuid.New().String()[:24]
	if err := h.accountWriter.CreateSystemAdmin(ctx, userports.AdminAccountCreate{
		UserID:              userID,
		Username:            username,
		Email:               in.Body.Email,
		PasswordHash:        credential.PasswordHash,
		ActivationTokenHash: credential.TokenHash,
		ActivationExpiresAt: credential.ExpiresAt,
	}); err != nil {
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
	out.Body.ActivationToken = credential.Token
	out.Body.ActivationExpiresIn = int64(time.Until(credential.ExpiresAt).Seconds())
	return out, nil
}

func (h *adminHandlers) updateSystemAdmin(ctx context.Context, in *updateSystemAdminInput) (*successOutput, error) {
	status := "active"
	if in.Body.Status == 2 {
		status = "disabled"
	}
	result, err := h.accountWriter.UpdateSystemAdmin(ctx, userports.AdminAccountUpdate{
		UserID: in.ID,
		Email:  in.Body.Email,
		Status: status,
	})
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if result.Forbidden {
		return nil, httpx.ErrForbidden.WithDetail("不允许修改超级管理员")
	}
	if !result.Updated {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if h.blacklist != nil {
		if in.Body.Status == 2 {
			_ = h.blacklist.BanUser(ctx, in.ID)
		} else {
			_ = h.blacklist.UnbanUser(ctx, in.ID)
		}
	}
	return okSuccess(), nil
}

func (h *adminHandlers) resetSystemAdminPassword(ctx context.Context, in *tenantIDInput) (*activationCredentialOutput, error) {
	var userType int
	if err := h.pool.QueryRow(ctx, `SELECT user_type FROM iam_accounts WHERE user_id = $1 AND status <> 'deleted'`, in.ID).Scan(&userType); err != nil || userType != 2 {
		return nil, httpx.ErrNotFound.WithDetail("平台管理员不存在")
	}
	result, err := h.activations.Reset(ctx, in.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrNotFound.WithDetail("平台管理员不存在")
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
	result, err := h.accountRepo.ListTenantUsers(ctx, in.TenantID, in.Keyword, in.Page, in.Size)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	items := make([]adminUserItem, 0, len(result.Records))
	for _, row := range result.Records {
		items = append(items, adminUserItemFromRecord(row, adminUserStatusToInt))
	}
	return &adminUserListOutput{Body: httpx.NewPage(items, result.Total, int(result.Page), int(result.Size))}, nil
}

func (h *adminHandlers) createTenantUser(ctx context.Context, in *createTenantUserInput) (*createUserOutput, error) {
	username := auth.NormalizeUsername(in.Body.Username)
	if username == "" {
		return nil, httpx.ErrBadRequest.WithDetail("用户名不能为空")
	}
	if _, err := h.tenantRepo.GetTenantDetails(ctx, in.Body.TenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrBadRequest.WithDetail("目标租户不存在")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	credential, err := h.activations.NewCredential()
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	userID := "TU_" + uuid.New().String()[:24]
	if err := h.accountWriter.CreateTenantUser(ctx, userports.AdminAccountCreate{
		UserID:              userID,
		TenantID:            in.Body.TenantID,
		Username:            username,
		Email:               in.Body.Email,
		PasswordHash:        credential.PasswordHash,
		ActivationTokenHash: credential.TokenHash,
		ActivationExpiresAt: credential.ExpiresAt,
	}); err != nil {
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
	out.Body.ActivationToken = credential.Token
	out.Body.ActivationExpiresIn = int64(time.Until(credential.ExpiresAt).Seconds())
	return out, nil
}

func (h *adminHandlers) updateTenantUserStatus(ctx context.Context, in *statusPathInput) (*successOutput, error) {
	updated, err := h.accountWriter.UpdateTenantUserStatus(ctx, in.ID, in.Body.Status)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !updated {
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
	status := "active"
	if in.Body.Status == 2 {
		status = "disabled"
	}
	updated, err := h.accountWriter.UpdateTenantUser(ctx, userports.AdminAccountUpdate{
		UserID: in.ID,
		Email:  in.Body.Email,
		Status: status,
	})
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !updated {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	// 停用时强制下线并标记封禁；启用时清除封禁标记
	if h.blacklist != nil {
		if status == "disabled" {
			_ = h.blacklist.BanUser(ctx, in.ID)
		} else {
			_ = h.blacklist.UnbanUser(ctx, in.ID)
		}
	}
	return okSuccess(), nil
}

func (h *adminHandlers) resetTenantUserPassword(ctx context.Context, in *tenantIDInput) (*activationCredentialOutput, error) {
	var exists bool
	if err := h.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM iam_accounts
			WHERE user_id = $1 AND user_type = 3 AND status <> 'deleted'
		)
	`, in.ID).Scan(&exists); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !exists {
		return nil, httpx.ErrNotFound.WithDetail("租户用户不存在")
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

// adminUserItemFromRecord maps the repository's non-secret projection to the
// legacy admin DTO while keeping status presentation in the HTTP boundary.
func adminUserItemFromRecord(row userports.AdminAccountRow, statusToInt func(string) int) adminUserItem {
	it := adminUserItem{
		UserID: row.UserID, Username: row.Username,
		Status: statusToInt(row.Status), CredentialState: row.CredentialState, CreatedTime: millisFromTime(row.CreatedAt),
	}
	if row.Email != nil {
		it.Email = *row.Email
	}
	it.StatusText = "正常"
	if it.Status != 1 {
		it.StatusText = "停用"
	} else if row.CredentialState == "pending_activation" {
		it.StatusText = "待激活"
	}
	return it
}
