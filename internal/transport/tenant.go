package transport

import (
	"context"
	"crypto/rand"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	tenantpg "xiaodou/dai/internal/tenant/pg"
	"xiaodou/dai/libs/go/httpx"
)

const inviteCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generateInviteCode() string {
	code := make([]byte, 8)
	n := big.NewInt(int64(len(inviteCharset)))
	for i := range code {
		x, _ := rand.Int(rand.Reader, n)
		code[i] = inviteCharset[x.Int64()]
	}
	return string(code)
}

// tenantSelfHandlers 承载 /api/v1 租户自助端点（仅租户用户 userType=3，限本租户）。
type tenantSelfHandlers struct {
	repo                  *tenantpg.TenantRepo
	log                   *zap.Logger
	customerPortalBaseURL string
}

func newTenantSelfHandlers(pool *pgxpool.Pool, log *zap.Logger, customerPortalBaseURL string) *tenantSelfHandlers {
	return &tenantSelfHandlers{
		repo:                  tenantpg.NewTenantRepo(pool),
		log:                   log,
		customerPortalBaseURL: customerPortalBaseURL,
	}
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
	Body httpx.Page[tenantpg.InviteCodeItem]
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
	Body *tenantpg.TenantOverviewStats
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
	Body []tenantpg.ClientConsumptionItem
}

type userConsumptionInput struct {
	TimeFrom int64 `query:"timeFrom" required:"false"`
	TimeTo   int64 `query:"timeTo" required:"false"`
	Limit    int   `query:"limit" default:"10" minimum:"1" maximum:"20"`
}

type userConsumptionOutput struct {
	Body []tenantpg.UserConsumptionItem
}

// registerTenantSelf 注册租户自助端点（requireUserType(3)）。
func registerTenantSelf(api huma.API, d Deps) {
	h := newTenantSelfHandlers(d.Pool, d.Logger, d.PortalBaseURL)
	tenantOnly := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist), requireUserType(api, 3)}

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
	user, err := h.repo.GetByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, httpx.ErrNotFound.WithDetail("用户不存在")
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
	page, size := normalizePage(in.Page, in.Size)
	list, total, err := h.repo.ListInvitationCodes(ctx, claims.TenantID, page, size)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	for i := range list {
		list[i].RegistrationURL = invitationRegistrationURL(h.customerPortalBaseURL, list[i].Code)
	}
	return &invitationsListOutput{Body: httpx.NewPage(list, total, page, size)}, nil
}

func (h *tenantSelfHandlers) createInvitation(ctx context.Context, in *createInvitationInput) (*createInvitationOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	code := generateInviteCode()
	var err error
	for i := 0; i < 5; i++ {
		err = h.repo.CreateInvitationCode(ctx, code, claims.TenantID, claims.UserID, in.Body.Description, in.Body.MaxUses, in.Body.ExpireTime)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
			break
		}
		code = generateInviteCode()
	}
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	out := &createInvitationOutput{}
	out.Body.Code = code
	out.Body.RegistrationURL = invitationRegistrationURL(h.customerPortalBaseURL, code)
	out.Body.TenantID = claims.TenantID
	out.Body.Description = in.Body.Description
	out.Body.MaxUses = in.Body.MaxUses
	out.Body.ExpireTime = in.Body.ExpireTime
	return out, nil
}

func (h *tenantSelfHandlers) updateInvitation(ctx context.Context, in *updateInvitationInput) (*successOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if err := h.repo.UpdateInvitationCode(ctx, in.ID, claims.TenantID, in.Body.Status, in.Body.Description); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return okSuccess(), nil
}

func (h *tenantSelfHandlers) deleteInvitation(ctx context.Context, in *invitationIDInput) (*successOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if err := h.repo.DeleteInvitationCode(ctx, in.ID, claims.TenantID); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return okSuccess(), nil
}

func (h *tenantSelfHandlers) analyticsOverview(ctx context.Context, in *overviewInput) (*overviewOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	stats, err := h.repo.GetTenantOverviewStats(ctx, claims.TenantID, timeFrom, timeTo)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return &overviewOutput{Body: stats}, nil
}

func (h *tenantSelfHandlers) clientConsumption(ctx context.Context, in *clientConsumptionInput) (*clientConsumptionOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	list, err := h.repo.GetClientConsumption(ctx, claims.TenantID, timeFrom, timeTo)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return &clientConsumptionOutput{Body: list}, nil
}

func (h *tenantSelfHandlers) userConsumption(ctx context.Context, in *userConsumptionInput) (*userConsumptionOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	timeFrom, timeTo, err := parseAnalyticsWindow(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	list, err := h.repo.GetUserConsumptionRanking(ctx, claims.TenantID, timeFrom, timeTo, in.Limit)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	return &userConsumptionOutput{Body: list}, nil
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

func invitationRegistrationURL(baseURL, code string) string {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(code) == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/register/" + url.PathEscape(code)
}
