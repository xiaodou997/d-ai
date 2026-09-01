package commercial

import (
	"context"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

func newValidationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

// Service is the vNext business layer for the commercial control plane.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateGroup(ctx context.Context, tenantID string, in GroupWrite) (Group, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return Group{}, newValidationError("tenant_id", "tenant_id is required")
	}
	normalized, err := normalizeGroupWrite(in)
	if err != nil {
		return Group{}, err
	}
	return s.repo.CreateGroup(ctx, tenantID, normalized)
}

func (s *Service) ListGroups(ctx context.Context, tenantID string) ([]Group, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, newValidationError("tenant_id", "tenant_id is required")
	}
	return s.repo.ListGroups(ctx, tenantID)
}

func (s *Service) GetGroup(ctx context.Context, scope TenantGroupScope) (Group, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return Group{}, err
	}
	return s.repo.GetGroup(ctx, normalized)
}

func (s *Service) UpdateGroup(ctx context.Context, scope TenantGroupScope, in GroupWrite) (Group, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return Group{}, err
	}
	normalized, err := normalizeGroupWrite(in)
	if err != nil {
		return Group{}, err
	}
	return s.repo.UpdateGroup(ctx, normalizedScope, normalized)
}

func (s *Service) UpdateGroupRoutePolicy(ctx context.Context, scope TenantGroupScope, in GroupRoutePolicyWrite) (Group, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return Group{}, err
	}
	normalized, err := normalizeGroupRoutePolicyWrite(in)
	if err != nil {
		return Group{}, err
	}
	if in.ExpectedVersion <= 0 {
		return Group{}, newValidationError("expected_version", "expected_version must be greater than 0")
	}
	return s.repo.UpdateGroupRoutePolicy(ctx, normalizedScope, normalized)
}

func (s *Service) UpdateGroupStatus(ctx context.Context, scope TenantGroupScope, status Status) (Group, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return Group{}, err
	}
	normalized, err := normalizeStatus(status)
	if err != nil {
		return Group{}, err
	}
	return s.repo.UpdateGroupStatus(ctx, normalizedScope, normalized)
}

func (s *Service) DeleteGroup(ctx context.Context, scope TenantGroupScope) error {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return err
	}
	return s.repo.DeleteGroup(ctx, normalized)
}

func (s *Service) ListGroupClientSurfaces(ctx context.Context, scope TenantGroupScope) ([]GroupClientSurface, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return nil, err
	}
	return s.repo.ListGroupClientSurfaces(ctx, normalized)
}

func (s *Service) GetGroupClientSurfacePolicy(ctx context.Context, scope TenantGroupScope) (GroupClientSurfacePolicy, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return GroupClientSurfacePolicy{}, err
	}
	if _, err := s.repo.GetGroup(ctx, normalized); err != nil {
		return GroupClientSurfacePolicy{}, err
	}
	entries, err := s.repo.ListGroupClientSurfaces(ctx, normalized)
	if err != nil {
		return GroupClientSurfacePolicy{}, err
	}
	policy := GroupClientSurfacePolicy{
		GroupID: normalized.GroupID,
		Mode:    GroupClientSurfacePolicyAll,
	}
	if len(entries) == 0 {
		return policy, nil
	}
	policy.Mode = GroupClientSurfacePolicyRestricted
	policy.AllowedSurfaces = make([]surface.ID, 0, len(entries))
	for _, entry := range entries {
		if entry.Status == StatusActive {
			policy.AllowedSurfaces = append(policy.AllowedSurfaces, entry.Surface)
		}
	}
	if len(policy.AllowedSurfaces) == len(surface.Known()) {
		policy.Mode = GroupClientSurfacePolicyAll
		policy.AllowedSurfaces = nil
	}
	return policy, nil
}

