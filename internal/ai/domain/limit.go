package domain

import "time"

// RuntimeLimitPolicy is the management-domain view of a row in
// ai_runtime_limit_policies. The concurrency limit is optional (nil = no
// limit). It is distinct from LimitPolicy, the slimmer serving projection.
type RuntimeLimitPolicy struct {
	ID               string
	ScopeType        string
	ScopeID          string
	ConcurrencyLimit *int32
	Status           string
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
