package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"xiaodou/dai/internal/auth"
	authports "xiaodou/dai/internal/auth/ports"
	billingdomain "xiaodou/dai/internal/billing"
	tenantports "xiaodou/dai/internal/tenant/ports"
	userports "xiaodou/dai/internal/user/ports"
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
	if h.tenantReader == nil {
		return httpx.ErrUnavailable.WithDetail("租户查询服务不可用")
	}
	tenantID, err := h.tenantReader.GetEndUserTenantID(ctx, userID)
	if errors.Is(err, tenantports.ErrTenantEndUserNotFound) {
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
	result, err := h.endUserRepo.ListEndUsers(ctx, userports.AdminEndUserListFilter{
		TenantID:   tenantID,
		TenantName: in.TenantName,
		Username:   in.Username,
		Keyword:    in.Keyword,
		Status:     statusFilterForAdminEndUsers(in.Status),
		Page:       in.Page,
		Size:       in.Size,
	})
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	items := make([]endUserItem, 0, len(result.Records))
	for _, row := range result.Records {
		items = append(items, endUserItemFromRecord(row))
	}
	return &endUserListOutput{Body: httpx.NewPage(items, result.Total, result.Page, result.Size)}, nil
}

func statusFilterForAdminEndUsers(status int) string {
	if status == 0 {
		return ""
	}
	return adminUserStatusFromInt(status)
}

func endUserItemFromRecord(row userports.AdminEndUserRow) endUserItem {
	item := endUserItem{
		UserID:          row.UserID,
		TenantID:        row.TenantID,
		Username:        row.Username,
		InternalNote:    row.InternalNote,
		Status:          adminUserStatusToInt(row.Status),
		CredentialState: row.CredentialState,
		BalanceUSD:      billingdomain.MicroToUSD(row.BalanceMicroUSD),
		CreatedTime:     millisFromTime(row.CreatedAt),
	}
	if row.TenantName != nil {
		item.TenantName = *row.TenantName
	}
	if row.Email != nil {
		item.Email = *row.Email
	}
	if row.Phone != nil {
		item.Phone = *row.Phone
	}
	if row.Nickname != nil {
		item.Nickname = *row.Nickname
	}
	if row.Avatar != nil {
		item.Avatar = *row.Avatar
	}
	if row.LastLoginAt != nil {
		value := millisFromTime(*row.LastLoginAt)
		item.LastLoginTime = &value
	}
	return item
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
	credential, err := h.activations.NewCredential()
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	userID := "U_" + strings.ToUpper(uuid.NewString()[:24])
	if err := h.endUserWriter.CreateEndUser(ctx, userports.AdminEndUserCreate{
		UserID:              userID,
		TenantID:            claims.TenantID,
		Username:            username,
		Email:               in.Body.Email,
		Phone:               in.Body.Phone,
		InternalNote:        strings.TrimSpace(in.Body.InternalNote),
		PasswordHash:        credential.PasswordHash,
		ActivationTokenHash: credential.TokenHash,
		ActivationExpiresAt: credential.ExpiresAt,
	}); err != nil {
		if errors.Is(err, authports.ErrUsernameTaken) {
			return nil, httpx.ErrConflict.WithDetail("用户名已被占用，请换一个")
		}
		if errors.Is(err, authports.ErrEmailTaken) {
			return nil, httpx.ErrConflict.WithDetail("邮箱已被使用")
		}
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

	updated, err := h.endUserWriter.UpdateEndUser(ctx, userports.AdminEndUserUpdate{
		UserID:          in.ID,
		TenantID:        claims.TenantID,
		EmailSet:        emailSet,
		Email:           email,
		PhoneSet:        phoneSet,
		Phone:           phone,
		InternalNoteSet: noteSet,
		InternalNote:    internalNote,
	})
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !updated {
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
	updated, err := h.endUserWriter.UpdateEndUserStatus(ctx, in.ID, in.Body.Status)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !updated {
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
	result, err := h.endUserWriter.ResetEndUserPassword(ctx, in.ID)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if result.Token == "" {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
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
	result, err := h.endUserWriter.DeleteEndUser(ctx, in.ID, func(ctx context.Context, userID string) error {
		return h.blacklist.BanUser(ctx, userID)
	})
	if err != nil {
		var guardErr *userports.AdminEndUserDeleteGuardError
		if errors.As(err, &guardErr) {
			return nil, httpx.ErrUnavailable.WithCause(guardErr.Cause)
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !result.Found {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	if result.BalanceMicroUSD < 0 {
		return nil, httpx.ErrConflict.WithDetail("用户仍有未结清欠费，不能删除")
	}
	if result.BalanceMicroUSD > 0 {
		return nil, httpx.ErrConflict.WithDetail("用户仍有可用 USD 余额，不能删除")
	}
	if !result.Deleted {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
	}
	return okSuccess(), nil
}