func (s *Service) ReplaceGroupClientSurfacePolicy(ctx context.Context, scope TenantGroupScope, in GroupClientSurfacePolicyWrite) (GroupClientSurfacePolicy, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return GroupClientSurfacePolicy{}, err
	}
	selected := make(map[surface.ID]struct{}, len(in.AllowedSurfaces))
	switch in.Mode {
	case GroupClientSurfacePolicyAll:
		for _, clientSurface := range surface.Known() {
			selected[clientSurface] = struct{}{}
		}
	case GroupClientSurfacePolicyRestricted:
		if len(in.AllowedSurfaces) == 0 {
			return GroupClientSurfacePolicy{}, newValidationError("allowed_surfaces", "at least one surface is required in restricted mode")
		}
		for _, raw := range in.AllowedSurfaces {
			clientSurface := surface.ID(strings.TrimSpace(string(raw)))
			if !surface.IsKnown(clientSurface) {
				return GroupClientSurfacePolicy{}, newValidationError("allowed_surfaces", "contains an unsupported surface")
			}
			selected[clientSurface] = struct{}{}
		}
	default:
		return GroupClientSurfacePolicy{}, newValidationError("mode", "mode must be all or restricted")
	}
	current, err := s.repo.ListGroupClientSurfaces(ctx, normalizedScope)
	if err != nil {
		return GroupClientSurfacePolicy{}, err
	}
	bridgeBySurface := make(map[surface.ID]bool, len(current))
	for _, entry := range current {
		bridgeBySurface[entry.Surface] = entry.BridgeEnabled
	}
	entries := make([]GroupClientSurfaceWrite, 0, len(surface.Known()))
	for _, clientSurface := range surface.Known() {
		bridgeEnabled, exists := bridgeBySurface[clientSurface]
		if !exists {
			bridgeEnabled = true
		}
		status := StatusDisabled
		if _, allowed := selected[clientSurface]; allowed {
			status = StatusActive
		}
		entries = append(entries, GroupClientSurfaceWrite{
			Surface:       string(clientSurface),
			BridgeEnabled: bridgeEnabled,
			Status:        status,
		})
	}
	if err := s.repo.ReplaceGroupClientSurfaces(ctx, normalizedScope, entries); err != nil {
		return GroupClientSurfacePolicy{}, err
	}
	return s.GetGroupClientSurfacePolicy(ctx, normalizedScope)
}

func (s *Service) AddGroupTarget(ctx context.Context, scope TenantGroupScope, in GroupTargetWrite) (GroupTarget, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return GroupTarget{}, err
	}
	normalized, err := normalizeGroupTargetWrite(in)
	if err != nil {
		return GroupTarget{}, err
	}
	return s.repo.AddGroupTarget(ctx, normalizedScope, normalized)
}

func (s *Service) ListGroupTargets(ctx context.Context, scope TenantGroupScope) ([]GroupTarget, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return nil, err
	}
	return s.repo.ListGroupTargets(ctx, normalized)
}

func (s *Service) ListGroupTargetDetails(ctx context.Context, scope TenantGroupScope) ([]GroupTargetDetail, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return nil, err
	}
	return s.repo.ListGroupTargetDetails(ctx, normalized)
}

func (s *Service) ListGroupTargetsByTarget(ctx context.Context, targetKind TargetKind, targetID string) ([]GroupTargetDetail, error) {
	if strings.TrimSpace(targetID) == "" {
		return nil, newValidationError("target_id", "target_id is required")
	}
	switch targetKind {
	case TargetKindDirectUpstream, TargetKindOAuthPool:
	default:
		return nil, newValidationError("target_kind", "unsupported target_kind")
	}
	return s.repo.ListGroupTargetsByTarget(ctx, targetKind, targetID)
}

func (s *Service) GetGroupTargetDetail(ctx context.Context, scope TenantGroupScope, id string) (GroupTargetDetail, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return GroupTargetDetail{}, err
	}
	if strings.TrimSpace(id) == "" {
		return GroupTargetDetail{}, newValidationError("id", "id is required")
	}
	return s.repo.GetGroupTargetDetail(ctx, normalized, id)
}

func (s *Service) UpdateGroupTarget(ctx context.Context, scope TenantGroupScope, id string, in GroupTargetWrite) (GroupTarget, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return GroupTarget{}, err
	}
	if strings.TrimSpace(id) == "" {
		return GroupTarget{}, newValidationError("id", "id is required")
	}
	normalized, err := normalizeGroupTargetWrite(in)
	if err != nil {
		return GroupTarget{}, err
	}
	return s.repo.UpdateGroupTarget(ctx, normalizedScope, id, normalized)
}

func (s *Service) DeleteGroupTarget(ctx context.Context, scope TenantGroupScope, id string) error {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return newValidationError("id", "id is required")
	}
	return s.repo.DeleteGroupTarget(ctx, normalized, id)
}

