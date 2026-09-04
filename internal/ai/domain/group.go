package domain

import "time"

// Group 是租户直属零售单元，持零售价格表和默认用户倍率；
// 组通过 GroupTargetBinding 直连一批上游目标（账号/凭证池）。
type Group struct {
	ID                    string
	TenantID              string
	Name                  string
	Description           string
	RetailPriceBookID     string
	DefaultUserMultiplier float64
	UserDefaultVisible    bool
	// 协议转换网关开关：true = 允许本组候选作为跨格式协议转换目标（见 routes.go）。
	AllowProtocolConversion bool
	RoutePolicy             string
	RoutePolicyVersion      int64
	SortOrder               int32
	Status                  string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// GroupListItem 分组列表项（含对外价格表名）。
type GroupListItem struct {
	Group
	RetailPriceBookName string
}

// VisibleGroup is a tenant-owned active group with its default user pricing.
type VisibleGroup struct {
	Group
	EffectiveUserMultiplier float64
}

// UserGroup 租户→用户的分组例外（打开非默认公开分组 / 覆盖倍率）。
// 用户默认继承租户设置为 user_default_visible 的分组；本表用于个别用户例外。
type UserGroup struct {
	ID                     string
	TenantID               string
	UserID                 string
	GroupID                string
	UserMultiplierOverride *float64 // nil = 继承分组默认用户倍率
	CreatedBy              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	// list 投影附带
	GroupName                  string
	GroupDefaultUserMultiplier float64
}

// GroupTargetBinding 是分组 → 上游目标（账号或凭证池）的直连关联，对齐
// ai_group_targets。TargetKind/TargetID 直接表达多态目标，不再保留 account/pool
// 二选一的过渡壳。
type GroupTargetBinding struct {
	ID         string
	GroupID    string
	TargetKind string // "direct_upstream" | "oauth_pool"
	TargetID   string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// GroupTargetDetail 是 GroupTargetBinding 的列表/详情投影，附目标展示信息。
//
// Available/UnavailableReason 反映「该绑定当前对本租户是否仍可服务」：管理员把上游
// 资源改成 restricted、撤销 access_granted、或把资源停用后，存量绑定不会自动删除，
// 请求时会被 RuntimeBindingAuthorizer fail-closed 拒掉。这两个字段让租户端把这种
// 「配置看着正常、请求全失败」的哑故障显式呈现出来。仅在带租户上下文的投影里填充。
type GroupTargetDetail struct {
	GroupTargetBinding
	AccountName       string
	APIFormats        []string
	PoolName          string
	FixedProviderType string
	// Available 为该绑定的上游资源当前是否仍可被本租户路由（active 且 public 或已授权）。
	Available bool
	// UnavailableReason 在 Available=false 时给出原因：
	// "inactive"（资源停用/失效）、"access_revoked"（转 restricted 且未授权）、
	// "missing"（资源已删除）。Available=true 时为空。
	UnavailableReason string
}

// GroupModelDispatchRule maps a client-facing requested model name to the
// logical model_code that the group should route and bill against.
type GroupModelDispatchRule struct {
	ID              string
	GroupID         string
	ClientSurface   string
	MatchType       string
	MatchValue      string
	TargetModelCode string
	Priority        int32
	Status          string
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type GroupDispatchPreview struct {
	RequestedModel       string
	ClientSurface        string
	MatchedRule          *GroupModelDispatchRule
	ResolvedLogicalModel string
	CandidateUpstreams   []GroupDispatchPreviewCandidate
}

type GroupDispatchPreviewCandidate struct {
	TargetType         string
	AccountID          string
	CredentialPoolID   string
	DisplayName        string
	ProviderFamily     string
	SelectedProtocol   string
	UpstreamModel      string
	ProtocolConversion bool
}
