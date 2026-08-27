package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	authports "xiaodou/dai/internal/auth/ports"
	billingdomain "xiaodou/dai/internal/billing"
	billingports "xiaodou/dai/internal/billing/ports"
	billingsvc "xiaodou/dai/internal/billing/service"
	shared "xiaodou/dai/internal/domain"
	tenantports "xiaodou/dai/internal/tenant/ports"
	"xiaodou/dai/libs/go/httpx"
)

// ---- DTO ----

type rechargeInput struct {
	Body struct {
		PackageType     int    `json:"packageType" doc:"1=租户充值 2=用户充值"`
		UserID          string `json:"userId" required:"false"`
		TenantID        string `json:"tenantId" required:"false"`
		PaidAmountMinor int64  `json:"paidAmountMinor" required:"false" doc:"支付渠道最小单位金额"`
		AmountMicroUSD  int64  `json:"amountMicroUsd" minimum:"1" doc:"到账金额，单位 micro-USD"`
		Note            string `json:"note" required:"false"`
		PaymentRef      string `json:"paymentRef" required:"false"`
		ExpireTime      *int64 `json:"expireTime" required:"false"`
	}
}

type rechargeOutput struct {
	Body struct {
		OrderID         string  `json:"orderId"`
		BalanceLotID    string  `json:"balanceLotId"`
		TenantID        string  `json:"tenantId"`
		UserID          string  `json:"userId"`
		Currency        string  `json:"currency"`
		AmountMicroUSD  int64   `json:"amountMicroUsd"`
		PaidAmountMinor int64   `json:"paidAmountMinor"`
		ClearedDebtUSD  float64 `json:"clearedDebtUsd"`
		BalanceLotUSD   float64 `json:"balanceLotUsd"`
		OrderTime       int64   `json:"orderTime"`
	}
}

type reverseRechargeInput struct {
	OrderID string `path:"orderId"`
	Body    struct {
		Reason string `json:"reason"`
	}
}

type reverseRechargeOutput struct {
	Body struct {
		Status            string  `json:"status"`
		OrderID           string  `json:"orderId"`
		BalanceLotID      string  `json:"balanceLotId"`
		ReversedAmountUSD float64 `json:"reversedAmountUsd"`
		OriginalAmountUSD float64 `json:"originalAmountUsd"`
		LostAmountUSD     float64 `json:"lostAmountUsd"`
		BalanceLotStatus  string  `json:"balanceLotStatus"`
	}
}

type usageRefundInput struct {
	Body struct {
		RequestID string `json:"requestId"`
		Reason    string `json:"reason" required:"false"`
	}
}

type debtStatusInput struct {
	OwnerType string `path:"owner_type" enum:"tenant,user"`
	AccountID string `path:"id"`
}

type debtStatusOutput struct {
	Body struct {
		OwnerType               string `json:"owner_type"`
		AccountID               string `json:"account_id"`
		OutstandingDebtMicroUSD int64  `json:"outstanding_debt_micro_usd"`
		ServiceState            string `json:"service_state" enum:"active,blocked_debt"`
	}
}

type auditLogItem struct {
	ID            int64  `json:"id"`
	EventType     string `json:"eventType"`
	PrincipalType string `json:"principalType"`
	UserID        string `json:"userId,omitempty"`
	Decision      string `json:"decision"`
	ReasonCode    string `json:"reasonCode,omitempty"`
	ReasonMessage string `json:"reasonMessage,omitempty"`
	CreatedAt     int64  `json:"createdAt"`
}

type auditLogsInput struct {
	EventType     string `query:"eventType" required:"false"`
	PrincipalType string `query:"principalType" required:"false"`
	UserID        string `query:"userId" required:"false"`
	Decision      string `query:"decision" required:"false"`
	Page          int    `query:"page" default:"1"`
	Size          int    `query:"size" default:"20"`
}

type auditLogsOutput struct {
	Body httpx.Page[auditLogItem]
}

func timePtrFromMillis(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms).UTC()
	return &t
}

func orderTypeFromPackageType(packageType int) (string, error) {
	switch packageType {
	case 1:
		return billingdomain.OrderTypePlatformToTenant, nil
	case 2:
		return billingdomain.OrderTypeTenantToUser, nil
	default:
		return "", fmt.Errorf("invalid packageType: %d", packageType)
	}
}

