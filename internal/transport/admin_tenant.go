package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	util "xiaodou/dai/internal/domain"
	tenantpg "xiaodou/dai/internal/tenant/pg"
	tenantports "xiaodou/dai/internal/tenant/ports"
	"xiaodou/dai/libs/go/httpx"
)

// ---- DTO ----

type tenantListItem struct {
	TenantID      string  `json:"tenantId"`
	TenantName    string  `json:"tenantName"`
	ContactPerson string  `json:"contactPerson"`
	ContactEmail  string  `json:"contactEmail"`
	Status        int     `json:"status"`
	StatusDisplay string  `json:"statusDisplay"`
	CreatedTime   int64   `json:"createdTime"`
	BalanceUSD    float64 `json:"balanceUsd"`
	UserCount     int64   `json:"userCount"`
}

type tenantListInput struct {
	Page    int    `query:"page" default:"1"`
	Size    int    `query:"size" default:"10"`
	Keyword string `query:"keyword" required:"false"`
	Status  int    `query:"status" required:"false" doc:"1=启用 2=停用 3=欠费封禁，0/省略表示全部"`
}

type tenantListOutput struct {
	Body httpx.Page[tenantListItem]
}

type createTenantInput struct {
	Body struct {
		TenantName    string `json:"tenantName"`
		ContactPerson string `json:"contactPerson" required:"false"`
		ContactEmail  string `json:"contactEmail" required:"false"`
		Status        int    `json:"status" required:"false"`
		InitUsername  string `json:"initUsername" required:"false"`
		InitEmail     string `json:"initEmail" required:"false"`
	}
}

type createTenantOutput struct {
	Body struct {
		TenantID            string `json:"tenantId"`
		InitUserID          string `json:"initUserId,omitempty"`
		InitUsername        string `json:"initUsername,omitempty"`
		ActivationToken     string `json:"activationToken,omitempty"`
		ActivationExpiresIn int64  `json:"activationExpiresIn,omitempty"`
	}
}

type tenantIDInput struct {
	ID string `path:"id"`
}

type tenantDetailOutput struct {
	Body struct {
		TenantID      string `json:"tenantId"`
		TenantName    string `json:"tenantName"`
		ContactPerson string `json:"contactPerson,omitempty"`
		ContactEmail  string `json:"contactEmail,omitempty"`
		Status        int    `json:"status"`
		StatusDisplay string `json:"statusDisplay"`
		CreatedTime   int64  `json:"createdTime"`
	}
}

type updateTenantInput struct {
	ID   string `path:"id"`
	Body struct {
		TenantName    string `json:"tenantName"`
		ContactPerson string `json:"contactPerson" required:"false"`
		ContactEmail  string `json:"contactEmail" required:"false"`
		Status        int    `json:"status" required:"false"`
	}
}

type updateTenantStatusInput struct {
	ID   string `path:"id"`
	Body struct {
		Status string `json:"status" enum:"active,disabled" doc:"active 或 disabled"`
	}
}

