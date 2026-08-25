package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/payment"
	paymentsvc "xiaodou/dai/internal/payment/service"
	"xiaodou/dai/libs/go/httpx"
)

// tenantCashHandlers 承载租户统一 USD 额度余额和额度明细端点（type=3 限本租户）。
type tenantCashHandlers struct {
	svc *paymentsvc.PaymentService
}

func newTenantCashHandlers(d Deps) *tenantCashHandlers {
	return &tenantCashHandlers{svc: d.Payment}
}

// ---- DTO ----

type cashAccountOutput struct {
	Body struct {
		Currency        string `json:"currency"`
		BalanceMicroUSD int64  `json:"balanceMicroUsd"`
	}
}

type cashLedgerInput struct {
	TxnType string `query:"txnType" required:"false"`
	Page    int    `query:"page" default:"1"`
	Size    int    `query:"size" default:"20"`
}

type cashLedgerItem struct {
	TxnID                string `json:"txnId"`
	TxnType              string `json:"txnType"`
	Currency             string `json:"currency"`
	AmountMicroUSD       int64  `json:"amountMicroUsd"`
	BalanceAfterMicroUSD int64  `json:"balanceAfterMicroUsd"`
	RefType              string `json:"refType,omitempty"`
	RefID                string `json:"refId,omitempty"`
	Note                 string `json:"note,omitempty"`
	CreatedAt            int64  `json:"createdAt"`
}

type cashLedgerOutput struct {
	Body httpx.Page[cashLedgerItem]
}

type tenantPaymentSettingsOutput struct {
	Body payment.TenantSettings
}

type updateTenantPaymentSettingsInput struct {
	Body payment.TenantSettings
}

// registerTenantCash 注册租户统一额度端点（type=3 限本租户）。
func registerTenantCash(api huma.API, d Deps) {
	h := newTenantCashHandlers(d)
	tenantOnly := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist), requireCapability(api, auth.CapabilityTenantSelf)}

	huma.Register(api, huma.Operation{OperationID: "tenant-balance", Method: http.MethodGet, Path: "/api/v1/tenant/balance",
		Summary: "租户 USD 额度余额", Tags: []string{"tenant-balance"}, Middlewares: tenantOnly}, h.getAccount)
	huma.Register(api, huma.Operation{OperationID: "tenant-balance-ledger", Method: http.MethodGet, Path: "/api/v1/tenant/balance-ledger",
		Summary: "额度明细流水", Tags: []string{"tenant-balance"}, Middlewares: tenantOnly}, h.listLedger)
	huma.Register(api, huma.Operation{OperationID: "tenant-get-payment-settings", Method: http.MethodGet, Path: "/api/v1/tenant/payment-settings",
		Summary: "用户充值设置", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.getPaymentSettings)
	huma.Register(api, huma.Operation{OperationID: "tenant-update-payment-settings", Method: http.MethodPut, Path: "/api/v1/tenant/payment-settings",
		Summary: "更新用户充值设置", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.updatePaymentSettings)
}

func (h *tenantCashHandlers) getAccount(ctx context.Context, _ *struct{}) (*cashAccountOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	account, err := h.svc.GetBalanceAccount(ctx, claims.TenantID)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &cashAccountOutput{}
	out.Body.Currency = "USD"
	out.Body.BalanceMicroUSD = account.BalanceMicroUSD
	return out, nil
}

func (h *tenantCashHandlers) listLedger(ctx context.Context, in *cashLedgerInput) (*cashLedgerOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	list, total, err := h.svc.ListCashLedger(ctx, claims.TenantID, in.TxnType, in.Page, in.Size)
	if err != nil {
		return nil, toProblem(err)
	}
	items := make([]cashLedgerItem, 0, len(list))
	for _, e := range list {
		items = append(items, cashLedgerItem{
			TxnID: e.TxnID, TxnType: e.TxnType, Currency: "USD",
			AmountMicroUSD: e.AmountMicroUSD, BalanceAfterMicroUSD: e.BalanceAfterMicroUSD,
			RefType: e.RefType, RefID: e.RefID, Note: e.Note, CreatedAt: millisFromTime(e.CreatedAt),
		})
	}
	page, size := normalizePage(in.Page, in.Size)
	return &cashLedgerOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *tenantCashHandlers) getPaymentSettings(ctx context.Context, _ *struct{}) (*tenantPaymentSettingsOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	settings, err := h.svc.GetTenantPaymentSettings(ctx, claims.TenantID)
	if err != nil {
		return nil, toProblem(err)
	}
	return &tenantPaymentSettingsOutput{Body: *settings}, nil
}

func (h *tenantCashHandlers) updatePaymentSettings(ctx context.Context, in *updateTenantPaymentSettingsInput) (*tenantPaymentSettingsOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	settings := in.Body
	if err := h.svc.UpdateTenantPaymentSettings(ctx, claims.TenantID, &settings, claims.UserID); err != nil {
		return nil, toProblem(err)
	}
	return &tenantPaymentSettingsOutput{Body: settings}, nil
}
