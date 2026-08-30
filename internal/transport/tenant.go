package transport

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	tenantports "xiaodou/dai/internal/tenant/ports"
	"xiaodou/dai/internal/weborigin"
	"xiaodou/dai/libs/go/httpx"
)

// tenantSelfHandlers 承载 /api/v1 租户自助端点（仅租户用户 userType=3，限本租户）。
type tenantSelfHandlers struct {
	service tenantports.TenantSelfService
}

func newTenantSelfHandlers(service tenantports.TenantSelfService) *tenantSelfHandlers {
	return &tenantSelfHandlers{service: service}
}

// ---- DTO ----

type meOutput struct {
	Body struct {
		UserID        string `json:"userId"`
		TenantID      string `json:"tenantId"`
		Username      string `json:"username"`
		Email         string `json:"email"`
		Phone         string `json:"phone"`
		Status        int    `json:"status"`
		LastLoginTime *int64 `json:"lastLoginTime"`
		CreatedTime   int64  `json:"createdTime"`
	}
}

type listInvitationsInput struct {
	Page int `query:"page" default:"1"`
	Size int `query:"size" default:"20"`
}

type invitationsListOutput struct {
	Body httpx.Page[tenantInvitationItemOutput]
}

type tenantInvitationItemOutput struct {
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

type createInvitationInput struct {
	Body struct {
		Description string `json:"description" required:"false"`
		MaxUses     int    `json:"max_uses" required:"false"`
		ExpireTime  *int64 `json:"expire_time" required:"false"`
	}
}

type createInvitationOutput struct {
	Body struct {
		Code            string `json:"code"`
		RegistrationURL string `json:"registrationUrl,omitempty"`
		TenantID        string `json:"tenantId"`
		Description     string `json:"description"`
		MaxUses         int    `json:"maxUses"`
		ExpireTime      *int64 `json:"expireTime,omitempty"`
	}
}

type updateInvitationInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Status      int    `json:"status"`
		Description string `json:"description" required:"false"`
	}
}

type invitationIDInput struct {
	ID int64 `path:"id"`
}

type overviewOutput struct {
	Body *tenantports.TenantOverviewStats
}

type overviewInput struct {
	TimeFrom int64 `query:"timeFrom" required:"false"`
	TimeTo   int64 `query:"timeTo" required:"false"`
}

type clientConsumptionInput struct {
	TimeFrom int64 `query:"timeFrom" required:"false"`
	TimeTo   int64 `query:"timeTo" required:"false"`
}

type clientConsumptionOutput struct {
	Body []tenantports.ClientConsumptionItem
}

type userConsumptionInput struct {
	TimeFrom int64 `query:"timeFrom" required:"false"`
	TimeTo   int64 `query:"timeTo" required:"false"`
	Limit    int   `query:"limit" default:"10" minimum:"1" maximum:"20"`
}

type userConsumptionOutput struct {
	Body []tenantports.UserConsumptionItem
}

