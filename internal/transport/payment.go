package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
	paymentsvc "xiaodou/dai/internal/payment/service"
	"xiaodou/dai/libs/go/httpx"
)

// paymentHandlers 承载用户和租户的 USD 在线充值端点。scene 由 userType 决定，
// 不接受调用方指定。
type paymentHandlers struct {
	svc *paymentsvc.PaymentService
}

func newPaymentHandlers(d Deps) *paymentHandlers {
	return &paymentHandlers{svc: d.Payment}
}

func millisFromTimePtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	ms := t.UTC().UnixMilli()
	return &ms
}

func sceneAndScopeFromClaims(ctx context.Context) (scene, tenantID, userID string, ok bool) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return "", "", "", false
	}
	switch claims.UserType {
	case 4:
		return payment.SceneUserTopup, claims.TenantID, claims.UserID, true
	case 3:
		return payment.SceneTenantTopup, claims.TenantID, "", true
	default:
		return "", "", "", false
	}
}

// ---- DTO ----

type topupConfigOutput struct {
	Body struct {
		Enabled      bool                   `json:"enabled"`
		Currency     string                 `json:"currency"`
		FeeRateBp    int                    `json:"feeRateBp"`
		MinMicroUSD  int64                  `json:"minMicroUsd"`
		MaxMicroUSD  int64                  `json:"maxMicroUsd"`
		ValidityDays *int32                 `json:"validityDays,omitempty"`
		Packages     []payment.TopupPackage `json:"packages"`
	}
}

type createTopupOrderInput struct {
	Body struct {
		AmountMicroUSD int64  `json:"amountMicroUsd" required:"false" doc:"自定义充值金额，单位 micro-USD；必须可精确换算为美分"`
		PackageID      string `json:"packageId" required:"false" doc:"快捷额度包 ID"`
	}
}

type topupOrderOutput struct {
	Body struct {
		OrderID                string `json:"orderId"`
		CodeURL                string `json:"codeUrl"`
		PaymentCurrency        string `json:"paymentCurrency"`
		PaymentAmountMinor     int64  `json:"paymentAmountMinor"`
		GrossAmountMicroUSD    int64  `json:"grossAmountMicroUsd"`
		FeeAmountMicroUSD      int64  `json:"feeAmountMicroUsd"`
		GiftAmountMicroUSD     int64  `json:"giftAmountMicroUsd"`
		CreditedAmountMicroUSD int64  `json:"creditedAmountMicroUsd"`
		TopupMode              string `json:"topupMode"`
		PackageName            string `json:"packageName,omitempty"`
		Status                 string `json:"status"`
		ExpiresAt              int64  `json:"expiresAt"`
		BalanceExpiresAt       *int64 `json:"balanceExpiresAt,omitempty"`
	}
}

type getTopupOrderInput struct {
	OrderID string `path:"orderId"`
}

type topupOrderStatusOutput struct {
	Body struct {
		OrderID                string `json:"orderId"`
		Status                 string `json:"status"`
		PaymentCurrency        string `json:"paymentCurrency"`
		PaymentAmountMinor     int64  `json:"paymentAmountMinor"`
		GrossAmountMicroUSD    int64  `json:"grossAmountMicroUsd"`
		FeeAmountMicroUSD      int64  `json:"feeAmountMicroUsd"`
		GiftAmountMicroUSD     int64  `json:"giftAmountMicroUsd"`
		CreditedAmountMicroUSD int64  `json:"creditedAmountMicroUsd"`
		TopupMode              string `json:"topupMode"`
		PackageName            string `json:"packageName,omitempty"`
		TransactionID          string `json:"transactionId,omitempty"`
		PaidAt                 *int64 `json:"paidAt,omitempty"`
		BalanceExpiresAt       *int64 `json:"balanceExpiresAt,omitempty"`
	}
}

type listTopupOrdersInput struct {
	Status string `query:"status" required:"false"`
	Page   int    `query:"page" default:"1"`
	Size   int    `query:"size" default:"20"`
}

