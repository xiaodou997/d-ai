package runtime

import (
	"time"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/core/upstream"
)

// ExecutionMode distinguishes an interactive request from queued work. It is
// part of the runtime contract because billing policy must not be inferred from
// the authentication method or endpoint that happened to enqueue the work.
type ExecutionMode string

const (
	ExecutionModeSync  ExecutionMode = "sync"
	ExecutionModeAsync ExecutionMode = "async"
)

// Request is the vNext runtime request descriptor after auth but before
// routing/bridge execution.
type Request struct {
	ExecutionMode   ExecutionMode
	RequestID       string
	TraceID         string
	Capability      catalog.Capability
	ClientSurface   surface.ID
	RequestedModel  string
	ResolvedModelID string
	GroupID         string
	ForcedGroupID   string
	Body            []byte
	Stream          bool
	ServiceTier     string
	// AllowedGroupIDs optionally constrains commercial dispatch to a subset of
	// the caller's accessible groups. The planner preserves the caller's group
	// order; this slice is a membership constraint, not an ordering source.
	AllowedGroupIDs []string
	ReceivedAt      time.Time
}

// PlannedTarget is one executable group-target binding. RouteID is always the
// ai_group_targets.id UUID. GroupRank is the caller-visible group failover rank;
// the targets inside a group are peers selected by its route policy.
type PlannedTarget struct {
	RouteID     string
	GroupRank   int
	Group       commercial.AccessibleGroup
	Target      commercial.GroupTarget
	ModelID     string
	MatchedRule *commercial.DispatchRule
	Binding     upstream.RuntimeBinding
}

// RoutePlan is the sole output of runtime route planning. Candidates are in
// deterministic structural order by group rank. Runtime health, conversion
// compatibility and the group policy choose among targets.
type RoutePlan struct {
	RequestID  string
	Candidates []PlannedTarget
}

// RouteInspection is the same planner/binder result used by Resolve, with
// rejected targets retained for non-executing preview and diagnostics.
type RouteInspection struct {
	RequestID          string
	Candidates         []PlannedTarget
	RejectedCandidates []RejectedTarget
}

type RejectionCode string

const (
	RejectionNoActiveTarget        RejectionCode = "no_active_group_target"
	RejectionAccessDenied          RejectionCode = "access_denied"
	RejectionTargetNotFound        RejectionCode = "target_not_found"
	RejectionTargetInactive        RejectionCode = "target_inactive"
	RejectionModelBindingMissing   RejectionCode = "model_binding_missing"
	RejectionProtocolIncompatible  RejectionCode = "protocol_incompatible"
	RejectionCredentialUnavailable RejectionCode = "credential_unavailable"
	RejectionBindingInvalid        RejectionCode = "binding_invalid"
	RejectionBindingUnavailable    RejectionCode = "binding_unavailable"
)

type RejectedTarget struct {
	RouteID     string
	GroupRank   int
	Group       commercial.AccessibleGroup
	Target      commercial.GroupTarget
	ModelID     string
	MatchedRule *commercial.DispatchRule
	Code        RejectionCode
	Detail      string
}

// Result is the normalized runtime outcome recorded into usage/audit.
type Result struct {
	RequestID            string
	Capability           catalog.Capability
	ClientSurface        surface.ID
	UpstreamSurface      surface.ID
	StatusCode           int
	RequestStatus        string
	ResponseCommitted    bool
	RouteID              string
	Body                 []byte
	Usage                map[string]any
	CatalogBaseMicro     int64
	TenantPayableMicro   int64
	UserPayableMicro     int64
	UserChargedMicro     int64
	APIKeyQuotaCostMicro int64
	CallerChargeMicro    int64
	ErrorCode            string
	ErrorMessage         string
	InternalErrorDetail  string
	FailedStep           string
	CreatedAt            time.Time
}