// registerAdminFinance 注册充值/撤销/退款/债务/审计日志端点。
func registerAdminFinance(api huma.API, d adminFinanceModule) {
	h := newAdminFinanceHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireCapability(api, auth.CapabilityPlatformAdmin)}
	superAdmin := huma.Middlewares{ua, requireCapability(api, auth.CapabilitySuperAdmin)}
	sysUserSensitive := huma.Middlewares{ua, requireCapability(api, auth.CapabilityPlatformAdmin), requireRecentAuth(api, d.RecentAuth)}
	sysOrTenantSensitive := huma.Middlewares{ua, requireAnyCapability(api, auth.CapabilityPlatformAdmin, auth.CapabilityTenantSelf), requireRecentAuth(api, d.RecentAuth)}

	huma.Register(api, huma.Operation{OperationID: "admin-recharge", Method: http.MethodPost, Path: "/api/v1/recharges",
		Summary: "充值（租户/用户）", Tags: []string{"admin-finance"}, Middlewares: sysOrTenantSensitive, DefaultStatus: http.StatusCreated}, h.recharge)
	huma.Register(api, huma.Operation{OperationID: "admin-reverse-recharge", Method: http.MethodPost, Path: "/api/v1/recharges/{orderId}/reverse",
		Summary: "撤销充值", Tags: []string{"admin-finance"}, Middlewares: sysOrTenantSensitive}, h.reverseRecharge)
	huma.Register(api, huma.Operation{OperationID: "admin-refund-usage", Method: http.MethodPost, Path: "/api/v1/ai/usage/refund",
		Summary: "AI 使用记录退款", Tags: []string{"admin-finance"}, Middlewares: sysUserSensitive}, h.refundUsage)
	huma.Register(api, huma.Operation{OperationID: "admin-get-debt", Method: http.MethodGet, Path: "/api/v1/admin/debts/{owner_type}/{id}",
		Summary: "查询账户当前债务", Tags: []string{"admin-finance"}, Middlewares: sysUser}, h.getDebt)
	huma.Register(api, huma.Operation{OperationID: "admin-auth-audit-logs", Method: http.MethodGet, Path: "/api/v1/auth-audit-logs",
		Summary: "认证审计日志", Tags: []string{"admin-finance"}, Middlewares: superAdmin}, h.authAuditLogs)
}

func (h *adminHandlers) recharge(ctx context.Context, in *rechargeInput) (*rechargeOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	actor := actorFromClaims(claims)

	if actor.Has(auth.CapabilityTenantSelf) && in.Body.PackageType != 2 {
		return nil, httpx.ErrForbidden.WithDetail("租户用户只能进行用户充值")
	}
	orderType, err := orderTypeFromPackageType(in.Body.PackageType)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail(err.Error())
	}
	expiresAt := timePtrFromMillis(in.Body.ExpireTime)

	var tenantID, userID string
	if orderType == billingdomain.OrderTypePlatformToTenant {
		if !actor.Has(auth.CapabilityPlatformAdmin) {
			return nil, httpx.ErrForbidden.WithDetail("只有平台管理员才能进行租户充值")
		}
		tenantID = in.Body.TenantID
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("租户充值时 tenantId 必填")
		}
		if h.tenantReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("租户查询服务不可用")
		}
		if _, err := h.tenantReader.GetTenantDetails(ctx, tenantID); err != nil {
			if !errors.Is(err, tenantports.ErrTenantNotFound) {
				return nil, httpx.ErrInternal.WithCause(err)
			}
			return nil, httpx.ErrBadRequest.WithDetail("目标租户不存在")
		}
	} else {
		if in.Body.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("用户充值时 userId 必填")
		}
		if h.tenantReader == nil {
			return nil, httpx.ErrUnavailable.WithDetail("租户查询服务不可用")
		}
		userTenantID, err := h.tenantReader.GetEndUserTenantID(ctx, in.Body.UserID)
		if err != nil {
			if !errors.Is(err, tenantports.ErrTenantEndUserNotFound) {
				return nil, httpx.ErrInternal.WithCause(err)
			}
			return nil, httpx.ErrBadRequest.WithDetail("用户不存在或无归属租户")
		}
		if userTenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("用户不存在或无归属租户")
		}
		if !actor.Owns(auth.NewResourceOwnership(userTenantID, "")) {
			return nil, httpx.ErrForbidden.WithDetail("只能为本租户用户充值")
		}
		tenantID = userTenantID
		userID = in.Body.UserID
	}

	pkgSource := billingdomain.PackageSourceAdminRecharge
	if orderType == billingdomain.OrderTypeTenantToUser {
		pkgSource = billingdomain.PackageSourceTenantRecharge
	}
	if in.Body.AmountMicroUSD <= 0 {
		return nil, httpx.ErrBadRequest.WithDetail("amountMicroUsd must be positive")
	}

	grant, err := h.rechargeSvc.GrantManual(ctx, billingsvc.GrantParams{
		OrderType: orderType, TenantID: tenantID, UserID: userID,
		AmountMicroUSD: in.Body.AmountMicroUSD, PaidAmount: in.Body.PaidAmountMinor,
		PaymentRef: in.Body.PaymentRef, Note: in.Body.Note, OperatorID: claims.UserID,
		Source: pkgSource, ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, toProblem(err)
	}

	out := &rechargeOutput{}
	out.Body.OrderID = grant.OrderID
	out.Body.BalanceLotID = grant.BalanceLotID
	out.Body.TenantID = tenantID
	out.Body.UserID = userID
	out.Body.Currency = "USD"
	out.Body.AmountMicroUSD = in.Body.AmountMicroUSD
	out.Body.PaidAmountMinor = in.Body.PaidAmountMinor
	out.Body.ClearedDebtUSD = billingdomain.MicroToUSD(grant.ClearedDebtMicroUSD)
	out.Body.BalanceLotUSD = billingdomain.MicroToUSD(grant.LotAmountMicroUSD)
	out.Body.OrderTime = millisFromTime(grant.OrderTime)
	return out, nil
}

