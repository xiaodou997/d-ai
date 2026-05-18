package routing

import "context"

// RouteStats holds real-time scoring signals for one upstream route.
type RouteStats struct {
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
	// Stats returns the current scoring signals. Returns zero-value when the
	// backing store (Redis) is unavailable.
	Stats(ctx context.Context, routeID string) RouteStats
}

// NoopRouteStats is a zero-allocation fallback for when Redis is unavailable.
// All stats return zero, which causes the scorer to fall back to
// priority+weighted random selection.
type NoopRouteStats struct{}

func (NoopRouteStats) RecordLatency(_ context.Context, _ string, _ int) {}
func (NoopRouteStats) IncrInflight(_ context.Context, _ string)         {}
func (NoopRouteStats) DecrInflight(_ context.Context, _ string)         {}
func (NoopRouteStats) Stats(_ context.Context, _ string) RouteStats     { return RouteStats{} }
