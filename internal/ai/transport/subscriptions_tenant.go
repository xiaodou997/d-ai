package transport

// subscriptions_tenant.go — 租户自助订阅套餐管理端点（tenantUserAuth，userType=3）。
// 租户自定义套餐并对自己的终端用户上架；tenant scope 全部来自 JWT claims。
// 端点清单见 docs/ai-subscription-design.md §7.2。
//
// 定价提示：订阅只覆盖用户侧收入（UserPayableMicro），平台侧成本（TenantPayableMicro）
// 在订阅期内仍照常按量结算，租户定价需自负盈亏（前端编辑页需展示该提示）。

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/libs/go/httpx"
)

// subPlanGroupInput 套餐绑定分组入参：两个字段均必填。
type subPlanGroupInput struct {
	GroupID              string  `json:"group_id" doc:"分组 ID"`
	QuotaDebitMultiplier float64 `json:"quota_debit_multiplier" minimum:"0.0001" doc:"套餐扣额倍率（额度消耗 = 基准价 × 套餐扣额倍率），必须 > 0"`
}

type subPurchasePolicyInput struct {
	LifetimeMaxPurchases *int32 `json:"lifetime_max_purchases,omitempty" minimum:"1" doc:"每位用户累计购买上限；空=不限"`
	PeriodType           string `json:"period_type" enum:"none,rolling,calendar" doc:"周期限制类型"`
	PeriodMaxPurchases   *int32 `json:"period_max_purchases,omitempty" minimum:"1" doc:"周期内购买上限"`
	RollingWindowHours   *int32 `json:"rolling_window_hours,omitempty" minimum:"1" maximum:"876000" doc:"滚动窗口小时数，最大 100 年"`
	CalendarUnit         string `json:"calendar_unit,omitempty" enum:"day,week,month" doc:"自然周期单位"`
	CalendarTimezone     string `json:"calendar_timezone,omitempty" doc:"自然周期 IANA 时区"`
	AllowAdvancePurchase bool   `json:"allow_advance_purchase" doc:"当前同套餐生效时是否允许提前购买"`
}

func purchasePolicyFromInput(in *subPurchasePolicyInput) subscription.PurchasePolicy {
	if in == nil {
		return subscription.DefaultPurchasePolicy()
	}
	return subscription.NormalizePurchasePolicy(subscription.PurchasePolicy{
		LifetimeMaxPurchases: in.LifetimeMaxPurchases,
		PeriodType:           in.PeriodType,
		PeriodMaxPurchases:   in.PeriodMaxPurchases,
		RollingWindowHours:   in.RollingWindowHours,
		CalendarUnit:         in.CalendarUnit,
		CalendarTimezone:     in.CalendarTimezone,
		AllowAdvancePurchase: in.AllowAdvancePurchase,
	})
}

func purchasePolicyUpdateFromInput(in *subPurchasePolicyInput) *subscription.PurchasePolicy {
	if in == nil {
		return nil
	}
	policy := purchasePolicyFromInput(in)
	return &policy
}

type subPlanWriteBody struct {
	Name               string                  `json:"name" doc:"套餐名称（同租户内唯一）"`
	Description        string                  `json:"description,omitempty" doc:"套餐说明"`
	PriceMicroUSD      int64                   `json:"price_micro_usd" minimum:"1" doc:"售价，单位 micro-USD"`
	DurationDays       int32                   `json:"duration_days" doc:"有效期天数，只能是 1/3/7/30"`
	TotalLimitMicro    int64                   `json:"total_limit_micro_usd" minimum:"1" doc:"套餐总额度，单位 micro-USD"`
	Window5hLimitMicro *int64                  `json:"window_5h_limit_micro_usd,omitempty" doc:"5 小时窗口额度，单位 micro-USD；空=不限"`
	Window7dLimitMicro *int64                  `json:"window_7d_limit_micro_usd,omitempty" doc:"7 天窗口额度，单位 micro-USD；空=不限，仅 ≥7 天套餐有意义"`
	SortOrder          int32                   `json:"sort_order,omitempty" doc:"排序权重"`
	SaleLimit          *int32                  `json:"sale_limit,omitempty" minimum:"1" doc:"套餐总销售份数；空=不限量"`
	Groups             []subPlanGroupInput     `json:"groups" minItems:"1" doc:"绑定分组（必填 ≥1）；额度按命中分组的基准价 × 套餐扣额倍率计量"`
	PurchasePolicy     *subPurchasePolicyInput `json:"purchase_policy,omitempty" doc:"每位用户的套餐购买限制；创建时省略=不限购且允许提前购买，更新时省略=保持现有政策"`
}

// planGroupsFromInput 把入参分组转成领域类型，sort 按入参序。
func planGroupsFromInput(in []subPlanGroupInput) []subscription.PlanGroup {
	out := make([]subscription.PlanGroup, 0, len(in))
	for i, g := range in {
		out = append(out, subscription.PlanGroup{GroupID: g.GroupID, QuotaDebitMultiplier: g.QuotaDebitMultiplier, SortOrder: int32(i)})
	}
	return out
}