func (h *adminHandlers) reverseRecharge(ctx context.Context, in *reverseRechargeInput) (*reverseRechargeOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if in.Body.Reason == "" {
		return nil, httpx.ErrBadRequest.WithDetail("reason is required")
	}
	var result *billingsvc.ReverseResult
	var err error
	actor := actorFromClaims(claims)
	if actor.Has(auth.CapabilityTenantSelf) {
		result, err = h.deduction.ReverseTenantOrder(ctx, in.OrderID, string(actor.TenantID), in.Body.Reason, string(actor.UserID))
	} else {
		result, err = h.deduction.ReverseOrder(ctx, in.OrderID, in.Body.Reason, string(actor.UserID))
	}
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) {
			return nil, httpx.ErrForbidden.WithDetail("只能撤销本租户的用户充值记录")
		}
		return nil, toProblem(err)
	}
	out := &reverseRechargeOutput{}
	out.Body.Status = "SUCCESS"
	if result.IsPartial {
		out.Body.Status = "PARTIAL_REVERSAL"
	}
	out.Body.OrderID = result.OrderID
	out.Body.BalanceLotID = result.BalanceLotID
	out.Body.ReversedAmountUSD = billingdomain.MicroToUSD(result.ReversedCredits)
	out.Body.OriginalAmountUSD = billingdomain.MicroToUSD(result.OriginalCredits)
	out.Body.LostAmountUSD = billingdomain.MicroToUSD(result.LostCredits)
	out.Body.BalanceLotStatus = "revoked"
	if result.IsPartial {
		out.Body.BalanceLotStatus = "depleted"
	}
	return out, nil
}

func (h *adminHandlers) refundUsage(ctx context.Context, in *usageRefundInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	reason := in.Body.Reason
	if reason == "" {
		reason = "管理员手动退款"
	}
	if err := h.deduction.RefundUsage(ctx, in.Body.RequestID, reason, claims.UserID); err != nil {
		return nil, toProblem(err)
	}
	out := &messageOutput{}
	out.Body.Message = "退款成功"
	return out, nil
}

// getDebt reports the negative half of the account balance. Debt is not stored:
// it is what a balance below zero means, so this reads the same number the
// admission gate reads instead of a parallel column that could disagree.
func (h *adminHandlers) getDebt(ctx context.Context, in *debtStatusInput) (*debtStatusOutput, error) {
	if h.accountQueries == nil {
		return nil, httpx.ErrUnavailable.WithDetail("账户查询服务不可用")
	}
	var balance *billingports.BalanceResponse
	var err error
	if in.OwnerType == "tenant" {
		balance, err = h.accountQueries.GetTenantBalance(ctx, in.AccountID, false)
	} else {
		balance, err = h.accountQueries.GetUserBalance(ctx, in.AccountID, false)
	}
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail("账户不存在")
	}
	out := &debtStatusOutput{}
	out.Body.OwnerType = in.OwnerType
	out.Body.AccountID = in.AccountID
	out.Body.OutstandingDebtMicroUSD = balance.OutstandingDebtMicroUSD
	out.Body.ServiceState = balance.ServiceState
	return out, nil
}

func (h *adminHandlers) authAuditLogs(ctx context.Context, in *auditLogsInput) (*auditLogsOutput, error) {
	if h.authAuditReader == nil {
		return nil, httpx.ErrUnavailable.WithDetail("auth audit log service is not configured")
	}
	page, err := h.authAuditReader.ListAuthAuditLogs(ctx, authports.AuthAuditLogFilter{
		EventType: in.EventType, PrincipalType: in.PrincipalType, UserID: in.UserID, Decision: in.Decision,
		Page: in.Page, Size: in.Size,
	})
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	items := make([]auditLogItem, 0, len(page.Records))
	for _, record := range page.Records {
		var it auditLogItem
		it.ID = record.ID
		it.EventType = record.EventType
		it.PrincipalType = record.PrincipalType
		it.UserID = record.UserID
		it.Decision = record.Decision
		it.ReasonCode = record.ReasonCode
		it.ReasonMessage = record.ReasonMessage
		it.CreatedAt = millisFromTime(record.CreatedAt)
		items = append(items, it)
	}
	return &auditLogsOutput{Body: httpx.NewPage(items, page.Total, page.Page, page.Size)}, nil
}