func (s *Service) ReplaceGroupTargets(ctx context.Context, scope TenantGroupScope, in GroupTargetBatchWrite) (GroupTargetBatchResult, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return GroupTargetBatchResult{}, err
	}
	if in.ExpectedVersion <= 0 {
		return GroupTargetBatchResult{}, newValidationError("expected_version", "expected_version must be greater than 0")
	}
	normalized := make([]GroupTargetWrite, 0, len(in.Targets))
	seen := make(map[string]struct{}, len(in.Targets))
	for index, target := range in.Targets {
		item, normalizeErr := normalizeGroupTargetWrite(target)
		if normalizeErr != nil {
			return GroupTargetBatchResult{}, newValidationError("targets", "target "+strconv.Itoa(index+1)+": "+normalizeErr.Error())
		}
		key := string(item.TargetKind) + "\x00" + item.TargetID
		if _, exists := seen[key]; exists {
			return GroupTargetBatchResult{}, newValidationError("targets", "duplicate target: "+item.TargetID)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}
	return s.repo.ReplaceGroupTargets(ctx, normalizedScope, GroupTargetBatchWrite{
		ExpectedVersion: in.ExpectedVersion,
		Targets:         normalized,
	})
}

func (s *Service) AddDispatchRule(ctx context.Context, scope TenantGroupScope, in DispatchRuleWrite) (DispatchRule, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return DispatchRule{}, err
	}
	normalized, err := normalizeDispatchRuleWrite(in)
	if err != nil {
		return DispatchRule{}, err
	}
	return s.repo.AddDispatchRule(ctx, normalizedScope, normalized)
}

func (s *Service) ListDispatchRules(ctx context.Context, scope TenantGroupScope) ([]DispatchRule, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return nil, err
	}
	return s.repo.ListDispatchRules(ctx, normalized)
}

func (s *Service) UpdateDispatchRule(ctx context.Context, scope TenantGroupScope, id string, in DispatchRuleWrite) (DispatchRule, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return DispatchRule{}, err
	}
	if strings.TrimSpace(id) == "" {
		return DispatchRule{}, newValidationError("id", "id is required")
	}
	normalized, err := normalizeDispatchRuleWrite(in)
	if err != nil {
		return DispatchRule{}, err
	}
	return s.repo.UpdateDispatchRule(ctx, normalizedScope, id, normalized)
}

func (s *Service) UpdateDispatchRuleStatus(ctx context.Context, scope TenantGroupScope, id string, status Status) (DispatchRule, error) {
	normalizedScope, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return DispatchRule{}, err
	}
	if strings.TrimSpace(id) == "" {
		return DispatchRule{}, newValidationError("id", "id is required")
	}
	normalizedStatus, err := normalizeStatus(status)
	if err != nil {
		return DispatchRule{}, err
	}
	return s.repo.UpdateDispatchRuleStatus(ctx, normalizedScope, id, normalizedStatus)
}

func (s *Service) DeleteDispatchRule(ctx context.Context, scope TenantGroupScope, id string) error {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return newValidationError("id", "id is required")
	}
	return s.repo.DeleteDispatchRule(ctx, normalized, id)
}

func (s *Service) PreviewDispatch(ctx context.Context, scope TenantGroupScope, requestedModel string, clientSurface surface.ID) (DispatchPreview, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return DispatchPreview{}, err
	}
	if strings.TrimSpace(requestedModel) == "" {
		return DispatchPreview{}, newValidationError("requested_model", "requested_model is required")
	}
	clientSurface = surface.ID(strings.TrimSpace(string(clientSurface)))
	if !surface.IsKnown(clientSurface) {
		return DispatchPreview{}, newValidationError("client_surface", "unsupported client_surface")
	}
	return s.repo.PreviewDispatch(ctx, normalized, strings.TrimSpace(requestedModel), clientSurface)
}

func (s *Service) ListDispatchModels(ctx context.Context, scope TenantGroupScope, clientSurface surface.ID) ([]DispatchModel, error) {
	normalized, err := normalizeTenantGroupScope(scope)
	if err != nil {
		return nil, err
	}
	clientSurface = surface.ID(strings.TrimSpace(string(clientSurface)))
	if !surface.IsKnown(clientSurface) {
		return nil, newValidationError("client_surface", "unsupported client_surface")
	}
	return s.repo.ListDispatchModels(ctx, normalized, clientSurface)
}

func (s *Service) ListVisibleGroupsForTenant(ctx context.Context, tenantID string) ([]AccessibleGroup, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, newValidationError("tenant_id", "tenant_id is required")
	}
	return s.ResolveAccessibleGroups(ctx, identity.Subject{TenantID: tenantID})
}

