package transport

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	billingsvc "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
	paymentsvc "xiaodou/dai/internal/payment/service"
	"xiaodou/dai/internal/payment/wechat"
	"xiaodou/dai/libs/go/httpx"
)

// adminPaymentHandlers 承载管理端支付配置、订单和提现端点（type 1,2）。
type adminPaymentHandlers struct {
	*paymentHandlers
	deduction *billingsvc.DeductionService
}

func newAdminPaymentHandlers(d Deps) *adminPaymentHandlers {
	return &adminPaymentHandlers{paymentHandlers: newPaymentHandlers(d), deduction: d.Deduction}
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

type adminRechargeOrdersInput struct {
	Keyword           string `query:"keyword" required:"false"`
	Method            string `query:"method" required:"false" enum:"manual,online"`
	TargetType        string `query:"targetType" required:"false" enum:"tenant,user"`
	PaymentStatus     string `query:"paymentStatus" required:"false"`
	FulfillmentStatus string `query:"fulfillmentStatus" required:"false"`
	RefundStatus      string `query:"refundStatus" required:"false" enum:"none,refunded,not_applicable"`
	TimeFrom          int64  `query:"timeFrom" required:"false"`
	TimeTo            int64  `query:"timeTo" required:"false"`
	Page              int    `query:"page" default:"1"`
	Size              int    `query:"size" default:"20"`
}

type adminRechargeOrdersOutput struct {
	Body httpx.Page[payment.AdminRechargeOrder]
}

type adminRechargeOrderInput struct {
	OrderID string `path:"orderId"`
}

type adminRechargeOrderOutput struct {
	Body payment.AdminRechargeOrder
}

type reverseAdminRechargeOrderInput struct {
	OrderID string `path:"orderId"`
	Body    struct {
		Reason string `json:"reason" minLength:"1"`
	}
}

type recordCompletedRefundInput struct {
	OrderID string `path:"orderId"`
	Body    struct {
		Method          string `json:"method" enum:"wechat,offline"`
		RefundReference string `json:"refundReference" minLength:"1" maxLength:"128"`
		ChannelRefundID string `json:"channelRefundId" required:"false" maxLength:"128"`
		RefundedAt      int64  `json:"refundedAt" minimum:"1"`
		Reason          string `json:"reason" minLength:"1" maxLength:"500"`
		Note            string `json:"note" required:"false" maxLength:"1000"`
	}
}

type syncOrderInput struct {
	OrderID string `path:"orderId"`
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

type createWithdrawalInput struct {
	Body struct {
		TenantID       string `json:"tenantId"`
		AmountMicroUSD int64  `json:"amountMicroUsd" minimum:"1"`
		AccountName    string `json:"accountName" required:"false"`
		BankName       string `json:"bankName" required:"false"`
		AccountNo      string `json:"accountNo" required:"false"`
		Note           string `json:"note" required:"false"`
		PaymentRef     string `json:"paymentRef" required:"false"`
	}
}

type withdrawalItem struct {
	WithdrawalID         string `json:"withdrawalId"`
	Currency             string `json:"currency"`
	AmountMicroUSD       int64  `json:"amountMicroUsd"`
	FeeAmountMicroUSD    int64  `json:"feeAmountMicroUsd"`
	PayoutAmountMicroUSD int64  `json:"payoutAmountMicroUsd"`
	AccountName          string `json:"accountName"`
	BankName             string `json:"bankName"`
	AccountNo            string `json:"accountNo"`
	Status               string `json:"status"`
	ApplyNote            string `json:"applyNote,omitempty"`
	ReviewNote           string `json:"reviewNote,omitempty"`
	PaymentRef           string `json:"paymentRef,omitempty"`
	PaidAt               *int64 `json:"paidAt,omitempty"`
	CreatedAt            int64  `json:"createdAt"`
}

type withdrawalOutput struct {
	Body withdrawalItem
}

func withdrawalToItem(w *payment.Withdrawal) withdrawalItem {
	return withdrawalItem{
		WithdrawalID: w.WithdrawalID, Currency: "USD", AmountMicroUSD: w.AmountMicroUSD,
		FeeAmountMicroUSD: w.FeeAmountMicroUSD, PayoutAmountMicroUSD: w.PayoutAmountMicroUSD, AccountName: w.AccountName,
		BankName: w.BankName, AccountNo: w.AccountNo, Status: w.Status,
		ApplyNote: w.ApplyNote, ReviewNote: w.ReviewNote, PaymentRef: w.PaymentRef,
		PaidAt: millisFromTimePtr(w.PaidAt), CreatedAt: millisFromTime(w.CreatedAt),
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
	huma.Register(api, huma.Operation{OperationID: "admin-list-recharge-orders", Method: http.MethodGet, Path: "/api/v1/admin/recharge-orders",
		Summary: "充值订单统一列表", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.listRechargeOrders)
	huma.Register(api, huma.Operation{OperationID: "admin-get-recharge-order", Method: http.MethodGet, Path: "/api/v1/admin/recharge-orders/{orderId}",
		Summary: "充值订单统一详情", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.getRechargeOrder)
	huma.Register(api, huma.Operation{OperationID: "admin-sync-recharge-order", Method: http.MethodPost, Path: "/api/v1/admin/recharge-orders/{orderId}/sync",
		Summary: "同步充值订单支付状态", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.syncRechargeOrder)
	huma.Register(api, huma.Operation{OperationID: "admin-reverse-recharge-order-credit", Method: http.MethodPost, Path: "/api/v1/admin/recharge-orders/{orderId}/reverse-credit",
		Summary: "撤回手动充值剩余额度", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.reverseRechargeOrderCredit)
	huma.Register(api, huma.Operation{OperationID: "admin-record-completed-recharge-refund", Method: http.MethodPost, Path: "/api/v1/admin/recharge-orders/{orderId}/refund-reversal",
		Summary: "登记已完成退款并整单冲正", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.recordCompletedRechargeRefund)

	huma.Register(api, huma.Operation{OperationID: "admin-list-balance-ledger", Method: http.MethodGet, Path: "/api/v1/admin/balance-ledger",
		Summary: "任意租户余额流水", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.listCashLedger)

	huma.Register(api, huma.Operation{OperationID: "admin-list-withdrawals", Method: http.MethodGet, Path: "/api/v1/admin/withdrawals",
		Summary: "提现记录列表", Tags: []string{"admin-payment"}, Middlewares: sysUser}, h.listWithdrawals)
	huma.Register(api, huma.Operation{OperationID: "admin-create-withdrawal", Method: http.MethodPost, Path: "/api/v1/admin/withdrawals",
		Summary: "创建提现记录并直接扣减租户额度", Tags: []string{"admin-payment"}, Middlewares: sysUser, DefaultStatus: http.StatusCreated}, h.createWithdrawal)
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
	list, total, err := h.svc.ListOrders(ctx, payment.ListOrdersParams{
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

func (h *adminPaymentHandlers) listRechargeOrders(ctx context.Context, in *adminRechargeOrdersInput) (*adminRechargeOrdersOutput, error) {
	p := payment.ListAdminRechargeOrdersParams{
		Keyword: in.Keyword, Method: in.Method, TargetType: in.TargetType,
		PaymentStatus: in.PaymentStatus, FulfillmentStatus: in.FulfillmentStatus,
		RefundStatus: in.RefundStatus,
		Page:         in.Page, Size: in.Size,
	}
	if in.TimeFrom > 0 {
		value := time.UnixMilli(in.TimeFrom).UTC()
		p.TimeFrom = &value
	}
	if in.TimeTo > 0 {
		value := time.UnixMilli(in.TimeTo).UTC()
		p.TimeTo = &value
	}
	items, total, err := h.svc.ListAdminRechargeOrders(ctx, p)
	if err != nil {
		return nil, httpx.ErrInternal.WithCause(err)
	}
	page, size := normalizePage(in.Page, in.Size)
	return &adminRechargeOrdersOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}

func (h *adminPaymentHandlers) getRechargeOrder(ctx context.Context, in *adminRechargeOrderInput) (*adminRechargeOrderOutput, error) {
	item, err := h.svc.GetAdminRechargeOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	return &adminRechargeOrderOutput{Body: *item}, nil
}

func (h *adminPaymentHandlers) syncRechargeOrder(ctx context.Context, in *adminRechargeOrderInput) (*adminRechargeOrderOutput, error) {
	item, err := h.svc.GetAdminRechargeOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	if item.Method != "online" {
		return nil, httpx.ErrBadRequest.WithDetail("手动充值不需要同步支付状态")
	}
	if _, err := h.svc.SyncOrder(ctx, item.OrderID); err != nil {
		return nil, toProblem(err)
	}
	return h.getRechargeOrder(ctx, in)
}

func (h *adminPaymentHandlers) reverseRechargeOrderCredit(ctx context.Context, in *reverseAdminRechargeOrderInput) (*adminRechargeOrderOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	item, err := h.svc.GetAdminRechargeOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	if item.BalanceOrderID == "" || item.FulfillmentStatus != payment.FulfillmentStatusCredited {
		return nil, toProblem(domain.ErrRechargeNotReversible)
	}
	if item.Method != "manual" {
		return nil, httpx.ErrBadRequest.WithDetail("在线充值必须在退款完成后执行整单冲正")
	}
	reason := strings.TrimSpace(in.Body.Reason)
	if reason == "" {
		return nil, httpx.ErrBadRequest.WithDetail("撤回原因不能为空")
	}
	if _, err := h.deduction.ReverseOrder(item.BalanceOrderID, reason, claims.UserID); err != nil {
		return nil, toProblem(err)
	}
	return h.getRechargeOrder(ctx, &adminRechargeOrderInput{OrderID: item.OrderID})
}

func (h *adminPaymentHandlers) recordCompletedRechargeRefund(ctx context.Context, in *recordCompletedRefundInput) (*adminRechargeOrderOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	item, err := h.svc.GetAdminRechargeOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	if item.Method != "online" {
		return nil, httpx.ErrBadRequest.WithDetail("手动充值没有支付退款流程")
	}
	if _, err := h.svc.RecordCompletedRefund(ctx, paymentsvc.RecordCompletedRefundParams{
		PaymentOrderID: item.OrderID,
		Method:         in.Body.Method, RefundReference: in.Body.RefundReference,
		ChannelRefundID: in.Body.ChannelRefundID, RefundedAt: time.UnixMilli(in.Body.RefundedAt).UTC(),
		Reason: in.Body.Reason, Note: in.Body.Note, OperatorID: claims.UserID,
	}); err != nil {
		return nil, toProblem(err)
	}
	return h.getRechargeOrder(ctx, &adminRechargeOrderInput{OrderID: item.OrderID})
}

func (h *adminPaymentHandlers) syncOrder(ctx context.Context, in *syncOrderInput) (*topupOrderStatusOutput, error) {
	order, err := h.svc.SyncOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &topupOrderStatusOutput{}
	out.Body.OrderID = order.OrderID
	out.Body.Status = order.Status
	out.Body.PaymentCurrency = order.PaymentCurrency
	out.Body.PaymentAmountMinor = order.PaymentAmountMinor
	out.Body.GrossAmountMicroUSD = order.GrossAmountMicroUSD
	out.Body.FeeAmountMicroUSD = order.FeeAmountMicroUSD
	out.Body.GiftAmountMicroUSD = order.GiftAmountMicroUSD
	out.Body.CreditedAmountMicroUSD = order.CreditedAmountMicroUSD
	out.Body.TransactionID = order.TransactionID
	out.Body.PaidAt = millisFromTimePtr(order.PaidAt)
	out.Body.BalanceExpiresAt = millisFromTimePtr(order.BalanceExpiresAt)
	return out, nil
}

func (h *adminPaymentHandlers) listCashLedger(ctx context.Context, in *adminCashLedgerInput) (*cashLedgerOutput, error) {
	list, total, err := h.svc.ListCashLedger(ctx, in.TenantID, in.TxnType, in.Page, in.Size)
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

func (h *adminPaymentHandlers) createWithdrawal(ctx context.Context, in *createWithdrawalInput) (*withdrawalOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	w, err := h.svc.CreateWithdrawal(ctx, paymentsvc.CreateWithdrawalParams{
		TenantID: in.Body.TenantID, AmountMicroUSD: in.Body.AmountMicroUSD,
		AccountName: in.Body.AccountName, BankName: in.Body.BankName, AccountNo: in.Body.AccountNo,
		Note: in.Body.Note, OperatorID: claims.UserID, PaymentRef: in.Body.PaymentRef,
	})
	if err != nil {
		return nil, toProblem(err)
	}
	return &withdrawalOutput{Body: withdrawalToItem(w)}, nil
}
