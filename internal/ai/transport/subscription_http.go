package transport

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/subscription"
)

// SubscriptionHTTPDeps is the complete dependency boundary for subscription
// HTTP routes. It is registered independently from the general AI route graph.
type SubscriptionHTTPDeps struct {
	Auth                       HTTPAuthDeps
	SubscriptionPlans          SubscriptionPlanCatalog
	SubscriptionPlanWriter     SubscriptionPlanManager
	SubscriptionPurchases      SubscriptionPurchaser
	Subscriptions              SubscriptionReader
	SubscriptionOrders         SubscriptionOrderReader
	SubscriptionGroupNames     SubscriptionGroupNameResolver
	IdentityProvider           IdentityProvider
	IdentityEnrichmentFailures IdentityEnrichmentFailureObserver
}

// SubscriptionPlanCatalog contains storefront and tenant plan queries.
type SubscriptionPlanCatalog interface {
	ListPlans(ctx context.Context, filter subscription.PlanFilter) ([]subscription.Plan, int64, error)
	ListPlansForUser(ctx context.Context, filter subscription.PlanFilter, userID string) ([]subscription.Plan, int64, error)
	GetPlan(ctx context.Context, id string) (*subscription.Plan, error)
	ListPurchasePolicyRevisions(ctx context.Context, planID string) ([]subscription.PurchasePolicyRevision, error)
}

// SubscriptionPlanManager owns tenant plan mutations.
type SubscriptionPlanManager interface {
	CreatePlan(ctx context.Context, params subscription.CreatePlanParams) (*subscription.Plan, error)
	UpdatePlan(ctx context.Context, params subscription.UpdatePlanParams) (bool, error)
	ReorderPlans(ctx context.Context, tenantID string, planIDs []string) error
	SetPlanStatus(ctx context.Context, id, tenantID, status string) (bool, error)
}

// SubscriptionPurchaser executes the idempotent order and balance transaction.
type SubscriptionPurchaser interface {
	Purchase(ctx context.Context, params subscription.PurchaseParams) (*subscription.Order, *subscription.Subscription, error)
}

// SubscriptionReader exposes subscription history and the lazily advanced
// current subscription.
type SubscriptionReader interface {
	ListSubscriptions(ctx context.Context, filter subscription.SubFilter) ([]subscription.Subscription, int64, error)
	CurrentSubscription(ctx context.Context, tenantID, userID string) (*subscription.Subscription, error)
}

// SubscriptionOrderReader contains non-mutating order queries.
type SubscriptionOrderReader interface {
	ListOrders(ctx context.Context, filter subscription.OrderFilter) ([]subscription.Order, int64, error)
	GetOrder(ctx context.Context, id string) (*subscription.Order, error)
}

// SubscriptionGroupNameResolver enriches immutable subscription snapshots for
// display. Failure remains fail-open in the transport converter.
type SubscriptionGroupNameResolver interface {
	GroupNames(ctx context.Context, ids []string) (map[string]string, error)
}

// RegisterSubscriptions owns the tenant and end-user subscription route
// groups, including their role-specific authentication middleware.
func RegisterSubscriptions(api huma.API, d SubscriptionHTTPDeps) {
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserSensitiveAuth(api, d.Auth))
	registerTenantSelfSubscriptions(tenant, d)

	userSelf := huma.NewGroup(api)
	userSelf.UseMiddleware(endUserSensitiveAuth(api, d.Auth))
	registerUserSelfSubscriptions(userSelf, d)
}