func (s *Service) ListVisibleGroupsForUser(ctx context.Context, tenantID, userID string) ([]AccessibleGroup, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, newValidationError("tenant_id", "tenant_id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, newValidationError("user_id", "user_id is required")
	}
	return s.ResolveAccessibleGroups(ctx, identity.Subject{TenantID: tenantID, UserID: userID})
}

func (s *Service) UpsertUserBinding(ctx context.Context, in UserGroupBindingWrite) (UserGroupBinding, error) {
	normalized, err := normalizeUserBindingWrite(in)
	if err != nil {
		return UserGroupBinding{}, err
	}
	return s.repo.UpsertUserBinding(ctx, normalized)
}

func (s *Service) ListUserBindings(ctx context.Context, tenantID, userID string) ([]UserGroupBinding, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, newValidationError("tenant_id", "tenant_id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, newValidationError("user_id", "user_id is required")
	}
	return s.repo.ListUserBindings(ctx, tenantID, userID)
}

func (s *Service) DeleteUserBinding(ctx context.Context, tenantID, userID, groupID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return newValidationError("tenant_id", "tenant_id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return newValidationError("user_id", "user_id is required")
	}
	if strings.TrimSpace(groupID) == "" {
		return newValidationError("group_id", "group_id is required")
	}
	return s.repo.DeleteUserBinding(ctx, tenantID, userID, groupID)
}

func (s *Service) CreateLimitPolicy(ctx context.Context, in LimitPolicyWrite) (LimitPolicy, error) {
	normalized, err := normalizeLimitPolicyWrite(in)
	if err != nil {
		return LimitPolicy{}, err
	}
	return s.repo.CreateLimitPolicy(ctx, normalized)
}

func (s *Service) ListLimitPolicies(ctx context.Context, filter LimitPolicyFilter) ([]LimitPolicy, error) {
	normalized, err := normalizeLimitPolicyFilter(filter)
	if err != nil {
		return nil, err
	}
	return s.repo.ListLimitPolicies(ctx, normalized)
}

func (s *Service) UpdateLimitPolicy(ctx context.Context, id string, in LimitPolicyWrite) (LimitPolicy, error) {
	if strings.TrimSpace(id) == "" {
		return LimitPolicy{}, newValidationError("id", "id is required")
	}
	normalized, err := normalizeLimitPolicyWrite(in)
	if err != nil {
		return LimitPolicy{}, err
	}
	return s.repo.UpdateLimitPolicy(ctx, id, normalized)
}

func (s *Service) UpdateLimitPolicyStatus(ctx context.Context, id string, status Status) (LimitPolicy, error) {
	if strings.TrimSpace(id) == "" {
		return LimitPolicy{}, newValidationError("id", "id is required")
	}
	normalized, err := normalizeStatus(status)
	if err != nil {
		return LimitPolicy{}, err
	}
	return s.repo.UpdateLimitPolicyStatus(ctx, id, normalized)
}

func (s *Service) DeleteLimitPolicies(ctx context.Context, filter LimitPolicyFilter) error {
	normalized, err := normalizeLimitPolicyFilter(filter)
	if err != nil {
		return err
	}
	if normalized.ScopeType == "" || normalized.ScopeID == "" {
		return newValidationError("scope", "scope_type and scope_id are required")
	}
	return s.repo.DeleteLimitPolicies(ctx, normalized)
}

