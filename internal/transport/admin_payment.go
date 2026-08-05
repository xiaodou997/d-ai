package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/httpx"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
	"xiaodou/dai/internal/payment/wechat"
)

// adminPaymentHandlers 承载管理端支付配置/订单/提现/现金总览端点（type 1,2）。
type adminPaymentHandlers struct {
	*paymentHandlers
}

func newAdminPaymentHandlers(d Deps) *adminPaymentHandlers {
	return &adminPaymentHandlers{paymentHandlers: newPaymentHandlers(d)}
}

// ---- DTO：平台充值/提现规则 ----

type globalPaymentSettingsOutput struct {
	Body payment.GlobalSettings
}

type updateGlobalPaymentSettingsInput struct {
	Body payment.GlobalSettings
}

// ---- DTO：微信商户配置 ----

type wechatConfigOutput struct {
	Body struct {
		Enabled            bool   `json:"enabled"`
		Mock               bool   `json:"mock"`
		VerifyMode         string `json:"verifyMode"`
		AppID              string `json:"appId"`
		MchID              string `json:"mchId"`
		MchCertSerialNo    string `json:"mchCertSerialNo"`
		NotifyBaseURL      string `json:"notifyBaseUrl"`
		OrderTTLSeconds    int    `json:"orderTtlSeconds"`
		HasPrivateKey      bool   `json:"hasPrivateKey"`
		HasAPIv3Key        bool   `json:"hasApiv3Key"`
		WechatPublicKeyID  string `json:"wechatPayPublicKeyId"`
		HasWechatPublicKey bool   `json:"hasWechatPayPublicKey"`
	}
}

type updateWechatConfigInput struct {
	Body struct {
		Enabled           bool    `json:"enabled"`
		Mock              bool    `json:"mock"`
		VerifyMode        string  `json:"verifyMode"`
		AppID             string  `json:"appId"`
		MchID             string  `json:"mchId"`
		MchCertSerialNo   string  `json:"mchCertSerialNo"`
		NotifyBaseURL     string  `json:"notifyBaseUrl"`
		OrderTTLSeconds   int     `json:"orderTtlSeconds"`
		MchPrivateKey     *string `json:"mchPrivateKey" doc:"留空=不修改现有私钥"`
		APIv3Key          *string `json:"apiv3Key" doc:"留空=不修改现有 APIv3Key"`
		WechatPublicKeyID *string `json:"wechatPayPublicKeyId" doc:"留空=不修改现有微信支付公钥 ID"`
		WechatPublicKey   *string `json:"wechatPayPublicKey" doc:"留空=不修改现有微信支付公钥"`
	}
}

// ---- DTO：订单/现金/提现管理 ----

type adminPaymentOrdersInput struct {
	Scene    string `query:"scene" required:"false"`
	Status   string `query:"status" required:"false"`
	TenantID string `query:"tenantId" required:"false"`
	Page     int    `query:"page" default:"1"`
	Size     int    `query:"size" default:"20"`
}

type adminPaymentOrdersOutput struct {
	Body httpx.Page[topupOrderItem]
}

type syncOrderInput struct {
	OrderID string `path:"orderId"`
}

type cashAccountsInput struct {
	Page int `query:"page" default:"1"`
	Size int `query:"size" default:"20"`
}

type cashAccountItem struct {
	TenantID   string `json:"tenantId"`
	TenantName string `json:"tenantName,omitempty"`
	Balance    int64  `json:"balance"`
	Frozen     int64  `json:"frozen"`
	Available  int64  `json:"available"`
}

type cashAccountsOutput struct {
	Body httpx.Page[cashAccountItem]
}

type adminCashLedgerInput struct {
	TenantID string `query:"tenantId"`
	TxnType  string `query:"txnType" required:"false"`
	Page     int    `query:"page" default:"1"`
	Size     int    `query:"size" default:"20"`
}

type adminWithdrawalsInput struct {
	Status string `query:"status" required:"false"`
	Page   int    `query:"page" default:"1"`
	Size   int    `query:"size" default:"20"`
}

type adminWithdrawalsOutput struct {
	Body httpx.Page[withdrawalItem]
}

type reviewWithdrawalInput struct {
	WithdrawalID string `path:"id"`
	Body         struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note" required:"false"`
	}
}

type settleWithdrawalInput struct {
	WithdrawalID string `path:"id"`
	Body         struct {
		PaymentRef string `json:"paymentRef"`
	}
}

