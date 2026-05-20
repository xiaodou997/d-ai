// Package redis provides a Redis-backed implementation of routing.RouteStatsStore.
package redis

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xiaodou/uni-ai-api/internal/routing"
)

const (
	statsTTL      = time.Hour
	inflightTTL   = 60 * time.Second
	localCacheTTL = 5 * time.Second

	statsKeyPrefix = "route:stats:"
	inflightPrefix = "route:inflight:"
)

type cachedStats struct {
	stats   routing.RouteStats
	fetchAt time.Time
}

// RedisRouteStats implements routing.RouteStatsStore backed by Redis.
// All writes are best-effort: failures are silently discarded so that a Redis
// outage never blocks request processing.
type RedisRouteStats struct {
	rdb *redis.Client

	mu    sync.RWMutex
	cache map[string]cachedStats
}

// NewRedisRouteStats creates a RedisRouteStats. rdb must not be nil.
func NewRedisRouteStats(rdb *redis.Client) *RedisRouteStats {
	return &RedisRouteStats{
		rdb:   rdb,
		cache: make(map[string]cachedStats),
	}
}

// RecordLatency updates the EWMA latency for routeID in Redis using a Lua
// atomic read-modify-write (α = 0.1). Errors are silently dropped.
func (s *RedisRouteStats) RecordLatency(ctx context.Context, routeID string, latencyMs int) {
	key := statsKeyPrefix + routeID
	const script = `
local cur = redis.call('HGET', KEYS[1], 'ewma')
local ewma
if cur == false then
  ewma = tonumber(ARGV[1])
else
  ewma = 0.1 * tonumber(ARGV[1]) + 0.9 * tonumber(cur)
end
redis.call('HSET', KEYS[1], 'ewma', ewma)
redis.call('EXPIRE', KEYS[1], ARGV[2])
return tostring(ewma)
`
	ttlSecs := int(statsTTL.Seconds())
	_ = s.rdb.Eval(ctx, script, []string{key},
		strconv.Itoa(latencyMs), strconv.Itoa(ttlSecs)).Err()
}

// IncrInflight increments the inflight counter for routeID and refreshes its TTL.
func (s *RedisRouteStats) IncrInflight(ctx context.Context, routeID string) {
	key := inflightPrefix + routeID
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, inflightTTL)
	_, _ = pipe.Exec(ctx)
}

// DecrInflight decrements the inflight counter, floored at 0.
func (s *RedisRouteStats) DecrInflight(ctx context.Context, routeID string) {
	key := inflightPrefix + routeID
	// Clamp to 0 so restarts can't drive the counter negative.
	const script = `
local v = tonumber(redis.call('GET', KEYS[1]))
if not v or v <= 0 then
  redis.call('SET', KEYS[1], 0)
  return 0
end
return redis.call('DECR', KEYS[1])
`
	_ = s.rdb.Eval(ctx, script, []string{key}).Err()
}

// Stats returns 5-second-cached scoring signals for routeID.
// Returns zero-value RouteStats on any Redis error.
func (s *RedisRouteStats) Stats(ctx context.Context, routeID string) routing.RouteStats {
	s.mu.RLock()
	if c, ok := s.cache[routeID]; ok && time.Since(c.fetchAt) < localCacheTTL {
		s.mu.RUnlock()
		return c.stats
	}
	s.mu.RUnlock()

	stats := s.fetchFromRedis(ctx, routeID)

	s.mu.Lock()
	s.cache[routeID] = cachedStats{stats: stats, fetchAt: time.Now()}
	s.mu.Unlock()
	return stats
}

func (s *RedisRouteStats) fetchFromRedis(ctx context.Context, routeID string) routing.RouteStats {
	pipe := s.rdb.Pipeline()
	ewmaCmd := pipe.HGet(ctx, statsKeyPrefix+routeID, "ewma")
	inflightCmd := pipe.Get(ctx, inflightPrefix+routeID)
	_, _ = pipe.Exec(ctx)

	var stats routing.RouteStats
	if v, err := ewmaCmd.Result(); err == nil {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			stats.EWMALatencyMs = f
		}
	}
	if v, err := inflightCmd.Int64(); err == nil {
		stats.InflightCount = v
	}
	return stats
}