type subTenantListPlansInput struct {
	Status string `query:"status" doc:"按状态筛选：draft/on_sale/off_sale"`
	Limit  int32  `query:"limit" default:"20" doc:"返回条数；默认 20，最大 200"`
	Offset int32  `query:"offset" default:"0" doc:"偏移量；默认 0"`
}

type subTenantCreatePlanInput struct {
	Body subPlanWriteBody
}

type subTenantPlanIDInput struct {
	PlanID string `path:"planID" doc:"套餐 ID"`
}

type subTenantUpdatePlanInput struct {
	PlanID string `path:"planID" doc:"套餐 ID"`
	Body   subPlanWriteBody
}

type subTenantPlanStatusInput struct {
	PlanID string `path:"planID" doc:"套餐 ID"`
	Body   struct {
		Status string `json:"status" doc:"目标状态：on_sale / off_sale"`
	}
}

type subTenantPlanReorderInput struct {
	Body struct {
		PlanIDs []string `json:"plan_ids" minItems:"1" maxItems:"2000" doc:"按目标展示顺序排列的套餐 ID"`
	}
}

type subTenantListSubsInput struct {
	UserID string `query:"user_id" doc:"按终端用户 ID 筛选"`
	Status string `query:"status" doc:"按订阅状态筛选：pending/active/expired/cancelled"`
	Limit  int32  `query:"limit" default:"20" doc:"返回条数；默认 20，最大 200"`
	Offset int32  `query:"offset" default:"0" doc:"偏移量；默认 0"`
}

type subTenantListOrdersInput struct {
	UserID string `query:"user_id" doc:"按终端用户 ID 筛选"`
	Status string `query:"status" doc:"按订单状态筛选：created/deducting/paid/failed"`
	Limit  int32  `query:"limit" default:"20" doc:"返回条数；默认 20，最大 200"`
	Offset int32  `query:"offset" default:"0" doc:"偏移量；默认 0"`
}

