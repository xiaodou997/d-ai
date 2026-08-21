package transport

// subscriptions_self.go — 终端用户自助订阅端点（endUserAuth，userType=4）。
// 所有 handler 从 JWT claims 取 tenant+user，绝不信任 path/query，用户只能操作自己的资源。
// 端点清单见 docs/ai-subscription-design.md §7.1。

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/libs/go/httpx"
)

type subSelfListInput struct {
	Limit  int32 `query:"limit" default:"20" doc:"返回条数；默认 20，最大 200"`
	Offset int32 `query:"offset" default:"0" doc:"偏移量；默认 0"`
}

type subSelfOrderIDInput struct {
	OrderID string `path:"orderID" doc:"订单 ID"`
}

type subSelfCreateOrderInput struct {
	IdempotencyKey string `header:"Idempotency-Key" required:"true" doc:"客户端购买幂等键；同一购买重试必须复用"`
	Body           struct {
		PlanID string `json:"plan_id" doc:"套餐 ID"`
	}
}

// subPurchaseOutput 用 huma 动态 Status：201 已开通 / 202 处理中（前端轮询订单）。
type subPurchaseOutput struct {
	Status int
	Body   struct {
		Order        *subOrderDTO     `json:"order"`
		Subscription *subscriptionDTO `json:"subscription,omitempty"`
		Processing   bool             `json:"processing" doc:"true 表示扣款处理中，请轮询订单终态"`
	}
}

func registerUserSelfSubscriptions(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-subscription-plans",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/subscription-plans",
		Summary:     "终端用户自助在售订阅套餐列表",
		Description: "返回当前用户所属租户已上架（on_sale）的订阅套餐。价格和额度单位为 micro-USD。",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subSelfListInput) (*subPublicPlanListOutput, error) {
		if d.SubscriptionPlans == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		plans, total, err := d.SubscriptionPlans.ListPlansForUser(ctx, subscription.PlanFilter{
			TenantID: tenantID, OnSaleOnly: true, Limit: in.Limit, Offset: in.Offset,
		}, userID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		items := make([]subPublicPlanDTO, 0, len(plans))
		for _, p := range plans {
			items = append(items, subPlanToPublicDTO(p))
		}
		page, size := subPageMeta(in.Limit, in.Offset)
		out := &subPublicPlanListOutput{Body: NewControlPage(items, total, page, size, emptyIdentityIncluded())}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "ai-create-user-self-subscription-order",
		Method:        http.MethodPost,
		Path:          "/api/v1/users/me/subscription-orders",
		Summary:       "终端用户购买订阅套餐",
		Description:   "用本人 USD 余额购买套餐（不可退款）。201=已开通；202=扣款处理中请轮询订单；402=余额不足；409=排队已满或该套餐已在待激活队列。",
		Tags:          []string{"subscriptions"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *subSelfCreateOrderInput) (*subPurchaseOutput, error) {
		if d.SubscriptionPurchases == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		if in.Body.PlanID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("plan_id is required")
		}
		if in.IdempotencyKey == "" {
			return nil, httpx.ErrBadRequest.WithDetail("Idempotency-Key header is required")
		}
		order, sub, err := d.SubscriptionPurchases.Purchase(ctx, subscription.PurchaseParams{
			TenantID: tenantID, UserID: userID, PlanID: in.Body.PlanID, IdempotencyKey: in.IdempotencyKey,
		})
		out := &subPurchaseOutput{}
		if err != nil {
			// 扣款未知态：订单停 deducting，janitor 补偿；返回 202 让前端轮询。
			if err == subscription.ErrOrderProcessing && order != nil {
				dto := subOrderToDTO(*order)
				out.Status = http.StatusAccepted
				out.Body.Order = &dto
				out.Body.Processing = true
				return out, nil
			}
			return nil, mapSubscriptionError(err)
		}
		odto := subOrderToDTO(*order)
		out.Status = http.StatusCreated
		out.Body.Order = &odto
		if sub != nil {
			sdto := subscriptionsToDTO(ctx, d.SubscriptionGroupNames, []subscription.Subscription{*sub}, time.Now())[0]
			out.Body.Subscription = &sdto
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-subscription-orders",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/subscription-orders",
		Summary:     "终端用户自助订阅订单列表",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subSelfListInput) (*subOrderListOutput, error) {
		if d.SubscriptionOrders == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		orders, total, err := d.SubscriptionOrders.ListOrders(ctx, subscription.OrderFilter{
			TenantID: tenantID, UserID: userID, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		items := make([]subOrderDTO, 0, len(orders))
		for _, o := range orders {
			items = append(items, subOrderToDTO(o))
		}
		page, size := subPageMeta(in.Limit, in.Offset)
		out := &subOrderListOutput{Body: NewControlPage(items, total, page, size, emptyIdentityIncluded())}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-user-self-subscription-order",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/subscription-orders/{orderID}",
		Summary:     "终端用户自助订阅订单详情（轮询用）",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subSelfOrderIDInput) (*subOrderOutput, error) {
		if d.SubscriptionOrders == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		order, err := d.SubscriptionOrders.GetOrder(ctx, in.OrderID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		// 归属校验：只能看自己的订单。
		if order.TenantID != tenantID || order.UserID != userID {
			return nil, httpx.ErrNotFound.WithDetail("订单不存在")
		}
		out := &subOrderOutput{Body: subOrderToDTO(*order)}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-user-self-subscriptions",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/subscriptions",
		Summary:     "终端用户自助订阅列表（含排队/历史）",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subSelfListInput) (*subscriptionListOutput, error) {
		if d.Subscriptions == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		subs, total, err := d.Subscriptions.ListSubscriptions(ctx, subscription.SubFilter{
			TenantID: tenantID, UserID: userID, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		items := subscriptionsToDTO(ctx, d.SubscriptionGroupNames, subs, time.Now())
		page, size := subPageMeta(in.Limit, in.Offset)
		out := &subscriptionListOutput{Body: NewControlPage(items, total, page, size, emptyIdentityIncluded())}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-user-self-current-subscription",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/me/subscriptions/current",
		Summary:     "终端用户当前生效订阅 + 三窗口余量",
		Description: "懒惰触发激活/过期流转后返回当前生效订阅；无生效订阅时 subscription 为 null。窗口重置时刻由后端按服务器时间算好下发。",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, _ *struct{}) (*subscriptionNullableOutput, error) {
		if d.Subscriptions == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" || userID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
		}
		sub, err := d.Subscriptions.CurrentSubscription(ctx, tenantID, userID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		out := &subscriptionNullableOutput{}
		if sub != nil {
			dto := subscriptionsToDTO(ctx, d.SubscriptionGroupNames, []subscription.Subscription{*sub}, time.Now())[0]
			out.Body = &dto
		}
		return out, nil
	})
}
