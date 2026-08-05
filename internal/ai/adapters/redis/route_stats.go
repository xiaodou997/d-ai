// Package redis provides a Redis-backed implementation of routing.RouteStatsStore.
package redis

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/ai/routing"
)

const (
	statsTTL      = time.Hour
	inflightTTL   = 35 * time.Minute
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
	s.invalidate(routeID)
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
	s.invalidate(routeID)
	key := inflightPrefix + routeID
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, inflightTTL)
	_, _ = pipe.Exec(ctx)
}

// DecrInflight decrements the inflight counter, floored at 0.
func (s *RedisRouteStats) DecrInflight(ctx context.Context, routeID string) {
	s.invalidate(routeID)
	key := inflightPrefix + routeID
	// Clamp to 0 so restarts can't drive the counter negative.
	const script = `
local v = tonumber(redis.call('GET', KEYS[1]))
if not v or v <= 0 then
  redis.call('SET', KEYS[1], 0, 'EX', ARGV[1])
  return 0
end
local next = redis.call('DECR', KEYS[1])
redis.call('EXPIRE', KEYS[1], ARGV[1])
return next
`
	_ = s.rdb.Eval(ctx, script, []string{key}, int(inflightTTL.Seconds())).Err()
}

// Snapshot returns a five-second-cached stats snapshot. All cache misses are
// fetched through one Redis pipeline so route count does not multiply latency.
func (s *RedisRouteStats) Snapshot(ctx context.Context, routeIDs []string) map[string]routing.RouteStats {
	out := make(map[string]routing.RouteStats, len(routeIDs))
	misses := make([]string, 0, len(routeIDs))
	now := time.Now()
	s.mu.RLock()
	for _, routeID := range routeIDs {
		if c, ok := s.cache[routeID]; ok && now.Sub(c.fetchAt) < localCacheTTL {
			out[routeID] = c.stats
		} else {
			misses = append(misses, routeID)
		}
	}
	s.mu.RUnlock()
	if len(misses) == 0 {
		return out
	}

	pipe := s.rdb.Pipeline()
	type commands struct {
		ewma     *redis.StringCmd
		inflight *redis.StringCmd
	}
	queued := make(map[string]commands, len(misses))
	for _, routeID := range misses {
		queued[routeID] = commands{
			ewma:     pipe.HGet(ctx, statsKeyPrefix+routeID, "ewma"),
			inflight: pipe.Get(ctx, inflightPrefix+routeID),
		}
	}
	_, _ = pipe.Exec(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, routeID := range misses {
		var stats routing.RouteStats
		cmd := queued[routeID]
		if value, err := cmd.ewma.Result(); err == nil {
			if parsed, parseErr := strconv.ParseFloat(value, 64); parseErr == nil {
				stats.EWMALatencyMs = parsed
			}
		}
		if value, err := cmd.inflight.Int64(); err == nil {
			stats.InflightCount = value
		}
		out[routeID] = stats
		s.cache[routeID] = cachedStats{stats: stats, fetchAt: now}
	}
	return out
}

func (s *RedisRouteStats) invalidate(routeID string) {
	s.mu.Lock()
	delete(s.cache, routeID)
	s.mu.Unlock()
}
