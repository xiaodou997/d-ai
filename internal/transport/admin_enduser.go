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
func registerAdminEndUsers(api huma.API, d adminEndUsersModule) {
	h := newAdminEndUsersHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysOrTenant := huma.Middlewares{ua, requireAnyCapability(api, auth.CapabilityPlatformAdmin, auth.CapabilityTenantSelf)}
	sysOrTenantSensitive := huma.Middlewares{ua, requireAnyCapability(api, auth.CapabilityPlatformAdmin, auth.CapabilityTenantSelf), requireRecentAuth(api, d.RecentAuth)}
	tenantOnlySensitive := huma.Middlewares{ua, requireCapability(api, auth.CapabilityTenantSelf), requireRecentAuth(api, d.RecentAuth)}

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

func (h *adminHandlers) listEndUsers(ctx context.Context, in *listEndUsersInput) (*endUserListOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	// 租户用户强制本租户；管理员用查询参数
	tenantID := in.TenantID
	if !actorFromClaims(claims).Has(auth.CapabilityPlatformAdmin) {
		tenantID = string(actorFromClaims(claims).TenantID)
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
		if errors.Is(err, userports.ErrTenantNotFound) {
			return nil, httpx.ErrBadRequest.WithDetail("目标租户不存在或已失效")
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
	emailSet, email := normalizedOptionalText(in.Body.Email)
	phoneSet, phone := normalizedOptionalText(in.Body.Phone)
	noteSet, internalNote := normalizedOptionalText(in.Body.InternalNote)
	if !emailSet && !phoneSet && !noteSet {
		return nil, httpx.ErrBadRequest.WithDetail("至少提供一个需要更新的字段")
	}

	if h.endUserLifecycle == nil {
		return nil, httpx.ErrUnavailable.WithDetail("终端用户服务不可用")
	}
	updated, err := h.endUserLifecycle.UpdateEndUser(ctx, userports.AdminEndUserUpdate{
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
		return nil, adminEndUserLifecycleError(err)
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
	if h.endUserLifecycle == nil {
		return nil, httpx.ErrUnavailable.WithDetail("终端用户服务不可用")
	}
	updated, err := h.endUserLifecycle.UpdateEndUserStatus(ctx, userports.AdminEndUserStatusUpdate{
		UserID: in.ID, TenantID: adminEndUserTenantScope(actorFromClaims(claims)), Status: in.Body.Status,
	})
	if err != nil {
		return nil, adminEndUserLifecycleError(err)
	}
	if !updated {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
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
	if h.endUserLifecycle == nil {
		return nil, httpx.ErrUnavailable.WithDetail("终端用户服务不可用")
	}
	result, err := h.endUserLifecycle.ResetEndUserPassword(ctx, userports.AdminEndUserPasswordReset{
		UserID: in.ID, TenantID: adminEndUserTenantScope(actorFromClaims(claims)),
	})
	if err != nil {
		return nil, adminEndUserLifecycleError(err)
	}
	if result.Token == "" {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
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
	if h.endUserLifecycle == nil {
		return nil, httpx.ErrUnavailable.WithDetail("终端用户服务不可用")
	}
	result, err := h.endUserLifecycle.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{
		UserID:   in.ID,
		TenantID: adminEndUserTenantScope(actorFromClaims(claims)),
	})
	if err != nil {
		return nil, adminEndUserLifecycleError(err)
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

// adminEndUserTenantScope returns the tenant predicate for a mutation. A
// platform administrator has global scope; tenant users must carry their
// claim into the repository so ownership is enforced by the write itself.
func adminEndUserTenantScope(actor auth.Actor) string {
	if actor.Has(auth.CapabilityPlatformAdmin) {
		return ""
	}
	return string(actor.TenantID)
}
