package routing

import "time"

// ScopeType declares where a routing score policy applies.
type ScopeType string

const (
	ScopeGlobal   ScopeType = "global"
	ScopeTenant   ScopeType = "tenant"
	ScopeGroup    ScopeType = "group"
	ScopeUpstream ScopeType = "upstream"
)

// WeightSet stores the relative importance of routing dimensions.
type WeightSet struct {
	Cost    float64 `json:"cost"`
	Latency float64 `json:"latency"`
	Load    float64 `json:"load"`
	Health  float64 `json:"health"`
}

// IsZero reports whether all dimensions are unset.
func (w WeightSet) IsZero() bool {
	return w.Cost == 0 && w.Latency == 0 && w.Load == 0 && w.Health == 0
}

// Policy is the persisted routing score policy row.
type Policy struct {
	ID        string
	ScopeType ScopeType
	ScopeID   string
	Weights   WeightSet
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DefaultPolicy returns the default global routing weight set.
func DefaultPolicy() Policy {
	return Policy{
		ScopeType: ScopeGlobal,
		ScopeID:   "global",
		Weights: WeightSet{
			Cost:    0.4,
			Latency: 0.3,
			Load:    0.2,
			Health:  0.1,
		},
	}
}
