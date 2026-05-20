// Package redis implements serving.RateLimiter using Redis sliding-window counters.
package redis

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/serving"
)

// RateLimiter implements serving.RateLimiter.
//
// For each active policy matching the request's scope (tenant / user / api_key /
// provider / endpoint), it enforces:
//   - RPM (requests per minute): sliding-window counter, cost=1 per request
//   - TPM (tokens per minute):   sliding-window counter, cost=defaultTokenEstimate
//     because actual token counts are not yet known at pre-flight check time
//
// Each policy uses a Redis sorted set:
//
//	Key:    ratelimit:{policy_id}:sw:rpm  /  ratelimit:{policy_id}:sw:tpm
//	Score:  Unix timestamp in milliseconds
//	Member: "{cost}:{random_id}"
//
// A single Lua script removes stale entries, sums current costs, and either
// rejects the request or records it — all atomically.
type RateLimiter struct {
	redis                *redis.Client
	q                    *dbgen.Queries
	defaultTokenEstimate int
}

func NewRateLimiter(rdb *redis.Client, q *dbgen.Queries, defaultTokenEstimate int) *RateLimiter {
	if defaultTokenEstimate <= 0 {
		defaultTokenEstimate = 4096
	}
	return &RateLimiter{redis: rdb, q: q, defaultTokenEstimate: defaultTokenEstimate}
}

// slidingWindowScript is the Lua script for atomic sliding-window rate limiting.
//
// KEYS[1]:  sorted-set key
// ARGV[1]:  now_ms   — current Unix time in milliseconds
// ARGV[2]:  window_ms — window size in milliseconds (60000 for per-minute)
// ARGV[3]:  cost      — weight of this request (1 for RPM, token estimate for TPM)
// ARGV[4]:  limit     — maximum allowed cost in the window
// ARGV[5]:  member    — unique identifier for this request entry
// ARGV[6]:  ttl_s     — TTL in seconds to set on the key
//
// Returns a two-element array: {current_total, allowed}
//   - current_total: total cost in the window after this call
//   - allowed: 1 if the request is within limits, 0 if rejected
var slidingWindowScript = redis.NewScript(`
local key       = KEYS[1]
local now_ms    = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local cost      = tonumber(ARGV[3])
local limit     = tonumber(ARGV[4])
local member    = ARGV[5]
local ttl_s     = tonumber(ARGV[6])

-- Remove entries older than the window
redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms - window_ms)

-- Sum costs of remaining entries (cost is encoded as the prefix before ':')
local entries = redis.call('ZRANGE', key, 0, -1)
local current = 0
for _, entry in ipairs(entries) do
    local sep = string.find(entry, ':')
    if sep then
        current = current + tonumber(string.sub(entry, 1, sep - 1))
    end
end

-- Refresh TTL regardless of outcome so the key doesn't linger after bursts
redis.call('EXPIRE', key, ttl_s)

if current + cost > limit then
    return {current, 0}
end

redis.call('ZADD', key, now_ms, cost .. ':' .. member)
return {current + cost, 1}
`)

const windowMs = 60_000 // 1 minute in milliseconds
const windowTTL = 120   // keep keys for 2 windows

// Check loads active policies for the request and enforces RPM / TPM limits.
// Returns an error (with a human-readable message) if any limit is exceeded.
func (r *RateLimiter) Check(ctx context.Context, req *serving.Request) error {
	if req.APIKey == nil || req.Candidate == nil {
		return nil
	}

	policies, err := r.q.ListActiveRuntimeLimitPolicies(ctx, dbgen.ListActiveRuntimeLimitPoliciesParams{
		CapabilityType: string(req.CapabilityType),
		ScopeID:        req.APIKey.TenantID,
		ScopeID_2:      req.APIKey.UserID,
		ScopeID_3:      req.APIKey.KeyID,
		ScopeID_4:      req.Candidate.ProviderCode,
		ScopeID_5:      req.Candidate.EndpointID,
		ModelCode:      pgtype.Text{},
	})
	if err != nil {
		// Non-fatal: unable to load policies → skip limiting
		return nil
	}

	nowMs := time.Now().UnixMilli()

	for _, p := range policies {
		id := uuidString(p.ID)
		member := fmt.Sprintf("%d-%d", nowMs, rand.Int63())

		if p.RpmLimit.Valid {
			key := fmt.Sprintf("ratelimit:%s:sw:rpm", id)
			result, err := slidingWindowScript.Run(ctx, r.redis, []string{key},
				nowMs, windowMs, 1, int64(p.RpmLimit.Int32), member, windowTTL,
			).Int64Slice()
			if err != nil {
				continue // Redis error → skip this policy
			}
			if len(result) == 2 && result[1] == 0 {
				return fmt.Errorf("rate limit exceeded: %s scope %q rpm=%d",
					p.ScopeType, p.ScopeID, p.RpmLimit.Int32)
			}
		}

		if p.TpmLimit.Valid {
			key := fmt.Sprintf("ratelimit:%s:sw:tpm", id)
			result, err := slidingWindowScript.Run(ctx, r.redis, []string{key},
				nowMs, windowMs, int64(r.defaultTokenEstimate), int64(p.TpmLimit.Int32), member, windowTTL,
			).Int64Slice()
			if err != nil {
				continue
			}
			if len(result) == 2 && result[1] == 0 {
				return fmt.Errorf("rate limit exceeded: %s scope %q tpm=%d",
					p.ScopeType, p.ScopeID, p.TpmLimit.Int32)
			}
		}
	}
	return nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