func normalizeGroupWrite(in GroupWrite) (GroupWrite, error) {
	if in.ExpectedRoutePolicyVersion < 0 {
		return GroupWrite{}, newValidationError("route_policy_version", "route_policy_version must be >= 0")
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	policy, err := normalizeGroupRoutePolicyWrite(GroupRoutePolicyWrite{
		RouteStrategy:  in.RouteStrategy,
		RouteObjective: in.RouteObjective,
	})
	if err != nil {
		return GroupWrite{}, err
	}
	in.RouteStrategy = policy.RouteStrategy
	in.RouteObjective = policy.RouteObjective
	in.RetailPriceBookID = strings.TrimSpace(in.RetailPriceBookID)
	if in.Name == "" && in.Code == "" {
		return GroupWrite{}, newValidationError("name", "name or code is required")
	}
	if in.Name == "" {
		in.Name = in.Code
	}
	if in.Code == "" {
		in.Code = in.Name
	}
	if in.RetailPriceBookID == "" {
		return GroupWrite{}, newValidationError("retail_price_book_id", "retail_price_book_id is required")
	}
	status, err := normalizeStatus(in.Status)
	if err != nil {
		return GroupWrite{}, err
	}
	if !isFiniteNonNegative(in.DefaultUserMultiplier) {
		return GroupWrite{}, newValidationError("default_user_multiplier", "default_user_multiplier must be a finite number >= 0")
	}
	if in.SortOrder < 0 {
		return GroupWrite{}, newValidationError("sort_order", "sort_order must be >= 0")
	}
	in.Status = status
	return in, nil
}

func normalizeGroupRoutePolicyWrite(in GroupRoutePolicyWrite) (GroupRoutePolicyWrite, error) {
	if in.RouteStrategy == "" {
		in.RouteStrategy = RouteStrategyAdaptive
	}
	if in.RouteObjective == "" {
		in.RouteObjective = RouteObjectiveBalanced
	}
	switch in.RouteStrategy {
	case RouteStrategyFailover, RouteStrategyWeighted, RouteStrategyAdaptive:
	default:
		return GroupRoutePolicyWrite{}, newValidationError("route_strategy", "unsupported route_strategy")
	}
	switch in.RouteObjective {
	case RouteObjectiveBalanced, RouteObjectiveCost, RouteObjectiveLatency, RouteObjectiveStability:
	default:
		return GroupRoutePolicyWrite{}, newValidationError("route_objective", "unsupported route_objective")
	}
	// Objectives are meaningful only for adaptive scoring. Canonicalising them
	// for structural strategies prevents stale UI values from looking active in
	// the API while keeping the group policy a single coherent document.
	if in.RouteStrategy != RouteStrategyAdaptive {
		in.RouteObjective = RouteObjectiveBalanced
	}
	return in, nil
}

func normalizeGroupTargetWrite(in GroupTargetWrite) (GroupTargetWrite, error) {
	in.TargetID = strings.TrimSpace(in.TargetID)
	if in.TargetID == "" {
		return GroupTargetWrite{}, newValidationError("target_id", "target_id is required")
	}
	switch in.TargetKind {
	case TargetKindDirectUpstream, TargetKindOAuthPool:
	default:
		return GroupTargetWrite{}, newValidationError("target_kind", "unsupported target_kind")
	}
	if in.Priority < 0 {
		return GroupTargetWrite{}, newValidationError("priority", "priority must be >= 0")
	}
	if !isFiniteNonNegative(in.RoutingWeight) {
		return GroupTargetWrite{}, newValidationError("routing_weight", "routing_weight must be a finite number >= 0")
	}
	status, err := normalizeStatus(in.Status)
	if err != nil {
		return GroupTargetWrite{}, err
	}
	in.Status = status
	return in, nil
}

// PostgreSQL NUMERIC(10,4), used by the group policy columns, can represent at
// most 999999.9999. Rejecting values outside that range at the application
// boundary gives callers a domain error instead of a driver-specific cast
// failure, and explicitly rejects NaN/Inf values that can otherwise bypass a
// simple >= 0 check in some callers.
const maxGroupPolicyNumber = 999999.9999

func isFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= maxGroupPolicyNumber
}

func normalizeDispatchRuleWrite(in DispatchRuleWrite) (DispatchRuleWrite, error) {
	in.ClientSurface = surface.ID(strings.TrimSpace(string(in.ClientSurface)))
	in.MatchValue = strings.TrimSpace(in.MatchValue)
	in.TargetModelID = strings.TrimSpace(in.TargetModelID)
	in.Notes = strings.TrimSpace(in.Notes)
	if !surface.IsKnown(in.ClientSurface) {
		return DispatchRuleWrite{}, newValidationError("client_surface", "unsupported client_surface")
	}
	switch in.MatchType {
	case DispatchMatchExact, DispatchMatchPrefix, DispatchMatchWildcard, DispatchMatchRegex:
	default:
		return DispatchRuleWrite{}, newValidationError("match_type", "unsupported match_type")
	}
	if in.MatchValue == "" {
		return DispatchRuleWrite{}, newValidationError("match_value", "match_value is required")
	}
	if in.TargetModelID == "" {
		return DispatchRuleWrite{}, newValidationError("target_model_id", "target_model_id is required")
	}
	if in.MatchType == DispatchMatchWildcard {
		if _, err := path.Match(in.MatchValue, ""); err != nil {
			return DispatchRuleWrite{}, newValidationError("match_value", "invalid wildcard pattern")
		}
	}
	if in.MatchType == DispatchMatchRegex {
		if _, err := regexp.Compile(in.MatchValue); err != nil {
			return DispatchRuleWrite{}, newValidationError("match_value", "invalid regular expression")
		}
	}
	if in.Priority < 0 {
		return DispatchRuleWrite{}, newValidationError("priority", "priority must be >= 0")
	}
	return in, nil
}

