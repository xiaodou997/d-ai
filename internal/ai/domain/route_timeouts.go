package domain

import "time"

// DefaultRouteTimeoutPreset is the system-wide budget for every upstream
// attempt. Upstream accounts and credential pools cannot override it.
var DefaultRouteTimeoutPreset = RouteTimeouts{
	ResponseHeader: 5 * time.Minute,
	FirstByte:      5 * time.Minute,
	Idle:           3 * time.Minute,
	MaxDuration:    15 * time.Minute,
}

// DefaultRouteTimeouts keeps the capability argument at call sites while all
// capabilities share one system policy.
func DefaultRouteTimeouts(_ CapabilityType) RouteTimeouts {
	return DefaultRouteTimeoutPreset
}
