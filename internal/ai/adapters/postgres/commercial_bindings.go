package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	commercial "xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/routing"
	"xiaodou/dai/internal/ai/serving"
)

func (r *CommercialRepo) UpsertUserBinding(ctx context.Context, in commercial.UserGroupBindingWrite) (commercial.UserGroupBinding, error) {
	item, err := r.group.UpsertUserGroup(ctx, commercial.UserGroupBindingWrite{
		TenantID:               in.TenantID,
		UserID:                 in.UserID,
		GroupID:                in.GroupID,
		UserMultiplierOverride: in.UserMultiplierOverride,
		CreatedBy:              in.CreatedBy,
		UpdatedBy:              in.UpdatedBy,
	})
	if err != nil {
		return commercial.UserGroupBinding{}, err
	}
	return legacyUserBindingToCommercial(item), nil
}

func (r *CommercialRepo) ListUserBindings(ctx context.Context, tenantID, userID string) ([]commercial.UserGroupBinding, error) {
	items, err := r.group.ListUserGroups(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.UserGroupBinding, 0, len(items))
	for _, item := range items {
		out = append(out, legacyUserBindingToCommercial(item))
	}
	return out, nil
}

func (r *CommercialRepo) DeleteUserBinding(ctx context.Context, tenantID, userID, groupID string) error {
	return r.group.DeleteUserGroup(ctx, tenantID, userID, groupID)
}

func (r *CommercialRepo) CreateLimitPolicy(ctx context.Context, in commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	if err := validateLegacyLimitBridge(in); err != nil {
		return commercial.LimitPolicy{}, err
	}
	item, err := r.limit.Create(ctx, in)
	if err != nil {
		return commercial.LimitPolicy{}, err
	}
	return item.ToCore(), nil
}

func (r *CommercialRepo) ListLimitPolicies(ctx context.Context, filter commercial.LimitPolicyFilter) ([]commercial.LimitPolicy, error) {
	items, err := r.limit.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]commercial.LimitPolicy, 0, len(items))
	for _, item := range items {
		mapped := item.ToCore()
		if filter.ScopeType != "" && mapped.ScopeType != filter.ScopeType {
			continue
		}
		if filter.ScopeID != "" && mapped.ScopeID != filter.ScopeID {
			continue
		}
		if filter.Status != "" && mapped.Status != filter.Status {
			continue
		}
		out = append(out, mapped)
	}
	return out, nil
}

func (r *CommercialRepo) UpdateLimitPolicy(ctx context.Context, id string, in commercial.LimitPolicyWrite) (commercial.LimitPolicy, error) {
	if err := validateLegacyLimitBridge(in); err != nil {
		return commercial.LimitPolicy{}, err
	}
	item, err := r.limit.Update(ctx, id, in)
	if err != nil {
		return commercial.LimitPolicy{}, err
	}
	return item.ToCore(), nil
}

func (r *CommercialRepo) UpdateLimitPolicyStatus(ctx context.Context, id string, status commercial.Status) (commercial.LimitPolicy, error) {
	item, err := r.limit.UpdateStatus(ctx, id, commercialStatusOrDefault(status))
	if err != nil {
		return commercial.LimitPolicy{}, err
	}
	return item.ToCore(), nil
}

func (r *CommercialRepo) DeleteLimitPolicies(ctx context.Context, filter commercial.LimitPolicyFilter) error {
	if filter.ScopeType == "" || filter.ScopeID == "" {
		return fmt.Errorf("scope_type and scope_id are required")
	}
	return r.limit.DeleteByScope(ctx, string(filter.ScopeType), filter.ScopeID)
}

func (r *CommercialRepo) UpsertRoutingPolicy(ctx context.Context, in commercial.RoutingPolicyWrite) (routing.Policy, error) {
	scopeKey, err := encodeRoutingScope(in.ScopeType, in.ScopeID)
	if err != nil {
		return routing.Policy{}, err
	}
	if err := r.weights.Upsert(ctx, scopeKey, serving.ScoreWeights{
		Cost:    in.Weights.Cost,
		Latency: in.Weights.Latency,
		Load:    in.Weights.Load,
		Health:  in.Weights.Health,
	}); err != nil {
		return routing.Policy{}, err
	}
	return r.getRoutingPolicyByScope(ctx, scopeKey)
}

func (r *CommercialRepo) ListRoutingPolicies(ctx context.Context) ([]routing.Policy, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, scope, weights, updated_at
		FROM ai_route_score_weights
		ORDER BY scope ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]routing.Policy, 0, 8)
	for rows.Next() {
		var (
			id        pgtype.UUID
			scopeKey  string
			weights   []byte
			updatedAt pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &scopeKey, &weights, &updatedAt); err != nil {
			return nil, err
		}
		item, err := routingPolicyFromRow(id, scopeKey, weights, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *CommercialRepo) getRoutingPolicyByScope(ctx context.Context, scopeKey string) (routing.Policy, error) {
	var (
		id        pgtype.UUID
		weights   []byte
		updatedAt pgtype.Timestamptz
	)
	if err := r.pool.QueryRow(ctx, `
		SELECT id, weights, updated_at
		FROM ai_route_score_weights
		WHERE scope = $1
	`, scopeKey).Scan(&id, &weights, &updatedAt); err != nil {
		return routing.Policy{}, err
	}
	return routingPolicyFromRow(id, scopeKey, weights, updatedAt)
}