// registerTenantSelf 注册租户自助端点（tenant_self capability）。
func registerTenantSelf(api huma.API, d tenantSelfModule) {
	h := newTenantSelfHandlers(d.service)
	tenantOnly := huma.Middlewares{userAuth(api, d.auth.JWT, d.auth.Blacklist), requireCapability(api, auth.CapabilityTenantSelf)}
	tenantOnly = append(tenantOnly, requireRecentAuthForMutation(api, d.auth.RecentAuth))

	huma.Register(api, huma.Operation{OperationID: "tenant-me", Method: http.MethodGet, Path: "/api/v1/tenants/me",
		Summary: "当前租户用户信息", Tags: []string{"tenant-self"}, Middlewares: tenantOnly}, h.me)
	huma.Register(api, huma.Operation{OperationID: "tenant-list-invitations", Method: http.MethodGet, Path: "/api/v1/invitations",
		Summary: "邀请码列表", Tags: []string{"tenant-self"}, Middlewares: tenantOnly}, h.listInvitations)
	huma.Register(api, huma.Operation{OperationID: "tenant-create-invitation", Method: http.MethodPost, Path: "/api/v1/invitations",
		Summary: "创建邀请码", Tags: []string{"tenant-self"}, Middlewares: tenantOnly, DefaultStatus: http.StatusCreated}, h.createInvitation)
	huma.Register(api, huma.Operation{OperationID: "tenant-update-invitation", Method: http.MethodPut, Path: "/api/v1/invitations/{id}",
		Summary: "更新邀请码", Tags: []string{"tenant-self"}, Middlewares: tenantOnly}, h.updateInvitation)
	huma.Register(api, huma.Operation{OperationID: "tenant-delete-invitation", Method: http.MethodDelete, Path: "/api/v1/invitations/{id}",
		Summary: "删除邀请码", Tags: []string{"tenant-self"}, Middlewares: tenantOnly}, h.deleteInvitation)
	huma.Register(api, huma.Operation{OperationID: "tenant-analytics-overview", Method: http.MethodGet, Path: "/api/v1/tenants/analytics/overview",
		Summary: "租户统计概览", Tags: []string{"tenant-self"}, Middlewares: tenantOnly}, h.analyticsOverview)
	huma.Register(api, huma.Operation{OperationID: "tenant-analytics-app-consumption", Method: http.MethodGet, Path: "/api/v1/tenants/analytics/app-consumption",
		Summary: "按应用消耗分布", Tags: []string{"tenant-self"}, Middlewares: tenantOnly}, h.clientConsumption)
	huma.Register(api, huma.Operation{OperationID: "tenant-analytics-user-consumption", Method: http.MethodGet, Path: "/api/v1/tenants/analytics/user-consumption",
		Summary: "终端用户消费贡献排行", Tags: []string{"tenant-self"}, Middlewares: tenantOnly}, h.userConsumption)
}

func (h *tenantSelfHandlers) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	user, err := service.GetByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	out := &meOutput{}
	out.Body.UserID = user.UserID
	out.Body.TenantID = user.TenantID
	out.Body.Username = user.Username
	out.Body.Email = user.Email
	out.Body.Phone = user.Phone
	out.Body.Status = user.Status
	out.Body.LastLoginTime = user.LastLoginTime
	out.Body.CreatedTime = user.CreatedTime
	return out, nil
}

func (h *tenantSelfHandlers) listInvitations(ctx context.Context, in *listInvitationsInput) (*invitationsListOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	page, size := normalizePage(in.Page, in.Size)
	list, total, err := service.ListInvitationCodes(ctx, claims.TenantID, page, size)
	if err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	items := make([]tenantInvitationItemOutput, 0, len(list))
	for i := range list {
		items = append(items, tenantInvitationResponse(ctx, list[i]))
	}
	return &invitationsListOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *tenantSelfHandlers) createInvitation(ctx context.Context, in *createInvitationInput) (*createInvitationOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	item, err := service.CreateInvitation(ctx, tenantports.InvitationCreateCommand{
		TenantID:    claims.TenantID,
		CreatedBy:   claims.UserID,
		Description: in.Body.Description,
		MaxUses:     in.Body.MaxUses,
		ExpireTime:  in.Body.ExpireTime,
	})
	if err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	out := &createInvitationOutput{}
	out.Body.Code = item.Code
	out.Body.RegistrationURL = invitationRegistrationURL(ctx, item.Code)
	out.Body.TenantID = item.TenantID
	out.Body.Description = item.Description
	out.Body.MaxUses = item.MaxUses
	out.Body.ExpireTime = item.ExpireTime
	return out, nil
}

func (h *tenantSelfHandlers) updateInvitation(ctx context.Context, in *updateInvitationInput) (*successOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	if err := service.UpdateInvitation(ctx, tenantports.InvitationUpdateCommand{
		ID:          in.ID,
		TenantID:    claims.TenantID,
		Status:      in.Body.Status,
		Description: in.Body.Description,
	}); err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	return okSuccess(), nil
}

