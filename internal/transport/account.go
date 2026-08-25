package transport

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	billingdomain "xiaodou/dai/internal/billing"
	billingports "xiaodou/dai/internal/billing/ports"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/libs/go/httpx"
)

// accountHandlers 承载 /api/v1/account 账户自助端点（任意已登录用户；按 userType
// 在 handler 层覆盖查询范围：租户用户限本租户、终端用户限本人）。
type accountHandlers struct {
	queries billingports.AccountQueryReader
}

func newAccountHandlers(queries billingports.AccountQueryReader) *accountHandlers {
	return &accountHandlers{queries: queries}
}

// ---- DTO ----

type accountBalanceInput struct {
	AccountType int    `query:"accountType" required:"false" doc:"1=租户 2=用户（管理员用）"`
	AccountID   string `query:"accountId" required:"false"`
	Detail      bool   `query:"detail"`
}

type accountBalanceOutput struct {
	Body *billingports.BalanceResponse
}

type rechargeRecordsInput struct {
	TenantID     string `query:"tenantId" required:"false"`
	UserID       string `query:"userId" required:"false"`
	TenantName   string `query:"tenantName" required:"false"`
	Username     string `query:"username" required:"false"`
	RechargeType string `query:"rechargeType" required:"false" doc:"1=平台充租户 2=租户充用户"`
	TimeFrom     int64  `query:"timeFrom" required:"false"`
	TimeTo       int64  `query:"timeTo" required:"false"`
	Page         int    `query:"page" default:"1"`
	Size         int    `query:"size" default:"20"`
}

type rechargeRecordsOutput struct {
	Body httpx.Page[billingports.RechargeRecordRow]
}

type accountStatsInput struct {
	AccountID string `query:"accountId" required:"false"`
}

type accountStatsOutput struct {
	Body *billingports.AccountStatsResult
}

// registerAccount 注册账户自助端点（RequireAuthenticated：1/2/3/4）。
func registerAccount(api huma.API, d Deps) {
	h := newAccountHandlers(d.AccountQueries)
	authed := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist), requireAnyCapability(api,
		auth.CapabilitySuperAdmin, auth.CapabilityPlatformAdmin,
		auth.CapabilityTenantSelf, auth.CapabilityCustomerSelf,
	)}

	huma.Register(api, huma.Operation{OperationID: "account-balance", Method: http.MethodGet, Path: "/api/v1/account/balance",
		Summary: "账户余额", Tags: []string{"account"}, Middlewares: authed}, h.balance)
	huma.Register(api, huma.Operation{OperationID: "account-recharge-records", Method: http.MethodGet, Path: "/api/v1/account/recharge-records",
		Summary: "充值记录", Tags: []string{"account"}, Middlewares: authed}, h.rechargeRecords)
	huma.Register(api, huma.Operation{OperationID: "account-stats", Method: http.MethodGet, Path: "/api/v1/account/stats",
		Summary: "账户统计", Tags: []string{"account"}, Middlewares: authed}, h.stats)
}

func (h *accountHandlers) balance(ctx context.Context, in *accountBalanceInput) (*accountBalanceOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	actor := actorFromClaims(claims)
	packageType, accountID := in.AccountType, in.AccountID
	if actor.Has(auth.CapabilityTenantSelf) {
		packageType, accountID = 1, actor.TenantID
	} else if actor.Has(auth.CapabilityCustomerSelf) {
		packageType, accountID = 2, actor.UserID
	}
	if accountID == "" {
		return nil, httpx.ErrBadRequest.WithDetail("缺少账户 ID")
	}
	if h.queries == nil {
		return nil, httpx.ErrUnavailable.WithDetail("账户查询服务不可用")
	}

	var res *billingports.BalanceResponse
	var err error
	if packageType == 1 {
		res, err = h.queries.GetTenantBalance(ctx, accountID, in.Detail)
	} else {
		res, err = h.queries.GetUserBalance(ctx, accountID, in.Detail)
	}
	if err != nil {
		return nil, accountQueryHTTPError(err)
	}
	return &accountBalanceOutput{Body: res}, nil
}

func (h *accountHandlers) rechargeRecords(ctx context.Context, in *rechargeRecordsInput) (*rechargeRecordsOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if h.queries == nil {
		return nil, httpx.ErrUnavailable.WithDetail("账户查询服务不可用")
	}
	tenantID, userID := in.TenantID, in.UserID
	orderTypes := orderTypesFromRechargeType(in.RechargeType)
	var timeFrom *time.Time
	var timeTo *time.Time
	if in.TimeFrom > 0 {
		t := time.UnixMilli(in.TimeFrom).UTC()
		timeFrom = &t
	}
	if in.TimeTo > 0 {
		t := time.UnixMilli(in.TimeTo).UTC()
		timeTo = &t
	}
	actor := actorFromClaims(claims)
	if actor.Has(auth.CapabilityTenantSelf) {
		tenantID = actor.TenantID
	} else if actor.Has(auth.CapabilityCustomerSelf) {
		tenantID, userID, orderTypes = actor.TenantID, actor.UserID, billingdomain.UserRechargeOrderTypes
	}
	page, size := normalizePage(in.Page, in.Size)
	list, total, err := h.queries.ListRechargeRecords(ctx, billingports.RechargeRecordsQuery{
		TenantID: tenantID, UserID: userID, TenantName: in.TenantName, Username: in.Username,
		OrderTypes: orderTypes, TimeFrom: timeFrom, TimeTo: timeTo, Page: page, Size: size,
	})
	if err != nil {
		return nil, accountQueryHTTPError(err)
	}
	return &rechargeRecordsOutput{Body: httpx.NewPage(list, total, page, size)}, nil
}

func (h *accountHandlers) stats(ctx context.Context, in *accountStatsInput) (*accountStatsOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	actor := actorFromClaims(claims)
	if actor.Has(auth.CapabilityCustomerSelf) {
		return nil, httpx.ErrForbidden.WithDetail("终端用户不支持查询账户统计")
	}
	tenantID := in.AccountID
	if actor.Has(auth.CapabilityTenantSelf) {
		tenantID = actor.TenantID
	}
	if tenantID == "" {
		return nil, httpx.ErrBadRequest.WithDetail("缺少 accountId 参数")
	}
	if h.queries == nil {
		return nil, httpx.ErrUnavailable.WithDetail("账户查询服务不可用")
	}
	stats, err := h.queries.GetAccountStats(ctx, tenantID)
	if err != nil {
		return nil, accountQueryHTTPError(err)
	}
	return &accountStatsOutput{Body: stats}, nil
}

func accountQueryHTTPError(err error) error {
	switch {
	case errors.Is(err, billingports.ErrAccountQueryUnavailable):
		return httpx.ErrUnavailable.WithDetail("账户查询服务不可用")
	case domain.IsNotFoundError(err):
		return httpx.ErrNotFound.WithDetail("账户不存在")
	default:
		return httpx.ErrInternal.WithCause(err)
	}
}

func orderTypesFromRechargeType(param string) []string {
	switch param {
	case "1":
		return billingdomain.TenantRechargeOrderTypes
	case "2":
		return billingdomain.UserRechargeOrderTypes
	}
	return nil
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
