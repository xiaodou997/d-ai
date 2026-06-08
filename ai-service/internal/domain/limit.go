package domain

import "time"

// RuntimeLimitPolicy is the management-domain view of a row in
// ai_runtime_limit_policies. ModelCode and the three throttle limits are
// optional (nil = NULL column / no limit). It is distinct from LimitPolicy, the
// slimmer projection used by the serving rate-limiter.
type RuntimeLimitPolicy struct {
	ID               string
	ScopeType        string
	ScopeID          string
	CapabilityType   string
	ModelCode        *string
	RpmLimit         *int32
	TpmLimit         *int32
	ConcurrencyLimit *int32
	Status           string
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
