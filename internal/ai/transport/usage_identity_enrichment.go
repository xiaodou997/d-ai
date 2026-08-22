package transport

import (
	"context"
	"sync"
	"time"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/subscription"
)

type IdentityIncludedUserDTO struct {
	UserID   string  `json:"user_id"`
	TenantID string  `json:"tenant_id"`
	Username string  `json:"username"`
	Nickname *string `json:"nickname,omitempty"`
	Email    *string `json:"email,omitempty"`
	Avatar   *string `json:"avatar,omitempty"`
}

type IdentityIncludedTenantDTO struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
}

type IdentityIncludedDTO struct {
	Users   map[string]IdentityIncludedUserDTO   `json:"users"`
	Tenants map[string]IdentityIncludedTenantDTO `json:"tenants"`
}

const identityEnrichmentTimeout = 750 * time.Millisecond

func emptyIdentityIncluded() IdentityIncludedDTO {
	return IdentityIncludedDTO{
		Users:   map[string]IdentityIncludedUserDTO{},
		Tenants: map[string]IdentityIncludedTenantDTO{},
	}
}

func buildIdentityIncludedForLogs(ctx context.Context, provider IdentityProvider, observer IdentityEnrichmentFailureObserver, records []domain.UsageLog) IdentityIncludedDTO {
	userIDs := make([]string, 0, len(records))
	tenantIDs := make([]string, 0, len(records))
	seenUsers := make(map[string]struct{}, len(records))
	seenTenants := make(map[string]struct{}, len(records))

	for _, record := range records {
		if record.UserID != "" {
			if _, exists := seenUsers[record.UserID]; !exists {
				seenUsers[record.UserID] = struct{}{}
				userIDs = append(userIDs, record.UserID)
			}
		}
		if record.TenantID != "" {
			if _, exists := seenTenants[record.TenantID]; !exists {
				seenTenants[record.TenantID] = struct{}{}
				tenantIDs = append(tenantIDs, record.TenantID)
			}
		}
	}

	return buildIdentityIncluded(ctx, provider, observer, userIDs, tenantIDs)
}

func buildIdentityIncludedForRanking(ctx context.Context, provider IdentityProvider, observer IdentityEnrichmentFailureObserver, rows []domain.UsageUserRankingRow) IdentityIncludedDTO {
	userIDs := make([]string, 0, len(rows))
	tenantIDs := make([]string, 0, len(rows))
	seenUsers := make(map[string]struct{}, len(rows))
	seenTenants := make(map[string]struct{}, len(rows))

	for _, row := range rows {
		if row.UserID != "" {
			if _, exists := seenUsers[row.UserID]; !exists {
				seenUsers[row.UserID] = struct{}{}
				userIDs = append(userIDs, row.UserID)
			}
		}
		if row.TenantID != "" {
			if _, exists := seenTenants[row.TenantID]; !exists {
				seenTenants[row.TenantID] = struct{}{}
				tenantIDs = append(tenantIDs, row.TenantID)
			}
		}
	}

	return buildIdentityIncluded(ctx, provider, observer, userIDs, tenantIDs)
}

func buildIdentityIncludedForDashboardTenants(ctx context.Context, provider IdentityProvider, observer IdentityEnrichmentFailureObserver, rows []domain.DashboardTopTenant) IdentityIncludedDTO {
	tenantIDs := make([]string, 0, len(rows))
	seenTenants := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		tenantIDs = appendUniqueID(tenantIDs, seenTenants, row.TenantID)
	}
	return buildIdentityIncluded(ctx, provider, observer, nil, tenantIDs)
}

func buildIdentityIncludedForLimitPolicies(ctx context.Context, d CoreHTTPDeps, policies []commercial.LimitPolicy) IdentityIncludedDTO {
	tenantIDs := make([]string, 0, len(policies))
	seenTenants := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		if policy.ScopeType != commercial.LimitScopeTenant {
			continue
		}
		tenantIDs = appendUniqueID(tenantIDs, seenTenants, policy.ScopeID)
	}
	return buildIdentityIncluded(ctx, d.IdentityProvider, d.IdentityEnrichmentFailures, nil, tenantIDs)
}