type topupOrderItem struct {
	OrderID                string `json:"orderId"`
	Scene                  string `json:"scene"`
	TenantName             string `json:"tenantName,omitempty"`
	Username               string `json:"username,omitempty"`
	Status                 string `json:"status"`
	PaymentCurrency        string `json:"paymentCurrency"`
	PaymentAmountMinor     int64  `json:"paymentAmountMinor"`
	GrossAmountMicroUSD    int64  `json:"grossAmountMicroUsd"`
	FeeAmountMicroUSD      int64  `json:"feeAmountMicroUsd"`
	GiftAmountMicroUSD     int64  `json:"giftAmountMicroUsd"`
	CreditedAmountMicroUSD int64  `json:"creditedAmountMicroUsd"`
	TopupMode              string `json:"topupMode"`
	PackageName            string `json:"packageName,omitempty"`
	TransactionID          string `json:"transactionId,omitempty"`
	CreatedAt              int64  `json:"createdAt"`
	PaidAt                 *int64 `json:"paidAt,omitempty"`
	BalanceExpiresAt       *int64 `json:"balanceExpiresAt,omitempty"`
}

type listTopupOrdersOutput struct {
	Body httpx.Page[topupOrderItem]
}

func orderToItem(o *payment.Order) topupOrderItem {
	return topupOrderItem{
		OrderID: o.OrderID, Scene: o.Scene, TenantName: o.TenantName, Username: o.Username, Status: o.Status,
		PaymentCurrency: o.PaymentCurrency, PaymentAmountMinor: o.PaymentAmountMinor,
		GrossAmountMicroUSD: o.GrossAmountMicroUSD, FeeAmountMicroUSD: o.FeeAmountMicroUSD,
		GiftAmountMicroUSD: o.GiftAmountMicroUSD, CreditedAmountMicroUSD: o.CreditedAmountMicroUSD,
		TopupMode: o.TopupMode, PackageName: o.PackageName, TransactionID: o.TransactionID,
		CreatedAt: millisFromTime(o.CreatedAt), PaidAt: millisFromTimePtr(o.PaidAt),
		BalanceExpiresAt: millisFromTimePtr(o.BalanceExpiresAt),
	}
}

// registerPayment 注册在线充值下单/查单端点（userAuth + requireUserType(3,4)）。
func registerPayment(api huma.API, d Deps) {
	h := newPaymentHandlers(d)
	authed := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist), requireUserType(api, 3, 4)}

	huma.Register(api, huma.Operation{OperationID: "payment-topup-config", Method: http.MethodGet, Path: "/api/v1/payments/topup-config",
		Summary: "USD 在线充值配置", Tags: []string{"payment"}, Middlewares: authed}, h.topupConfig)
	huma.Register(api, huma.Operation{OperationID: "payment-create-topup-order", Method: http.MethodPost, Path: "/api/v1/payments/topup-orders",
		Summary: "发起在线充值（微信 Native 扫码）", Tags: []string{"payment"}, Middlewares: authed, DefaultStatus: http.StatusCreated}, h.createOrder)
	huma.Register(api, huma.Operation{OperationID: "payment-get-topup-order", Method: http.MethodGet, Path: "/api/v1/payments/topup-orders/{orderId}",
		Summary: "查询充值订单状态（轮询用，仅读本地）", Tags: []string{"payment"}, Middlewares: authed}, h.getOrder)
	huma.Register(api, huma.Operation{OperationID: "payment-list-topup-orders", Method: http.MethodGet, Path: "/api/v1/payments/topup-orders",
		Summary: "我的在线充值订单", Tags: []string{"payment"}, Middlewares: authed}, h.listOrders)
}