// registerAdminTenants 注册 /api/v1/tenants 租户 CRUD。
func registerAdminTenants(api huma.API, d Deps) {
	h := newAdminHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireUserType(api, 1, 2)}
	sysOrTenant := huma.Middlewares{ua, requireUserType(api, 1, 2, 3)}
	sysUserSensitive := huma.Middlewares{ua, requireUserType(api, 1, 2), requireRecentAuth(api, d.RecentAuth)}

	huma.Register(api, huma.Operation{
		OperationID: "admin-list-tenants", Method: http.MethodGet, Path: "/api/v1/tenants",
		Summary: "租户列表", Tags: []string{"admin-tenants"}, Middlewares: sysUser,
	}, h.listTenants)

	huma.Register(api, huma.Operation{
		OperationID: "admin-create-tenant", Method: http.MethodPost, Path: "/api/v1/tenants",
		Summary: "创建租户", Tags: []string{"admin-tenants"}, Middlewares: sysUserSensitive, DefaultStatus: http.StatusCreated,
	}, h.createTenant)

	huma.Register(api, huma.Operation{
		OperationID: "admin-get-tenant", Method: http.MethodGet, Path: "/api/v1/tenants/{id}",
		Summary: "租户详情", Tags: []string{"admin-tenants"}, Middlewares: sysOrTenant,
	}, h.getTenant)

	huma.Register(api, huma.Operation{
		OperationID: "admin-update-tenant", Method: http.MethodPut, Path: "/api/v1/tenants/{id}",
		Summary: "更新租户", Tags: []string{"admin-tenants"}, Middlewares: sysUserSensitive,
	}, h.updateTenant)

	huma.Register(api, huma.Operation{
		OperationID: "admin-delete-tenant", Method: http.MethodDelete, Path: "/api/v1/tenants/{id}",
		Summary: "删除租户", Tags: []string{"admin-tenants"}, Middlewares: sysUserSensitive,
	}, h.deleteTenant)

	huma.Register(api, huma.Operation{
		OperationID: "admin-update-tenant-status", Method: http.MethodPatch, Path: "/api/v1/tenants/{id}/status",
		Summary: "启用/停用租户（级联）", Tags: []string{"admin-tenants"}, Middlewares: sysUserSensitive,
	}, h.updateTenantStatus)
}

func (h *adminHandlers) listTenants(ctx context.Context, in *tenantListInput) (*tenantListOutput, error) {
	var statusFilter string
	if in.Status != 0 {
		statusFilter = adminStatusFromInt(in.Status)
	}
	result, err := h.tenantRepo.List(ctx, tenantpg.ListTenantsParams{
		Keyword:    in.Keyword,
		Status:     statusFilter,
		Pagination: tenantpg.NewPagination(int64(in.Page), int64(in.Size)),
	})
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	items := make([]tenantListItem, 0, len(result.Records))
	for _, row := range result.Records {
		status := 1
		if row.Status != nil {
			status = adminTenantStatusToInt(*row.Status)
		}
		item := tenantListItem{
			TenantID: row.TenantID, TenantName: row.TenantName,
			Status: status, StatusDisplay: tenantStatusText(status),
			BalanceUSD: row.BalanceUSD, UserCount: row.UserCount,
		}
		if row.ContactPerson != nil {
			item.ContactPerson = *row.ContactPerson
		}
		if row.ContactEmail != nil {
			item.ContactEmail = *row.ContactEmail
		}
		if row.CreatedTime != nil {
			item.CreatedTime = *row.CreatedTime
		}
		items = append(items, item)
	}
	out := &tenantListOutput{Body: httpx.NewPage(items, result.Total, int(result.Page), int(result.Size))}
	return out, nil
}

func (h *adminHandlers) createTenant(ctx context.Context, in *createTenantInput) (*createTenantOutput, error) {
	tenantName := strings.TrimSpace(in.Body.TenantName)
	if tenantName == "" {
		return nil, httpx.ErrBadRequest.WithDetail("租户名称不能为空")
	}
	initUsername := auth.NormalizeUsername(in.Body.InitUsername)
	tenantID := "T_" + strings.ToUpper(util.GenerateRandomString(6))
	status := in.Body.Status
	if status == 0 {
		status = 1
	}
	out := &createTenantOutput{}
	out.Body.TenantID = tenantID
	command := tenantports.TenantCreateCommand{
		TenantID:      tenantID,
		TenantName:    tenantName,
		ContactPerson: in.Body.ContactPerson,
		ContactEmail:  in.Body.ContactEmail,
		Status:        adminStatusFromInt(status),
	}
	if initUsername != "" {
		credential, err := h.activations.NewCredential()
		if err != nil {
			return nil, httpx.ErrInternal.WithCause(err)
		}
		initUserID := "TU_" + uuid.New().String()[:24]
		command.InitialUser = &tenantports.TenantInitialUserCreate{
			UserID:              initUserID,
			Username:            initUsername,
			Email:               in.Body.InitEmail,
			PasswordHash:        credential.PasswordHash,
			ActivationTokenHash: credential.TokenHash,
			ActivationExpiresAt: credential.ExpiresAt,
		}
		out.Body.InitUserID = initUserID
		out.Body.InitUsername = initUsername
		out.Body.ActivationToken = credential.Token
		out.Body.ActivationExpiresIn = int64(time.Until(credential.ExpiresAt).Seconds())
	}
	if err := h.tenantWriter.CreateTenant(ctx, command); err != nil {
		if tenantpg.IsTenantNameTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("租户名称已存在")
		}
		if authpg.IsUsernameTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("用户名已存在")
		}
		if authpg.IsEmailTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("邮箱已被使用")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return out, nil
}

