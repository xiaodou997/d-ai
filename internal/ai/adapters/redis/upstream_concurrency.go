package redis

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/ai/serving"
)

// upstreamConcurrencyAcquireScript atomically evicts expired leases and claims
// one slot. Members are scored by lease expiry so a crashed process cannot
// strand a slot forever, and re-acquiring under the same member is idempotent
// (a retry against the same account reuses its slot instead of double-counting).
var upstreamConcurrencyAcquireScript = redis.NewScript(`
local key        = KEYS[1]
local now_ms     = tonumber(ARGV[1])
local expires_ms = tonumber(ARGV[2])
local limit      = tonumber(ARGV[3])
local member     = ARGV[4]
local ttl_ms     = tonumber(ARGV[5])

redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms)
if redis.call('ZSCORE', key, member) then
    redis.call('ZADD', key, expires_ms, member)
    redis.call('PEXPIRE', key, ttl_ms)
    return {redis.call('ZCARD', key), 1}
end
local current = redis.call('ZCARD', key)
if current >= limit then
    redis.call('PEXPIRE', key, ttl_ms)
    return {current, 0}
end
redis.call('ZADD', key, expires_ms, member)
redis.call('PEXPIRE', key, ttl_ms)
return {current + 1, 1}
`)

// UpstreamConcurrencyLimiter caps how many outbound requests may be in flight
// against one direct upstream account at the same time.
//
// Concurrency rather than requests-per-minute: LLM call durations span orders
// of magnitude (a short completion is under a second, a streamed conversation
// runs for minutes), so a per-minute request count does not track the resource
// the upstream actually runs out of. One slot is held for exactly as long as
// one upstream attempt occupies the upstream.
type UpstreamConcurrencyLimiter struct {
	redis *redis.Client
	now   func() time.Time
}

func NewUpstreamConcurrencyLimiter(client *redis.Client) *UpstreamConcurrencyLimiter {
	return &UpstreamConcurrencyLimiter{redis: client, now: time.Now}
}

type upstreamSlot struct {
	redis  *redis.Client
	key    string
	member string
}

// Release returns the slot. It deliberately detaches from the caller's context:
// the common release path runs while the request context is already cancelled
// (client disconnected, deadline exceeded), and a cancelled release would leak
// the slot until its lease expires.
func (s *upstreamSlot) Release(ctx context.Context) {
	if s == nil || s.redis == nil || s.key == "" {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = s.redis.ZRem(releaseCtx, s.key, s.member).Err()
}

// Acquire claims one concurrency slot for accountID. A nil slot is returned
// when the account has no limit configured; callers may always call Release on
// the result.
func (l *UpstreamConcurrencyLimiter) Acquire(ctx context.Context, accountID, requestID string, limit int, ttl time.Duration) (serving.UpstreamSlot, error) {
	if limit <= 0 || accountID == "" {
		return nil, nil
	}
	if l == nil || l.redis == nil {
		return nil, serving.ErrUpstreamConcurrencyLimiterUnavailable
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	nowMs := l.now().UnixMilli()
	key := fmt.Sprintf("ratelimit:upstream-account:%s:concurrency", accountID)
	member := requestID
	if member == "" {
		member = fmt.Sprintf("anonymous-%d-%d", nowMs, rand.Int63())
	}
	result, err := upstreamConcurrencyAcquireScript.Run(ctx, l.redis, []string{key},
		nowMs, nowMs+ttl.Milliseconds(), int64(limit), member,
		ttl.Milliseconds()+time.Minute.Milliseconds(),
	).Int64Slice()
	if err != nil {
		return nil, fmt.Errorf("%w: account %s: %v", serving.ErrUpstreamConcurrencyLimiterUnavailable, accountID, err)
	}
	if len(result) != 2 {
		return nil, fmt.Errorf("%w: account %s returned invalid result", serving.ErrUpstreamConcurrencyLimiterUnavailable, accountID)
	}
	if result[1] == 0 {
		return nil, fmt.Errorf("%w: account %s concurrency=%d", serving.ErrUpstreamConcurrencyExceeded, accountID, limit)
	}
	return &upstreamSlot{redis: l.redis, key: key, member: member}, nil
}
