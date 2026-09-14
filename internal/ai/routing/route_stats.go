package routing

import (
	"context"
	"time"
)

// RouteStats holds real-time scoring signals for one upstream route.
type RouteStats struct {
	EWMAFirstByteMs float64
	EWMATotalMs     float64
	Successes       int64
	Failures        int64

	EWMALatencyMs float64
	InflightCount int64
}

// RouteStatsStore reads and writes per-route latency and inflight counters.
// Implementations must be safe for concurrent use.
type RouteStatsStore interface {
	// RecordLatency updates the EWMA latency for routeID after a call completes.
	RecordLatency(ctx context.Context, routeID string, latencyMs int)
	// IncrInflight atomically increments the in-flight request counter.
	IncrInflight(ctx context.Context, routeID string)
	// DecrInflight atomically decrements the in-flight request counter.
	DecrInflight(ctx context.Context, routeID string)
	// Snapshot returns scoring signals for all requested routes in one backing-
	// store operation. Missing routes have zero-value stats.
	Snapshot(ctx context.Context, routeIDs []string) map[string]RouteStats
}

// NoopRouteStats is a zero-allocation fallback for when Redis is unavailable.
// All stats return zero, which causes the scorer to fall back to
// priority-tier random selection.
type NoopRouteStats struct{}

func (NoopRouteStats) RecordLatency(_ context.Context, _ string, _ int) {}
func (NoopRouteStats) IncrInflight(_ context.Context, _ string)         {}
func (NoopRouteStats) DecrInflight(_ context.Context, _ string)         {}
func (NoopRouteStats) Snapshot(_ context.Context, routeIDs []string) map[string]RouteStats {
	return make(map[string]RouteStats, len(routeIDs))
}

// OutcomeStats records only actual provider results; client/local failures are excluded.
type OutcomeStats interface {
	Observe(context.Context, string, bool, bool, int, int)
}

// InflightLeaseStats expires individual in-flight calls after process crashes.
type InflightLeaseStats interface {
	BeginInflight(context.Context, string, string, time.Duration)
	EndInflight(context.Context, string, string)
}