// registerAdminPayment 注册管理端支付相关端点（type 1,2）。
func registerAdminPayment(api huma.API, d Deps) {
	h := newAdminPaymentHandlers(d)
	ua := userAuth(api, d.JWT, d.Blacklist)
	sysUser := huma.Middlewares{ua, requireUserType(api, 1, 2)}

	huma.Register(api, huma.Operation{OperationID: "admin-get-payment-settings", Method: http.MethodGet, Path: "/api/v1/admin/payment-settings",
		Summary: "平台充值与提现规则", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.getGlobalSettings)
	huma.Register(api, huma.Operation{OperationID: "admin-update-payment-settings", Method: http.MethodPut, Path: "/api/v1/admin/payment-settings",
		Summary: "更新平台充值与提现规则", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.updateGlobalSettings)

	huma.Register(api, huma.Operation{OperationID: "admin-get-wechat-config", Method: http.MethodGet, Path: "/api/v1/admin/wechat-config",
		Summary: "微信商户配置（脱敏）", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.getWechatConfig)
	huma.Register(api, huma.Operation{OperationID: "admin-update-wechat-config", Method: http.MethodPut, Path: "/api/v1/admin/wechat-config",
		Summary: "更新微信商户配置（开关/商户号/密钥）", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.updateWechatConfig)

	huma.Register(api, huma.Operation{OperationID: "admin-list-payment-orders", Method: http.MethodGet, Path: "/api/v1/admin/payment-orders",
		Summary: "在线支付订单列表", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.listOrders)
	huma.Register(api, huma.Operation{OperationID: "admin-sync-payment-order", Method: http.MethodPost, Path: "/api/v1/admin/payment-orders/{orderId}/sync",
		Summary: "手动查单同步（mock 模式下即仿真支付成功）", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.syncOrder)

	huma.Register(api, huma.Operation{OperationID: "admin-list-cash-accounts", Method: http.MethodGet, Path: "/api/v1/admin/cash-accounts",
		Summary: "租户现金账户总览", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.listCashAccounts)
	huma.Register(api, huma.Operation{OperationID: "admin-list-cash-ledger", Method: http.MethodGet, Path: "/api/v1/admin/cash-ledger",
		Summary: "任意租户现金流水", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.listCashLedger)

	huma.Register(api, huma.Operation{OperationID: "admin-list-withdrawals", Method: http.MethodGet, Path: "/api/v1/admin/withdrawals",
		Summary: "提现申请列表", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.listWithdrawals)
	huma.Register(api, huma.Operation{OperationID: "admin-review-withdrawal", Method: http.MethodPost, Path: "/api/v1/admin/withdrawals/{id}/review",
		Summary: "审核提现申请", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.reviewWithdrawal)
	huma.Register(api, huma.Operation{OperationID: "admin-settle-withdrawal", Method: http.MethodPost, Path: "/api/v1/admin/withdrawals/{id}/settle",
		Summary: "线下打款核销提现", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.settleWithdrawal)
}

func (h *adminPaymentHandlers) getGlobalSettings(ctx context.Context, _ *struct{}) (*globalPaymentSettingsOutput, error) {
	g, err := h.svc.GetGlobalSettings(ctx)
	if err != nil {
		return nil, toProblem(err)
	}
	return &globalPaymentSettingsOutput{Body: *g}, nil
}

func (h *adminPaymentHandlers) updateGlobalSettings(ctx context.Context, in *updateGlobalPaymentSettingsInput) (*globalPaymentSettingsOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	g := in.Body
	if err := h.svc.UpdateGlobalSettings(ctx, &g, claims.UserID); err != nil {
		return nil, toProblem(err)
	}
	return &globalPaymentSettingsOutput{Body: g}, nil
}

func (h *adminPaymentHandlers) getWechatConfig(ctx context.Context, _ *struct{}) (*wechatConfigOutput, error) {
	v, err := h.svc.GetWechatConfigView(ctx)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &wechatConfigOutput{}
	out.Body.Enabled = v.Enabled
	out.Body.Mock = v.Mock
	out.Body.VerifyMode = v.VerifyMode
	out.Body.AppID = v.AppID
	out.Body.MchID = v.MchID
	out.Body.MchCertSerialNo = v.MchCertSerialNo
	out.Body.NotifyBaseURL = v.NotifyBaseURL
	out.Body.OrderTTLSeconds = v.OrderTTLSeconds
	out.Body.HasPrivateKey = v.HasPrivateKey
	out.Body.HasAPIv3Key = v.HasAPIv3Key
	out.Body.WechatPublicKeyID = v.WechatPublicKeyID
	out.Body.HasWechatPublicKey = v.HasWechatPublicKey
	return out, nil
}

func (h *adminPaymentHandlers) updateWechatConfig(ctx context.Context, in *updateWechatConfigInput) (*wechatConfigOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	err := h.svc.UpdateWechatConfig(ctx, wechat.UpdateInput{
		Enabled: in.Body.Enabled, Mock: in.Body.Mock, VerifyMode: in.Body.VerifyMode, AppID: in.Body.AppID, MchID: in.Body.MchID,
		MchCertSerialNo: in.Body.MchCertSerialNo, NotifyBaseURL: in.Body.NotifyBaseURL,
		OrderTTLSeconds: in.Body.OrderTTLSeconds, MchPrivateKey: in.Body.MchPrivateKey, APIv3Key: in.Body.APIv3Key,
		WechatPublicKeyID: in.Body.WechatPublicKeyID, WechatPublicKey: in.Body.WechatPublicKey,
	}, claims.UserID)
	if err != nil {
		return nil, toProblem(err)
	}
	return h.getWechatConfig(ctx, nil)
}

func (h *adminPaymentHandlers) listOrders(ctx context.Context, in *adminPaymentOrdersInput) (*adminPaymentOrdersOutput, error) {
	list, total, err := h.svc.ListOrders(ctx, paymentpg.ListOrdersParams{
		Scene: in.Scene, Status: in.Status, TenantID: in.TenantID, Page: in.Page, Size: in.Size,
	})
	if err != nil {
		return nil, toProblem(err)
	}
	items := make([]topupOrderItem, 0, len(list))
	for _, o := range list {
		items = append(items, orderToItem(o))
	}
	page, size := normalizePage(in.Page, in.Size)
	return &adminPaymentOrdersOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *adminPaymentHandlers) syncOrder(ctx context.Context, in *syncOrderInput) (*topupOrderStatusOutput, error) {
	order, err := h.svc.SyncOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &topupOrderStatusOutput{}
	out.Body.OrderID = order.OrderID
	out.Body.Status = order.Status
	out.Body.Amount = order.Amount
	out.Body.CreditAmount = order.CreditAmount
	out.Body.TransactionID = order.TransactionID
	out.Body.PaidAt = millisFromTimePtr(order.PaidAt)
	return out, nil
}

func (h *adminPaymentHandlers) listCashAccounts(ctx context.Context, in *cashAccountsInput) (*cashAccountsOutput, error) {
	list, total, err := h.svc.ListCashAccounts(ctx, in.Page, in.Size)
	if err != nil {
		return nil, toProblem(err)
	}
	items := make([]cashAccountItem, 0, len(list))
	for _, a := range list {
		items = append(items, cashAccountItem{
			TenantID: a.TenantID, TenantName: a.TenantName, Balance: a.Balance, Frozen: a.Frozen, Available: a.Available(),
		})
	}
	page, size := normalizePage(in.Page, in.Size)
	return &cashAccountsOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *adminPaymentHandlers) listCashLedger(ctx context.Context, in *adminCashLedgerInput) (*cashLedgerOutput, error) {
	list, total, err := h.svc.ListCashLedger(ctx, in.TenantID, in.TxnType, in.Page, in.Size)
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

func (h *adminPaymentHandlers) listWithdrawals(ctx context.Context, in *adminWithdrawalsInput) (*adminWithdrawalsOutput, error) {
	list, total, err := h.svc.ListWithdrawals(ctx, "", in.Status, in.Page, in.Size)
	if err != nil {
		return nil, toProblem(err)
	}
	items := make([]withdrawalItem, 0, len(list))
	for _, w := range list {
		items = append(items, withdrawalToItem(w))
	}
	page, size := normalizePage(in.Page, in.Size)
	return &adminWithdrawalsOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *adminPaymentHandlers) reviewWithdrawal(ctx context.Context, in *reviewWithdrawalInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if err := h.svc.ReviewWithdrawal(ctx, in.WithdrawalID, in.Body.Approve, claims.UserID, in.Body.Note); err != nil {
		return nil, toProblem(err)
	}
	out := &messageOutput{}
	out.Body.Message = "已处理"
	return out, nil
}

func (h *adminPaymentHandlers) settleWithdrawal(ctx context.Context, in *settleWithdrawalInput) (*messageOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if err := h.svc.SettleWithdrawal(ctx, in.WithdrawalID, claims.UserID, in.Body.PaymentRef); err != nil {
		return nil, toProblem(err)
	}
	out := &messageOutput{}
	out.Body.Message = "已核销"
	return out, nil
}
