package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/httpx"
	billingdomain "xiaodou/dai/internal/billing"
	"xiaodou/dai/internal/payment"
	paymentsvc "xiaodou/dai/internal/payment/service"
)

// tenantCashHandlers 承载租户现金账户/余额购积分/提现自助端点（type=3 限本租户）。
type tenantCashHandlers struct {
	svc *paymentsvc.PaymentService
}

func newTenantCashHandlers(d Deps) *tenantCashHandlers {
	return &tenantCashHandlers{svc: d.Payment}
}

// ---- DTO ----

type cashAccountOutput struct {
	Body struct {
		Balance       int64 `json:"balance"`
		Frozen        int64 `json:"frozen"`
		Available     int64 `json:"available"`
		CreditsPerCNY int64 `json:"creditsPerCny"`
		WithdrawFeeBp int   `json:"withdrawFeeBp"`
	}
}

type cashLedgerInput struct {
	TxnType string `query:"txnType" required:"false"`
	Page    int    `query:"page" default:"1"`
	Size    int    `query:"size" default:"20"`
}

type cashLedgerItem struct {
	TxnID        string `json:"txnId"`
	TxnType      string `json:"txnType"`
	Amount       int64  `json:"amount"`
	BalanceAfter int64  `json:"balanceAfter"`
	RefType      string `json:"refType,omitempty"`
	RefID        string `json:"refId,omitempty"`
	Note         string `json:"note,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
}

type cashLedgerOutput struct {
	Body httpx.Page[cashLedgerItem]
}

type buyCreditsInput struct {
	Body struct {
		Amount int64 `json:"amount" minimum:"1" doc:"用现金余额购买租户积分，单位分"`
	}
}

type buyCreditsOutput struct {
	Body struct {
		CreditOrderID string  `json:"creditOrderId"`
		Credits       float64 `json:"credits"`
	}
}

type tenantPaymentSettingsOutput struct {
	Body payment.TenantSettings
}

type updateTenantPaymentSettingsInput struct {
	Body payment.TenantSettings
}

type applyWithdrawalInput struct {
	Body struct {
		Amount      int64  `json:"amount" minimum:"1"`
		AccountName string `json:"accountName"`
		BankName    string `json:"bankName"`
		AccountNo   string `json:"accountNo"`
		Note        string `json:"note" required:"false"`
	}
}

type withdrawalItem struct {
	WithdrawalID string `json:"withdrawalId"`
	Amount       int64  `json:"amount"`
	FeeAmount    int64  `json:"feeAmount"`
	PayoutAmount int64  `json:"payoutAmount"`
	AccountName  string `json:"accountName"`
	BankName     string `json:"bankName"`
	AccountNo    string `json:"accountNo"`
	Status       string `json:"status"`
	ApplyNote    string `json:"applyNote,omitempty"`
	ReviewNote   string `json:"reviewNote,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
}

type withdrawalOutput struct {
	Body withdrawalItem
}

type listWithdrawalsInput struct {
	Status string `query:"status" required:"false"`
	Page   int    `query:"page" default:"1"`
	Size   int    `query:"size" default:"20"`
}

type listWithdrawalsOutput struct {
	Body httpx.Page[withdrawalItem]
}

type cancelWithdrawalInput struct {
	WithdrawalID string `path:"id"`
}

func withdrawalToItem(w *payment.Withdrawal) withdrawalItem {
	return withdrawalItem{
		WithdrawalID: w.WithdrawalID, Amount: w.Amount, FeeAmount: w.FeeAmount, PayoutAmount: w.PayoutAmount, AccountName: w.AccountName,
		BankName: w.BankName, AccountNo: w.AccountNo, Status: w.Status,
		ApplyNote: w.ApplyNote, ReviewNote: w.ReviewNote, CreatedAt: millisFromTime(w.CreatedAt),
	}
}