func (h *tenantSelfHandlers) deleteInvitation(ctx context.Context, in *invitationIDInput) (*successOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	if err := service.DeleteInvitation(ctx, in.ID, claims.TenantID); err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	return okSuccess(), nil
}

func (h *tenantSelfHandlers) analyticsOverview(ctx context.Context, in *overviewInput) (*overviewOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	stats, err := service.GetTenantOverviewStats(ctx, claims.TenantID, timeFrom, timeTo)
	if err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	return &overviewOutput{Body: stats}, nil
}

func (h *tenantSelfHandlers) clientConsumption(ctx context.Context, in *clientConsumptionInput) (*clientConsumptionOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	list, err := service.GetClientConsumption(ctx, claims.TenantID, timeFrom, timeTo)
	if err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	return &clientConsumptionOutput{Body: list}, nil
}

func (h *tenantSelfHandlers) userConsumption(ctx context.Context, in *userConsumptionInput) (*userConsumptionOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	service, err := h.requireService()
	if err != nil {
		return nil, err
	}
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	list, err := service.GetUserConsumptionRanking(ctx, claims.TenantID, timeFrom, timeTo, in.Limit)
	if err != nil {
		return nil, tenantSelfHTTPError(err)
	}
	return &userConsumptionOutput{Body: list}, nil
}

func (h *tenantSelfHandlers) requireService() (tenantports.TenantSelfService, error) {
	if h == nil || h.service == nil {
		return nil, httpx.ErrUnavailable.WithDetail("租户自助服务不可用")
	}
	return h.service, nil
}

func tenantSelfHTTPError(err error) error {
	switch {
	case errors.Is(err, tenantports.ErrTenantUserNotFound):
		return httpx.ErrNotFound.WithDetail("用户不存在")
	case errors.Is(err, tenantports.ErrSelfServiceUnavailable):
		return httpx.ErrUnavailable.WithDetail("租户自助服务不可用")
	case errors.Is(err, tenantports.ErrInvitationCodeTaken):
		return httpx.ErrConflict.WithDetail("邀请码生成冲突，请重试")
	default:
		return httpx.ErrInternal.WithCause(err)
	}
}

func tenantInvitationResponse(ctx context.Context, item tenantports.InviteCodeItem) tenantInvitationItemOutput {
	return tenantInvitationItemOutput{
		ID:              item.ID,
		Code:            item.Code,
		RegistrationURL: invitationRegistrationURL(ctx, item.Code),
		TenantID:        item.TenantID,
		CreatedBy:       item.CreatedBy,
		Description:     item.Description,
		MaxUses:         item.MaxUses,
		UsedCount:       item.UsedCount,
		Status:          item.Status,
		ExpireTime:      item.ExpireTime,
		CreatedTime:     item.CreatedTime,
		UpdatedTime:     item.UpdatedTime,
	}
}

func parseAnalyticsWindow(timeFromValue, timeToValue int64) (*time.Time, *time.Time, error) {
	var timeFrom *time.Time
	var timeTo *time.Time
	if timeFromValue > 0 {
		t := time.UnixMilli(timeFromValue).UTC()
		timeFrom = &t
	}
	if timeToValue > 0 {
		t := time.UnixMilli(timeToValue).UTC()
		timeTo = &t
	}
	if timeFrom != nil && timeTo != nil && !timeFrom.Before(*timeTo) {
		return nil, nil, httpx.ErrBadRequest.WithDetail("timeTo must be greater than timeFrom")
	}
	return timeFrom, timeTo, nil
}

func invitationRegistrationURL(ctx context.Context, code string) string {
	if strings.TrimSpace(code) == "" {
		return ""
	}
	return weborigin.Resolve(ctx, "/register/"+url.PathEscape(code))
}
