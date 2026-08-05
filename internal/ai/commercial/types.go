package commercial

import (
	"time"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// TenantGroupScope is the only identity accepted by tenant-owned group
// operations. Request bodies never carry tenant or group ownership.
type TenantGroupScope struct {
	TenantID string
	GroupID  string
}

// Group is the product-side sellable unit.
type Group struct {
	ID                      string
	TenantID                string
	Code                    string
	Name                    string
	Description             string
	RetailPriceBookID       string
	DefaultUserMultiplier   float64
	UserDefaultVisible      bool
	AllowProtocolConversion bool
	Status                  Status
	SortOrder               int
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// GroupClientSurface declares which client surfaces a group accepts and
// whether bridge execution is allowed for that surface.
type GroupClientSurface struct {
	ID            string
	GroupID       string
	Surface       surface.ID
	BridgeEnabled bool
	Status        Status
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type GroupClientSurfacePolicyMode string

const (
	GroupClientSurfacePolicyAll        GroupClientSurfacePolicyMode = "all"
	GroupClientSurfacePolicyRestricted GroupClientSurfacePolicyMode = "restricted"
)

type GroupClientSurfacePolicy struct {
	GroupID         string
	Mode            GroupClientSurfacePolicyMode
	AllowedSurfaces []surface.ID
}

type GroupClientSurfacePolicyWrite struct {
	Mode            GroupClientSurfacePolicyMode
	AllowedSurfaces []surface.ID
}

type TargetKind string

const (
	TargetKindDirectUpstream TargetKind = "direct_upstream"
	TargetKindOAuthPool      TargetKind = "oauth_pool"
)

// GroupTarget connects a group to one upstream resource.
type GroupTarget struct {
	ID         string
	GroupID    string
	TargetKind TargetKind
	TargetID   string
	Priority   int
	Status     Status
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type GroupTargetDetail struct {
	GroupTarget
	AccountName       string
	DefaultProtocol   string
	PoolName          string
	FixedProviderType string
	// Available/UnavailableReason 表达该绑定当前对本租户是否仍可服务，用于把
	// 授权吊销后「仍显示已绑定、请求却被拒」的哑故障显式化。见 domain.GroupTargetDetail。
	Available         bool
	UnavailableReason string
}

type DispatchMatchType string

const (
	DispatchMatchExact    DispatchMatchType = "exact"
	DispatchMatchPrefix   DispatchMatchType = "prefix"
	DispatchMatchWildcard DispatchMatchType = "wildcard"
	DispatchMatchRegex    DispatchMatchType = "regex"
)

// DispatchRule resolves a requested model name from a concrete client surface
// to an internal model.
type DispatchRule struct {
	ID                 string
	GroupID            string
	ClientSurface      surface.ID
	MatchType          DispatchMatchType
	MatchValue         string
	TargetModelID      string
	Priority           int
	Status             Status
	Notes              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	RequiredCapability string
	PriceState         string
	CanEnable          bool
}

const (
	DispatchRulePriceStatePriced   = "priced"
	DispatchRulePriceStateUnpriced = "unpriced"
)

type DispatchPreview struct {
	RequestedModel     string
	ClientSurface      string
	MatchedRule        *DispatchRule
	ResolvedModelID    string
	CandidateUpstreams []DispatchPreviewCandidate
	RejectedCandidates []DispatchPreviewRejection
}

type DispatchPreviewCandidate struct {
	TargetType         string
	AccountID          string
	CredentialPoolID   string
	DisplayName        string
	ProviderFamily     string
	SelectedProtocol   string
	UpstreamModel      string
	ProtocolConversion bool
	Priority           int
}

type DispatchPreviewRejection struct {
	TargetType      string
	TargetID        string
	ResolvedModelID string
	ReasonCode      string
	ReasonDetail    string
	Priority        int
}

// UserGroupBinding is an explicit user-to-group authorization relation.
type UserGroupBinding struct {
	ID                     string
	TenantID               string
	UserID                 string
	GroupID                string
	UserMultiplierOverride *float64
	CreatedBy              string
	UpdatedBy              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// AccessibleGroup is the effective sellable group view for one runtime caller
// after visibility and tenant/user/API key narrowing are applied.
type AccessibleGroup struct {
	Group                   Group
	EffectiveUserMultiplier float64
	UserDefaultVisible      bool
	UserBound               bool
	APIKeyBound             bool
}

// DispatchResolution is the commercial dispatch result before upstream binding
// resolution happens.
type DispatchResolution struct {
	Group           AccessibleGroup
	RequestedModel  string
	ResolvedModelID string
	MatchedRule     *DispatchRule
	Targets         []GroupTarget
}

type LimitScope string

const (
	LimitScopePlatform  LimitScope = "platform"
	LimitScopeTenant    LimitScope = "tenant"
	LimitScopeUser      LimitScope = "user"
	LimitScopeAPIKey    LimitScope = "api_key"
	LimitScopeInvokeKey LimitScope = "invoke_key"
)

// LimitPolicy is the runtime-facing commercial throttling policy.
type LimitPolicy struct {
	ID               string
	ScopeType        LimitScope
	ScopeID          string
	Capability       *catalog.Capability
	ModelID          string
	ConcurrencyLimit *int
	Status           Status
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