func (h *paymentHandlers) topupConfig(ctx context.Context, _ *struct{}) (*topupConfigOutput, error) {
	scene, tenantID, _, ok := sceneAndScopeFromClaims(ctx)
	if !ok {
		return nil, httpx.ErrForbidden
	}
	view, err := h.svc.GetTopupConfigView(ctx, scene, tenantID)
	if err != nil {
		return nil, toProblem(err)
	}
	out := &topupConfigOutput{}
	out.Body.Enabled = view.Enabled
	out.Body.Currency = view.Currency
	out.Body.FeeRateBp = view.FeeRateBp
	out.Body.MinMicroUSD = view.MinMicroUSD
	out.Body.MaxMicroUSD = view.MaxMicroUSD
	out.Body.ValidityDays = view.ValidityDays
	out.Body.Packages = view.Packages
	return out, nil
}

func (h *paymentHandlers) createOrder(ctx context.Context, in *createTopupOrderInput) (*topupOrderOutput, error) {
	scene, tenantID, userID, ok := sceneAndScopeFromClaims(ctx)
	if !ok {
		return nil, httpx.ErrForbidden
	}
	order, err := h.svc.CreateTopupOrder(ctx, paymentsvc.CreateTopupOrderParams{
		Scene: scene, TenantID: tenantID, UserID: userID, AmountMicroUSD: in.Body.AmountMicroUSD, PackageID: in.Body.PackageID,
	})
	if err != nil {
		return nil, toProblem(err)
	}
	out := &topupOrderOutput{}
	out.Body.OrderID = order.OrderID
	out.Body.CodeURL = order.CodeURL
	out.Body.PaymentCurrency = order.PaymentCurrency
	out.Body.PaymentAmountMinor = order.PaymentAmountMinor
	out.Body.GrossAmountMicroUSD = order.GrossAmountMicroUSD
	out.Body.FeeAmountMicroUSD = order.FeeAmountMicroUSD
	out.Body.GiftAmountMicroUSD = order.GiftAmountMicroUSD
	out.Body.CreditedAmountMicroUSD = order.CreditedAmountMicroUSD
	out.Body.TopupMode = order.TopupMode
	out.Body.PackageName = order.PackageName
	out.Body.Status = order.Status
	out.Body.ExpiresAt = millisFromTime(order.ExpiresAt)
	out.Body.BalanceExpiresAt = millisFromTimePtr(order.BalanceExpiresAt)
	return out, nil
}

func (h *paymentHandlers) getOrder(ctx context.Context, in *getTopupOrderInput) (*topupOrderStatusOutput, error) {
	_, _, _, ok := sceneAndScopeFromClaims(ctx)
	if !ok {
		return nil, httpx.ErrForbidden
	}
	order, err := h.svc.GetOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	if !actorFromClaims(userClaimsFromCtx(ctx)).CanAccessUser(order.TenantID, order.UserID) {
		return nil, domain.ErrPaymentOrderNotFound
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
	out.Body.TopupMode = order.TopupMode
	out.Body.PackageName = order.PackageName
	out.Body.TransactionID = order.TransactionID
	out.Body.PaidAt = millisFromTimePtr(order.PaidAt)
	out.Body.BalanceExpiresAt = millisFromTimePtr(order.BalanceExpiresAt)
	return out, nil
}

func (h *paymentHandlers) listOrders(ctx context.Context, in *listTopupOrdersInput) (*listTopupOrdersOutput, error) {
	scene, tenantID, userID, ok := sceneAndScopeFromClaims(ctx)
	if !ok {
		return nil, httpx.ErrForbidden
	}
	list, total, err := h.svc.ListOrders(ctx, payment.ListOrdersParams{
		Scene: scene, Status: in.Status, TenantID: tenantID, UserID: userID, Page: in.Page, Size: in.Size,
	})
	if err != nil {
		return nil, toProblem(err)
	}
	items := make([]topupOrderItem, 0, len(list))
	for _, o := range list {
		items = append(items, orderToItem(o))
	}
	page, size := normalizePage(in.Page, in.Size)
	return &listTopupOrdersOutput{Body: httpx.NewPage(items, total, page, size)}, nil
}
