package transport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
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

type refundInput struct {
	Body struct {
		EventID string `json:"eventId"`
		Reason  string `json:"reason" required:"false"`
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
func registerAdminFinance(api huma.API, d Deps) {
	h := newAdminHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysOrTenant := huma.Middlewares{ua, requireUserType(api, 1, 2, 3)}
	sysUser := huma.Middlewares{ua, requireUserType(api, 1, 2)}
	superAdmin := huma.Middlewares{ua, requireUserType(api, 1)}

	huma.Register(api, huma.Operation{OperationID: "admin-recharge", Method: http.MethodPost, Path: "/api/v1/recharges",
		Summary: "充值（租户/用户）", Tags: []string{"admin-finance"}, Middlewares: sysOrTenant, DefaultStatus: http.StatusCreated}, h.recharge)
	huma.Register(api, huma.Operation{OperationID: "admin-reverse-recharge", Method: http.MethodPost, Path: "/api/v1/recharges/{orderId}/reverse",
		Summary: "撤销充值", Tags: []string{"admin-finance"}, Middlewares: sysOrTenant}, h.reverseRecharge)
	huma.Register(api, huma.Operation{OperationID: "admin-refund", Method: http.MethodPost, Path: "/api/v1/refunds",
		Summary: "手动全额退款", Tags: []string{"admin-finance"}, Middlewares: sysUser}, h.refund)
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
	userType := claims.UserType
	loginTenantID := claims.TenantID

	if userType == 3 && in.Body.PackageType != 2 {
		return nil, httpx.ErrForbidden.WithDetail("租户用户只能进行用户充值")
	}
	orderType, err := orderTypeFromPackageType(in.Body.PackageType)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail(err.Error())
	}
	expiresAt := timePtrFromMillis(in.Body.ExpireTime)

	var tenantID, userID string
	if orderType == billingdomain.OrderTypePlatformToTenant {
		if userType != 1 && userType != 2 {
			return nil, httpx.ErrForbidden.WithDetail("只有平台管理员才能进行租户充值")
		}
		tenantID = in.Body.TenantID
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("租户充值时 tenantId 必填")
		}
		var exists int
		if err := h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_tenants WHERE tenant_id = $1`, tenantID).Scan(&exists); err != nil || exists == 0 {
			return nil, httpx.ErrBadRequest.WithDetail("目标租户不存在")
		}
	} else {
		if in.Body.UserID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("用户充值时 userId 必填")
		}
		var userTenantID string
		if err := h.pool.QueryRow(ctx, `SELECT tenant_id FROM iam_accounts WHERE user_id = $1 AND user_type = 4 AND status <> 'deleted'`, in.Body.UserID).Scan(&userTenantID); err != nil || userTenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("用户不存在或无归属租户")
		}
		if userType == 3 && loginTenantID != "" && loginTenantID != userTenantID {
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

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	defer tx.Rollback(ctx)
	if userID != "" {
		var active bool
		if err := tx.QueryRow(ctx, `
			SELECT true FROM iam_accounts
			WHERE user_id = $1 AND tenant_id = $2 AND user_type = 4 AND status = 'active'
			FOR UPDATE
		`, userID, tenantID).Scan(&active); err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("用户不存在或已删除")
		}
	}

	grant, err := billingsvc.GrantBalance(ctx, tx, billingsvc.GrantParams{
		OrderType: orderType, TenantID: tenantID, UserID: userID,
		AmountMicroUSD: in.Body.AmountMicroUSD, PaidAmount: in.Body.PaidAmountMinor,
		PaymentRef: in.Body.PaymentRef, Note: in.Body.Note, OperatorID: claims.UserID,
		Source: pkgSource, ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
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
	// 租户用户只能撤销本租户的 tenant_to_user 充值
	if claims.UserType == 3 && claims.TenantID != "" {
		var orderTenantID, orderType string
		err := h.pool.QueryRow(ctx, `SELECT tenant_id, order_type FROM bill_recharge_orders WHERE order_id = $1`, in.OrderID).Scan(&orderTenantID, &orderType)
		if err != nil || orderTenantID != claims.TenantID || orderType != billingdomain.OrderTypeTenantToUser {
			return nil, httpx.ErrForbidden.WithDetail("只能撤销本租户的用户充值记录")
		}
	}

	result, err := h.deduction.ReverseOrder(in.OrderID, in.Body.Reason, claims.UserID)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &reverseRechargeOutput{}
	out.Body.Status = "SUCCESS"
	if result.IsPartial {
		out.Body.Status = "PARTIAL_REVERSAL"
	}
	out.Body.OrderID = result.OrderID
	out.Body.BalanceLotID = result.PackageID
	out.Body.ReversedAmountUSD = billingdomain.MicroToUSD(result.ReversedCredits)
	out.Body.OriginalAmountUSD = billingdomain.MicroToUSD(result.OriginalCredits)
	out.Body.LostAmountUSD = billingdomain.MicroToUSD(result.LostCredits)
	out.Body.BalanceLotStatus = result.PackageStatus
	return out, nil
}

func (h *adminHandlers) refund(ctx context.Context, in *refundInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	reason := in.Body.Reason
	if reason == "" {
		reason = "管理员手动退款"
	}
	if err := h.deduction.Refund(in.Body.EventID, reason, claims.UserID); err != nil {
		return nil, toProblem(err)
	}
	out := &messageOutput{}
	out.Body.Message = "退款成功"
	return out, nil
}

func (h *adminHandlers) getDebt(ctx context.Context, in *debtStatusInput) (*debtStatusOutput, error) {
	var debt int64
	var err error
	if in.OwnerType == "tenant" {
		err = h.pool.QueryRow(ctx, `SELECT COALESCE(current_overdraft,0) FROM iam_tenants WHERE tenant_id = $1`, in.AccountID).Scan(&debt)
	} else {
		err = h.pool.QueryRow(ctx, `SELECT COALESCE(current_overdraft,0) FROM iam_accounts WHERE user_id = $1 AND user_type = 4`, in.AccountID).Scan(&debt)
	}
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail("账户不存在")
	}
	out := &debtStatusOutput{}
	out.Body.OwnerType = in.OwnerType
	out.Body.AccountID = in.AccountID
	out.Body.OutstandingDebtMicroUSD = debt
	out.Body.ServiceState = "active"
	if debt > 0 {
		out.Body.ServiceState = "blocked_debt"
	}
	return out, nil
}

func (h *adminHandlers) authAuditLogs(ctx context.Context, in *auditLogsInput) (*auditLogsOutput, error) {
	where := "WHERE 1=1"
	args := []any{}
	idx := 1
	addFilter := func(col, val string) {
		if val != "" {
			where += fmt.Sprintf(" AND %s = $%d", col, idx)
			args = append(args, val)
			idx++
		}
	}
	addFilter("event_type", in.EventType)
	addFilter("principal_type", in.PrincipalType)
	addFilter("user_id", in.UserID)
	addFilter("decision", in.Decision)

	var total int64
	_ = h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM auth_audit_logs "+where, args...).Scan(&total)

	size := in.Size
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (in.Page - 1) * size
	qargs := append(append([]any{}, args...), size, offset)
	rows, err := h.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, event_type, principal_type, user_id,
		       decision, reason_code, reason_message, created_at
		FROM auth_audit_logs %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), qargs...)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	defer rows.Close()

	items := make([]auditLogItem, 0)
	for rows.Next() {
		var it auditLogItem
		var userID, reasonCode, reasonMsg *string
		var createdAt time.Time
		if err := rows.Scan(&it.ID, &it.EventType, &it.PrincipalType, &userID,
			&it.Decision, &reasonCode, &reasonMsg, &createdAt); err != nil {
			continue
		}
		it.CreatedAt = millisFromTime(createdAt)
		if userID != nil {
			it.UserID = *userID
		}
		if reasonCode != nil {
			it.ReasonCode = *reasonCode
		}
		if reasonMsg != nil {
			it.ReasonMessage = *reasonMsg
		}
		items = append(items, it)
	}
	return &auditLogsOutput{Body: httpx.NewPage(items, total, in.Page, size)}, nil
}
