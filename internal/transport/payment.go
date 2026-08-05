package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/httpx"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
	paymentsvc "xiaodou/dai/internal/payment/service"
)

// paymentHandlers 承载用户自助端点：终端用户(type=4)在线充值个人积分、租户管理员(type=3)
// 在线充值租户积分。scene 由 userType 决定，不接受调用方指定。
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
		ExchangeRate int64                  `json:"exchangeRate"`
		FeeRateBp    int                    `json:"feeRateBp"`
		Min          int64                  `json:"min"`
		Max          int64                  `json:"max"`
		Packages     []payment.TopupPackage `json:"packages"`
	}
}

type createTopupOrderInput struct {
	Body struct {
		Amount    int64  `json:"amount" required:"false" doc:"自定义充值金额，单位分"`
		PackageID string `json:"packageId" required:"false" doc:"快捷充值套餐 ID"`
	}
}

type topupOrderOutput struct {
	Body struct {
		OrderID      string `json:"orderId"`
		CodeURL      string `json:"codeUrl"`
		Amount       int64  `json:"amount"`
		CreditAmount int64  `json:"creditAmount"`
		GrossCredits int64  `json:"grossCredits"`
		FeeCredits   int64  `json:"feeCredits"`
		TopupMode    string `json:"topupMode"`
		PackageName  string `json:"packageName,omitempty"`
		Status       string `json:"status"`
		ExpiresAt    int64  `json:"expiresAt"`
	}
}

type getTopupOrderInput struct {
	OrderID string `path:"orderId"`
}

type topupOrderStatusOutput struct {
	Body struct {
		OrderID       string `json:"orderId"`
		Status        string `json:"status"`
		Amount        int64  `json:"amount"`
		CreditAmount  int64  `json:"creditAmount"`
		GrossCredits  int64  `json:"grossCredits"`
		FeeCredits    int64  `json:"feeCredits"`
		TopupMode     string `json:"topupMode"`
		PackageName   string `json:"packageName,omitempty"`
		TransactionID string `json:"transactionId,omitempty"`
		PaidAt        *int64 `json:"paidAt,omitempty"`
	}
}

type listTopupOrdersInput struct {
	Status string `query:"status" required:"false"`
	Page   int    `query:"page" default:"1"`
	Size   int    `query:"size" default:"20"`
}

type topupOrderItem struct {
	OrderID       string `json:"orderId"`
	Scene         string `json:"scene"`
	Status        string `json:"status"`
	Amount        int64  `json:"amount"`
	CreditAmount  int64  `json:"creditAmount"`
	GrossCredits  int64  `json:"grossCredits"`
	FeeCredits    int64  `json:"feeCredits"`
	TopupMode     string `json:"topupMode"`
	PackageName   string `json:"packageName,omitempty"`
	TransactionID string `json:"transactionId,omitempty"`
	CreatedAt     int64  `json:"createdAt"`
	PaidAt        *int64 `json:"paidAt,omitempty"`
}

type listTopupOrdersOutput struct {
	Body httpx.Page[topupOrderItem]
}

func orderToItem(o *payment.Order) topupOrderItem {
	return topupOrderItem{
		OrderID: o.OrderID, Scene: o.Scene, Status: o.Status, Amount: o.Amount, CreditAmount: o.CreditAmount,
		GrossCredits: o.GrossCreditAmount, FeeCredits: o.FeeCreditAmount, TopupMode: o.TopupMode, PackageName: o.PackageName,
		TransactionID: o.TransactionID, CreatedAt: millisFromTime(o.CreatedAt), PaidAt: millisFromTimePtr(o.PaidAt),
	}
}

// registerPayment 注册在线充值下单/查单端点（userAuth + requireUserType(3,4)）。
func registerPayment(api huma.API, d Deps) {
	h := newPaymentHandlers(d)
	authed := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist), requireUserType(api, 3, 4)}

	huma.Register(api, huma.Operation{OperationID: "payment-topup-config", Method: http.MethodGet, Path: "/api/v1/payments/topup-config",
		Summary: "在线充值配置（汇率/限额）", Tags: []string{"payment"}, Middlewares: authed}, h.topupConfig)
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
	out.Body.ExchangeRate = view.ExchangeRate
	out.Body.FeeRateBp = view.FeeRateBp
	out.Body.Min = view.Min
	out.Body.Max = view.Max
	out.Body.Packages = view.Packages
	return out, nil
}

func (h *paymentHandlers) createOrder(ctx context.Context, in *createTopupOrderInput) (*topupOrderOutput, error) {
	scene, tenantID, userID, ok := sceneAndScopeFromClaims(ctx)
	if !ok {
		return nil, httpx.ErrForbidden
	}
	order, err := h.svc.CreateTopupOrder(ctx, paymentsvc.CreateTopupOrderParams{
		Scene: scene, TenantID: tenantID, UserID: userID, AmountFen: in.Body.Amount, PackageID: in.Body.PackageID,
	})
	if err != nil {
		return nil, toProblem(err)
	}
	out := &topupOrderOutput{}
	out.Body.OrderID = order.OrderID
	out.Body.CodeURL = order.CodeURL
	out.Body.Amount = order.Amount
	out.Body.CreditAmount = order.CreditAmount
	out.Body.GrossCredits = order.GrossCreditAmount
	out.Body.FeeCredits = order.FeeCreditAmount
	out.Body.TopupMode = order.TopupMode
	out.Body.PackageName = order.PackageName
	out.Body.Status = order.Status
	out.Body.ExpiresAt = millisFromTime(order.ExpiresAt)
	return out, nil
}

func (h *paymentHandlers) getOrder(ctx context.Context, in *getTopupOrderInput) (*topupOrderStatusOutput, error) {
	_, tenantID, userID, ok := sceneAndScopeFromClaims(ctx)
	if !ok {
		return nil, httpx.ErrForbidden
	}
	order, err := h.svc.GetOrder(ctx, in.OrderID)
	if err != nil {
		return nil, toProblem(err)
	}
	if order.TenantID != tenantID || (userID != "" && order.UserID != userID) {
		return nil, domain.ErrPaymentOrderNotFound
	}
	out := &topupOrderStatusOutput{}
	out.Body.OrderID = order.OrderID
	out.Body.Status = order.Status
	out.Body.Amount = order.Amount
	out.Body.CreditAmount = order.CreditAmount
	out.Body.GrossCredits = order.GrossCreditAmount
	out.Body.FeeCredits = order.FeeCreditAmount
	out.Body.TopupMode = order.TopupMode
	out.Body.PackageName = order.PackageName
	out.Body.TransactionID = order.TransactionID
	out.Body.PaidAt = millisFromTimePtr(order.PaidAt)
	return out, nil
}

func (h *paymentHandlers) listOrders(ctx context.Context, in *listTopupOrdersInput) (*listTopupOrdersOutput, error) {
	scene, tenantID, userID, ok := sceneAndScopeFromClaims(ctx)
	if !ok {
		return nil, httpx.ErrForbidden
	}
	list, total, err := h.svc.ListOrders(ctx, paymentpg.ListOrdersParams{
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