func buildIdentityIncludedForSubPlans(ctx context.Context, d SubscriptionHTTPDeps, plans []subscription.Plan) IdentityIncludedDTO {
	tenantIDs := make([]string, 0, len(plans))
	seenTenants := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		tenantIDs = appendUniqueID(tenantIDs, seenTenants, plan.TenantID)
	}
	return buildIdentityIncluded(ctx, d.IdentityProvider, d.IdentityEnrichmentFailures, nil, tenantIDs)
}

func buildIdentityIncludedForSubscriptions(ctx context.Context, d SubscriptionHTTPDeps, subs []subscription.Subscription) IdentityIncludedDTO {
	userIDs := make([]string, 0, len(subs))
	tenantIDs := make([]string, 0, len(subs))
	seenUsers := make(map[string]struct{}, len(subs))
	seenTenants := make(map[string]struct{}, len(subs))
	for _, sub := range subs {
		userIDs = appendUniqueID(userIDs, seenUsers, sub.UserID)
		tenantIDs = appendUniqueID(tenantIDs, seenTenants, sub.TenantID)
	}
	return buildIdentityIncluded(ctx, d.IdentityProvider, d.IdentityEnrichmentFailures, userIDs, tenantIDs)
}

func buildIdentityIncludedForSubOrders(ctx context.Context, d SubscriptionHTTPDeps, orders []subscription.Order) IdentityIncludedDTO {
	userIDs := make([]string, 0, len(orders))
	tenantIDs := make([]string, 0, len(orders))
	seenUsers := make(map[string]struct{}, len(orders))
	seenTenants := make(map[string]struct{}, len(orders))
	for _, order := range orders {
		userIDs = appendUniqueID(userIDs, seenUsers, order.UserID)
		tenantIDs = appendUniqueID(tenantIDs, seenTenants, order.TenantID)
	}
	return buildIdentityIncluded(ctx, d.IdentityProvider, d.IdentityEnrichmentFailures, userIDs, tenantIDs)
}

func buildIdentityIncluded(ctx context.Context, provider IdentityProvider, observer IdentityEnrichmentFailureObserver, userIDs []string, tenantIDs []string) IdentityIncludedDTO {
	if provider == nil {
		return emptyIdentityIncluded()
	}

	included := emptyIdentityIncluded()
	enrichCtx, cancel := context.WithTimeout(ctx, identityEnrichmentTimeout)
	defer cancel()

	var wg sync.WaitGroup
	if len(userIDs) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			users, err := provider.BatchGetUsers(enrichCtx, userIDs)
			if err != nil {
				logIdentityEnrichmentFailure(observer, "users", err)
				return
			}
			for userID, user := range users {
				included.Users[userID] = IdentityIncludedUserDTO{
					UserID: user.UserID, TenantID: user.TenantID, Username: user.Username,
					Nickname: user.Nickname, Email: user.Email, Avatar: user.Avatar,
				}
			}
		}()
	}

	if len(tenantIDs) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tenants, err := provider.BatchGetTenants(enrichCtx, tenantIDs)
			if err != nil {
				logIdentityEnrichmentFailure(observer, "tenants", err)
				return
			}
			for tenantID, tenant := range tenants {
				included.Tenants[tenantID] = IdentityIncludedTenantDTO{
					TenantID: tenant.TenantID, TenantName: tenant.TenantName,
				}
			}
		}()
	}
	wg.Wait()

	return included
}

func logIdentityEnrichmentFailure(observer IdentityEnrichmentFailureObserver, kind string, err error) {
	if observer != nil {
		observer.ObserveFailure(kind, err)
	}
}

func appendUniqueID(ids []string, seen map[string]struct{}, id string) []string {
	if id == "" {
		return ids
	}
	if _, exists := seen[id]; exists {
		return ids
	}
	seen[id] = struct{}{}
	return append(ids, id)
}