// registerTenantCash 注册租户现金账户端点（type=3 限本租户）。
func registerTenantCash(api huma.API, d Deps) {
	h := newTenantCashHandlers(d)
	tenantOnly := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist), requireUserType(api, 3)}

	huma.Register(api, huma.Operation{OperationID: "tenant-cash-account", Method: http.MethodGet, Path: "/api/v1/tenant/cash-account",
		Summary: "租户现金账户余额", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.getAccount)
	huma.Register(api, huma.Operation{OperationID: "tenant-cash-ledger", Method: http.MethodGet, Path: "/api/v1/tenant/cash-ledger",
		Summary: "现金流水", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.listLedger)
	huma.Register(api, huma.Operation{OperationID: "tenant-cash-buy-credits", Method: http.MethodPost, Path: "/api/v1/tenant/cash/buy-credits",
		Summary: "用现金余额购买租户积分", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.buyCredits)
	huma.Register(api, huma.Operation{OperationID: "tenant-get-payment-settings", Method: http.MethodGet, Path: "/api/v1/tenant/payment-settings",
		Summary: "用户充值设置", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.getPaymentSettings)
	huma.Register(api, huma.Operation{OperationID: "tenant-update-payment-settings", Method: http.MethodPut, Path: "/api/v1/tenant/payment-settings",
		Summary: "更新用户充值设置", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.updatePaymentSettings)
	huma.Register(api, huma.Operation{OperationID: "tenant-apply-withdrawal", Method: http.MethodPost, Path: "/api/v1/tenant/withdrawals",
		Summary: "申请提现", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly, DefaultStatus: http.StatusCreated}, h.applyWithdrawal)
	huma.Register(api, huma.Operation{OperationID: "tenant-list-withdrawals", Method: http.MethodGet, Path: "/api/v1/tenant/withdrawals",
		Summary: "提现记录", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.listWithdrawals)
	huma.Register(api, huma.Operation{OperationID: "tenant-cancel-withdrawal", Method: http.MethodPost, Path: "/api/v1/tenant/withdrawals/{id}/cancel",
		Summary: "取消提现申请（仅 pending 可取消）", Tags: []string{"tenant-cash"}, Middlewares: tenantOnly}, h.cancelWithdrawal)
}

func (h *tenantCashHandlers) getAccount(ctx context.Context, _ *struct{}) (*cashAccountOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	account, err := h.svc.GetCashAccount(ctx, claims.TenantID)
	if err != nil {
		return nil, toProblem(err)
	}
	settings, err := h.svc.GetGlobalSettings(ctx)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &cashAccountOutput{}
	out.Body.Balance = account.Balance
	out.Body.Frozen = account.Frozen
	out.Body.Available = account.Available()
	out.Body.CreditsPerCNY = settings.CreditsPerCNY
	out.Body.WithdrawFeeBp = settings.TenantWithdrawFeeBp
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
			TxnID: e.TxnID, TxnType: e.TxnType, Amount: e.Amount, BalanceAfter: e.BalanceAfter,
			RefType: e.RefType, RefID: e.RefID, Note: e.Note, CreatedAt: millisFromTime(e.CreatedAt),
		})
	}
	page, size := normalizePage(in.Page, in.Size)
	return &cashLedgerOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *tenantCashHandlers) buyCredits(ctx context.Context, in *buyCreditsInput) (*buyCreditsOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	grant, err := h.svc.BuyCredits(ctx, claims.TenantID, in.Body.Amount, claims.UserID)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &buyCreditsOutput{}
	out.Body.CreditOrderID = grant.OrderID
	out.Body.Credits = billingdomain.MicroToCredits(grant.PackageMicro)
	return out, nil
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

func (h *tenantCashHandlers) applyWithdrawal(ctx context.Context, in *applyWithdrawalInput) (*withdrawalOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	w, err := h.svc.ApplyWithdrawal(ctx, paymentsvc.ApplyWithdrawalParams{
		TenantID: claims.TenantID, Amount: in.Body.Amount, AccountName: in.Body.AccountName,
		BankName: in.Body.BankName, AccountNo: in.Body.AccountNo, Note: in.Body.Note, AppliedBy: claims.UserID,
	})
	if err != nil {
		return nil, toProblem(err)
	}
	return &withdrawalOutput{Body: withdrawalToItem(w)}, nil
}

func (h *tenantCashHandlers) listWithdrawals(ctx context.Context, in *listWithdrawalsInput) (*listWithdrawalsOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	list, total, err := h.svc.ListWithdrawals(ctx, claims.TenantID, in.Status, in.Page, in.Size)
	if err != nil {
		return nil, toProblem(err)
	}
	items := make([]withdrawalItem, 0, len(list))
	for _, w := range list {
		items = append(items, withdrawalToItem(w))
	}
	page, size := normalizePage(in.Page, in.Size)
	return &listWithdrawalsOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *tenantCashHandlers) cancelWithdrawal(ctx context.Context, in *cancelWithdrawalInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if err := h.svc.CancelWithdrawal(ctx, in.WithdrawalID, claims.TenantID); err != nil {
		return nil, toProblem(err)
	}
	out := &messageOutput{}
	out.Body.Message = "已取消"
	return out, nil
}