func (h *adminHandlers) getTenant(ctx context.Context, in *tenantIDInput) (*tenantDetailOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if !isAdminClaims(claims) && claims.TenantID != in.ID {
		return nil, httpx.ErrForbidden.WithDetail("无权查看其他租户信息")
	}

	details, err := h.tenantRepo.GetTenantDetails(ctx, in.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound.WithDetail("租户不存在")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}

	statusInt := adminTenantStatusToInt(details.Status)
	out := &tenantDetailOutput{}
	out.Body.TenantID = details.TenantID
	out.Body.TenantName = details.TenantName
	out.Body.Status = statusInt
	out.Body.StatusDisplay = tenantStatusText(statusInt)
	out.Body.CreatedTime = details.CreatedTime
	if details.ContactPerson != nil {
		out.Body.ContactPerson = *details.ContactPerson
	}
	if details.ContactEmail != nil {
		out.Body.ContactEmail = *details.ContactEmail
	}
	return out, nil
}

func (h *adminHandlers) updateTenant(ctx context.Context, in *updateTenantInput) (*successOutput, error) {
	tenantName := strings.TrimSpace(in.Body.TenantName)
	if tenantName == "" {
		return nil, httpx.ErrBadRequest.WithDetail("租户名称不能为空")
	}
	updated, err := h.tenantWriter.UpdateTenant(ctx, tenantports.TenantUpdateCommand{
		TenantID:      in.ID,
		TenantName:    tenantName,
		ContactPerson: in.Body.ContactPerson,
		ContactEmail:  in.Body.ContactEmail,
		Status:        adminStatusFromInt(in.Body.Status),
	})
	if err != nil {
		if tenantpg.IsTenantNameTaken(err) {
			return nil, httpx.ErrConflict.WithDetail("租户名称已存在")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !updated {
		return nil, httpx.ErrNotFound.WithDetail("租户不存在")
	}
	return okSuccess(), nil
}

func (h *adminHandlers) deleteTenant(ctx context.Context, in *tenantIDInput) (*successOutput, error) {
	deleted, err := h.tenantWriter.DeleteTenant(ctx, in.ID)
	if err != nil {
		if tenantpg.IsTenantReferenced(err) {
			return nil, httpx.ErrConflict.WithDetail("租户仍有关联账号或业务数据，不能删除")
		}
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !deleted {
		return nil, httpx.ErrNotFound.WithDetail("租户不存在")
	}
	return okSuccess(), nil
}

func (h *adminHandlers) updateTenantStatus(ctx context.Context, in *updateTenantStatusInput) (*successOutput, error) {
	// 停用/启用只改访问权，不动钱。租户被停用后由 BanChecker 在网关层拦截
	// （gateway.go:141），余额不需要、也不应该跟着变：过期额度批次现在是一次
	// 真实的余额扣减，用它当"冻结开关"会真的把租户的钱扣掉，而且启用时无法还原。
	result, err := h.tenantStatusWriter.UpdateStatus(ctx, in.ID, in.Body.Status)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	if !result.Updated {
		return nil, httpx.ErrNotFound.WithDetail("租户不存在")
	}

	if h.blacklist != nil {
		if in.Body.Status == "disabled" {
			_ = h.blacklist.BanTenant(ctx, in.ID)
		} else {
			_ = h.blacklist.UnbanTenant(ctx, in.ID)
			for _, id := range result.RestoredUserIDs {
				_ = h.blacklist.UnbanUser(ctx, id)
			}
		}
	}

	return okSuccess(), nil
}