func normalizeTenantGroupScope(scope TenantGroupScope) (TenantGroupScope, error) {
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.GroupID = strings.TrimSpace(scope.GroupID)
	if scope.TenantID == "" {
		return TenantGroupScope{}, newValidationError("tenant_id", "tenant_id is required")
	}
	if scope.GroupID == "" {
		return TenantGroupScope{}, newValidationError("group_id", "group_id is required")
	}
	return scope, nil
}

func normalizeUserBindingWrite(in UserGroupBindingWrite) (UserGroupBindingWrite, error) {
	in.TenantID = strings.TrimSpace(in.TenantID)
	in.UserID = strings.TrimSpace(in.UserID)
	in.GroupID = strings.TrimSpace(in.GroupID)
	in.CreatedBy = strings.TrimSpace(in.CreatedBy)
	in.UpdatedBy = strings.TrimSpace(in.UpdatedBy)
	if in.TenantID == "" {
		return UserGroupBindingWrite{}, newValidationError("tenant_id", "tenant_id is required")
	}
	if in.UserID == "" {
		return UserGroupBindingWrite{}, newValidationError("user_id", "user_id is required")
	}
	if in.GroupID == "" {
		return UserGroupBindingWrite{}, newValidationError("group_id", "group_id is required")
	}
	if in.UserMultiplierOverride != nil && *in.UserMultiplierOverride < 0 {
		return UserGroupBindingWrite{}, newValidationError("multiplier_override", "multiplier_override must be >= 0")
	}
	return in, nil
}

func normalizeLimitPolicyWrite(in LimitPolicyWrite) (LimitPolicyWrite, error) {
	in.ScopeID = strings.TrimSpace(in.ScopeID)
	in.ModelID = strings.TrimSpace(in.ModelID)
	in.CreatedBy = strings.TrimSpace(in.CreatedBy)
	switch in.ScopeType {
	case LimitScopePlatform, LimitScopeTenant, LimitScopeUser, LimitScopeAPIKey:
	default:
		return LimitPolicyWrite{}, newValidationError("scope_type", "unsupported scope_type")
	}
	if in.ScopeID == "" {
		return LimitPolicyWrite{}, newValidationError("scope_id", "scope_id is required")
	}
	if in.Capability != "" {
		switch catalog.Capability(in.Capability) {
		case catalog.CapabilityChat,
			catalog.CapabilityEmbedding,
			catalog.CapabilityImageGeneration,
			catalog.CapabilityImageEdit,
			catalog.CapabilityAudioTTS,
			catalog.CapabilityAudioSTT,
			catalog.CapabilityVideoGeneration,
			catalog.CapabilityWorkflow:
		default:
			return LimitPolicyWrite{}, newValidationError("capability", "unsupported capability")
		}
	}
	if err := validateOptionalNonNegative("concurrency_limit", in.ConcurrencyLimit); err != nil {
		return LimitPolicyWrite{}, err
	}
	status, err := normalizeStatus(in.Status)
	if err != nil {
		return LimitPolicyWrite{}, err
	}
	in.Status = status
	return in, nil
}

func normalizeLimitPolicyFilter(filter LimitPolicyFilter) (LimitPolicyFilter, error) {
	if filter.ScopeType != "" {
		switch filter.ScopeType {
		case LimitScopePlatform, LimitScopeTenant, LimitScopeUser, LimitScopeAPIKey:
		default:
			return LimitPolicyFilter{}, newValidationError("scope_type", "unsupported scope_type")
		}
	}
	if filter.Status != "" {
		status, err := normalizeStatus(filter.Status)
		if err != nil {
			return LimitPolicyFilter{}, err
		}
		filter.Status = status
	}
	filter.ScopeID = strings.TrimSpace(filter.ScopeID)
	return filter, nil
}

func normalizeStatus(status Status) (Status, error) {
	switch status {
	case "":
		return StatusActive, nil
	case StatusActive, StatusDisabled:
		return status, nil
	default:
		return "", newValidationError("status", "status must be active or disabled")
	}
}

func validateOptionalNonNegative(field string, v *int) error {
	if v != nil && *v < 0 {
		return newValidationError(field, field+" must be >= 0")
	}
	return nil
}