func registerTenantSelfSubscriptions(api huma.API, d SubscriptionHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-subscription-plans",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/subscription-plans",
		Summary:     "租户自助订阅套餐列表",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantListPlansInput) (*subPlanListOutput, error) {
		if d.SubscriptionPlans == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		plans, total, err := d.SubscriptionPlans.ListPlans(ctx, subscription.PlanFilter{
			TenantID: tenantID, Status: in.Status, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		items := make([]subPlanDTO, 0, len(plans))
		for _, p := range plans {
			items = append(items, subPlanToDTO(p))
		}
		page, size := subPageMeta(in.Limit, in.Offset)
		return &subPlanListOutput{Body: NewControlPage(items, total, page, size, emptyIdentityIncluded())}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "ai-create-tenant-self-subscription-plan",
		Method:        http.MethodPost,
		Path:          "/api/v1/tenants/me/subscription-plans",
		Summary:       "租户新建订阅套餐（草稿）",
		Description:   "新建套餐初始为 draft，需再上架。注意：订阅仅覆盖用户侧收入，平台侧成本仍按量结算，请自负盈亏定价。",
		Tags:          []string{"subscriptions"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *subTenantCreatePlanInput) (*subPlanOutput, error) {
		if d.SubscriptionPlanWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		userID := userIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		plan, err := d.SubscriptionPlanWriter.CreatePlan(ctx, subscription.CreatePlanParams{
			TenantID:           tenantID,
			Name:               in.Body.Name,
			Description:        in.Body.Description,
			PriceMicroUSD:      in.Body.PriceMicroUSD,
			DurationDays:       in.Body.DurationDays,
			TotalLimitMicro:    in.Body.TotalLimitMicro,
			Window5hLimitMicro: in.Body.Window5hLimitMicro,
			Window7dLimitMicro: in.Body.Window7dLimitMicro,
			SortOrder:          in.Body.SortOrder,
			SaleLimit:          in.Body.SaleLimit,
			CreatedBy:          userID,
			Groups:             planGroupsFromInput(in.Body.Groups),
			PurchasePolicy:     purchasePolicyFromInput(in.Body.PurchasePolicy),
		})
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		return &subPlanOutput{Body: subPlanToDTO(*plan)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-reorder-tenant-self-subscription-plans",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/me/subscription-plans/reorder",
		Summary:     "租户批量调整套餐展示顺序",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantPlanReorderInput) (*struct{}, error) {
		if d.SubscriptionPlanWriter == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		if err := d.SubscriptionPlanWriter.ReorderPlans(ctx, tenantID, in.Body.PlanIDs); err != nil {
			return nil, mapSubscriptionError(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-tenant-self-subscription-plan",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/subscription-plans/{planID}",
		Summary:     "租户订阅套餐详情",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantPlanIDInput) (*subPlanOutput, error) {
		if d.SubscriptionPlans == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		plan, err := d.SubscriptionPlans.GetPlan(ctx, in.PlanID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		if plan.TenantID != tenantID {
			return nil, httpx.ErrNotFound.WithDetail("套餐不存在")
		}
		return &subPlanOutput{Body: subPlanToDTO(*plan)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-subscription-plan-purchase-policy-revisions",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/subscription-plans/{planID}/purchase-policy-revisions",
		Summary:     "租户查看套餐购买政策修订历史",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantPlanIDInput) (*subPurchasePolicyRevisionListOutput, error) {
		if d.SubscriptionPlans == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		plan, err := d.SubscriptionPlans.GetPlan(ctx, in.PlanID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		if plan.TenantID != tenantID {
			return nil, httpx.ErrNotFound.WithDetail("套餐不存在")
		}
		revisions, err := d.SubscriptionPlans.ListPurchasePolicyRevisions(ctx, in.PlanID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		items := make([]subPurchasePolicyRevisionDTO, 0, len(revisions))
		for _, revision := range revisions {
			items = append(items, purchasePolicyRevisionToDTO(revision))
		}
		return &subPurchasePolicyRevisionListOutput{Body: subPurchasePolicyRevisionListBody{Items: items}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-tenant-self-subscription-plan",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/me/subscription-plans/{planID}",
		Summary:     "租户编辑订阅套餐",
		Description: "编辑只影响后续新购，已售订阅持有下单时的快照不受影响。",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantUpdatePlanInput) (*subPlanOutput, error) {
		if d.SubscriptionPlanWriter == nil || d.SubscriptionPlans == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		userID := userIDFromContext(ctx)
		ok, err := d.SubscriptionPlanWriter.UpdatePlan(ctx, subscription.UpdatePlanParams{
			ID:                 in.PlanID,
			TenantID:           tenantID,
			Name:               in.Body.Name,
			Description:        in.Body.Description,
			PriceMicroUSD:      in.Body.PriceMicroUSD,
			DurationDays:       in.Body.DurationDays,
			TotalLimitMicro:    in.Body.TotalLimitMicro,
			Window5hLimitMicro: in.Body.Window5hLimitMicro,
			Window7dLimitMicro: in.Body.Window7dLimitMicro,
			SortOrder:          in.Body.SortOrder,
			SaleLimit:          in.Body.SaleLimit,
			Groups:             planGroupsFromInput(in.Body.Groups),
			PurchasePolicy:     purchasePolicyUpdateFromInput(in.Body.PurchasePolicy),
			UpdatedBy:          userID,
		})
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		if !ok {
			return nil, httpx.ErrNotFound.WithDetail("套餐不存在或不属于当前租户")
		}
		plan, err := d.SubscriptionPlans.GetPlan(ctx, in.PlanID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		return &subPlanOutput{Body: subPlanToDTO(*plan)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-set-tenant-self-subscription-plan-status",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/me/subscription-plans/{planID}/status",
		Summary:     "租户上/下架订阅套餐",
		Description: "目标状态只能是 on_sale / off_sale。",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantPlanStatusInput) (*subPlanOutput, error) {
		if d.SubscriptionPlanWriter == nil || d.SubscriptionPlans == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		ok, err := d.SubscriptionPlanWriter.SetPlanStatus(ctx, in.PlanID, tenantID, in.Body.Status)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		if !ok {
			return nil, httpx.ErrNotFound.WithDetail("套餐不存在或不属于当前租户")
		}
		plan, err := d.SubscriptionPlans.GetPlan(ctx, in.PlanID)
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		return &subPlanOutput{Body: subPlanToDTO(*plan)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-subscriptions",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/subscriptions",
		Summary:     "租户下终端用户订阅列表",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantListSubsInput) (*subscriptionListOutput, error) {
		if d.Subscriptions == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		subs, total, err := d.Subscriptions.ListSubscriptions(ctx, subscription.SubFilter{
			TenantID: tenantID, UserID: in.UserID, Status: in.Status, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		items := subscriptionsToDTO(ctx, d.SubscriptionGroupNames, subs, time.Now())
		page, size := subPageMeta(in.Limit, in.Offset)
		included := buildIdentityIncludedForSubscriptions(ctx, d, subs)
		return &subscriptionListOutput{Body: NewControlPage(items, total, page, size, included)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-self-subscription-orders",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/subscription-orders",
		Summary:     "租户下终端用户订阅订单列表",
		Tags:        []string{"subscriptions"},
	}, func(ctx context.Context, in *subTenantListOrdersInput) (*subOrderListOutput, error) {
		if d.SubscriptionOrders == nil {
			return nil, httpx.ErrUnavailable.WithDetail("subscription service is not configured")
		}
		tenantID := tenantIDFromContext(ctx)
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		orders, total, err := d.SubscriptionOrders.ListOrders(ctx, subscription.OrderFilter{
			TenantID: tenantID, UserID: in.UserID, Status: in.Status, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return nil, mapSubscriptionError(err)
		}
		items := make([]subOrderDTO, 0, len(orders))
		for _, o := range orders {
			items = append(items, subOrderToDTO(o))
		}
		page, size := subPageMeta(in.Limit, in.Offset)
		included := buildIdentityIncludedForSubOrders(ctx, d, orders)
		return &subOrderListOutput{Body: NewControlPage(items, total, page, size, included)}, nil
	})
}
