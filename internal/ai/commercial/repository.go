package commercial

import (
	"context"

	"xiaodou/dai/internal/ai/core/surface"
)

// Repository is the vNext persistence port for the commercial control plane.
type Repository interface {
	CreateGroup(ctx context.Context, tenantID string, in GroupWrite) (Group, error)
	ListGroups(ctx context.Context, tenantID string) ([]Group, error)
	GetGroup(ctx context.Context, scope TenantGroupScope) (Group, error)
	UpdateGroup(ctx context.Context, scope TenantGroupScope, in GroupWrite) (Group, error)
	UpdateGroupRoutePolicy(ctx context.Context, scope TenantGroupScope, in GroupRoutePolicyWrite) (Group, error)
	UpdateGroupStatus(ctx context.Context, scope TenantGroupScope, status Status) (Group, error)
	DeleteGroup(ctx context.Context, scope TenantGroupScope) error
	LoadDispatchData(ctx context.Context, tenantID string, groupIDs []string) (DispatchData, error)

	ListGroupClientSurfaces(ctx context.Context, scope TenantGroupScope) ([]GroupClientSurface, error)
	ReplaceGroupClientSurfaces(ctx context.Context, scope TenantGroupScope, entries []GroupClientSurfaceWrite) error

	AddGroupTarget(ctx context.Context, scope TenantGroupScope, in GroupTargetWrite) (GroupTarget, error)
	ListGroupTargets(ctx context.Context, scope TenantGroupScope) ([]GroupTarget, error)
	ListGroupTargetDetails(ctx context.Context, scope TenantGroupScope) ([]GroupTargetDetail, error)
	ListGroupTargetsByTarget(ctx context.Context, targetKind TargetKind, targetID string) ([]GroupTargetDetail, error)
	GetGroupTargetDetail(ctx context.Context, scope TenantGroupScope, id string) (GroupTargetDetail, error)
	UpdateGroupTarget(ctx context.Context, scope TenantGroupScope, id string, in GroupTargetWrite) (GroupTarget, error)
	DeleteGroupTarget(ctx context.Context, scope TenantGroupScope, id string) error
	ReplaceGroupTargets(ctx context.Context, scope TenantGroupScope, in GroupTargetBatchWrite) (GroupTargetBatchResult, error)

	AddDispatchRule(ctx context.Context, scope TenantGroupScope, in DispatchRuleWrite) (DispatchRule, error)
	ListDispatchRules(ctx context.Context, scope TenantGroupScope) ([]DispatchRule, error)
	UpdateDispatchRule(ctx context.Context, scope TenantGroupScope, id string, in DispatchRuleWrite) (DispatchRule, error)
	UpdateDispatchRuleStatus(ctx context.Context, scope TenantGroupScope, id string, status Status) (DispatchRule, error)
	DeleteDispatchRule(ctx context.Context, scope TenantGroupScope, id string) error
	PreviewDispatch(ctx context.Context, scope TenantGroupScope, requestedModel string, clientSurface surface.ID) (DispatchPreview, error)
	ListDispatchModels(ctx context.Context, scope TenantGroupScope, clientSurface surface.ID) ([]DispatchModel, error)

	UpsertUserBinding(ctx context.Context, in UserGroupBindingWrite) (UserGroupBinding, error)
	ListUserBindings(ctx context.Context, tenantID, userID string) ([]UserGroupBinding, error)
	DeleteUserBinding(ctx context.Context, tenantID, userID, groupID string) error

	CreateLimitPolicy(ctx context.Context, in LimitPolicyWrite) (LimitPolicy, error)
	ListLimitPolicies(ctx context.Context, filter LimitPolicyFilter) ([]LimitPolicy, error)
	UpdateLimitPolicy(ctx context.Context, id string, in LimitPolicyWrite) (LimitPolicy, error)
	UpdateLimitPolicyStatus(ctx context.Context, id string, status Status) (LimitPolicy, error)
	DeleteLimitPolicies(ctx context.Context, filter LimitPolicyFilter) error
}

// DispatchData is the immutable configuration snapshot used for one dispatch
// decision. Repositories load all requested groups in one round trip.
type DispatchData struct {
	ClientSurfaces map[string][]GroupClientSurface
	Rules          map[string][]DispatchRule
	Targets        map[string][]GroupTarget
}

type GroupWrite struct {
	Code                    string
	Name                    string
	Description             string
	RetailPriceBookID       string
	DefaultUserMultiplier   float64
	UserDefaultVisible      bool
	AllowProtocolConversion bool
	RoutePolicy             RoutePolicy
	// ExpectedRoutePolicyVersion is optional for create/legacy callers. When
	// supplied on an update, the adapter rejects a stale full-group form rather
	// than allowing it to overwrite a newer route-policy edit.
	ExpectedRoutePolicyVersion int64
	Status                     Status
	SortOrder                  int
}

// GroupRoutePolicyWrite is the narrow mutation used by the route-policy panel.
// Keeping it separate from GroupWrite prevents changing a routing policy from
// overwriting unrelated pricing, visibility, or status fields.
type GroupRoutePolicyWrite struct {
	ExpectedVersion int64
	RoutePolicy     RoutePolicy
}

type GroupClientSurfaceWrite struct {
	Surface       string
	BridgeEnabled bool
	Status        Status
}

type GroupTargetWrite struct {
	TargetKind TargetKind
	TargetID   string
	Status     Status
}

// GroupTargetBatchWrite describes the complete desired target set for a group.
// The repository applies it in one transaction against ExpectedVersion.
type GroupTargetBatchWrite struct {
	ExpectedVersion int64
	Targets         []GroupTargetWrite
}

type GroupTargetBatchResult struct {
	RoutePolicyVersion int64
	Targets            []GroupTarget
}

type DispatchRuleWrite struct {
	ClientSurface surface.ID
	MatchType     DispatchMatchType
	MatchValue    string
	TargetModelID string
	Priority      int
	Notes         string
}

type DispatchModel struct {
	ModelCode        string
	Capability       string
	AvailableTargets int
}

type UserGroupBindingWrite struct {
	TenantID               string
	UserID                 string
	GroupID                string
	UserMultiplierOverride *float64
	CreatedBy              string
	UpdatedBy              string
}

type LimitPolicyWrite struct {
	ScopeType        LimitScope
	ScopeID          string
	Capability       string
	ModelID          string
	ConcurrencyLimit *int
	Status           Status
	CreatedBy        string
}

type LimitPolicyFilter struct {
	ScopeType LimitScope
	ScopeID   string
	Status    Status
}
