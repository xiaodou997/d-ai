// Package redis implements distributed runtime limits.
package redis

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/serving"
)

// RateLimiter enforces caller-side concurrency policies for tenant, user, and
// API-key scopes.
//
// It also carries the default in-flight cap, which is what bounds billing
// overshoot. Settlement is post-paid: admission stops the next request once the
// balance goes negative, but it cannot stop the requests already in flight, so
// the worst overdraft an account can reach is
//
//	in-flight cap × cost of the most expensive single request
//
// Without a cap that product is unbounded — a caller with one cent left can
// open five hundred connections at once. The default exists so the number is
// finite by construction; an explicit policy row overrides it per scope.
type RateLimiter struct {
	redis           *redis.Client
	q               *dbgen.Queries
	defaultInFlight int
}

func NewRateLimiter(rdb *redis.Client, q *dbgen.Queries) *RateLimiter {
	return &RateLimiter{redis: rdb, q: q, defaultInFlight: DefaultInFlightPerAccount}
}

// DefaultInFlightPerAccount bounds concurrent billed requests for one tenant or
// one end user when no explicit policy is configured. It is deliberately
// generous: the job is to make the overdraft computable, not to shape traffic.
const DefaultInFlightPerAccount = 32

// WithDefaultInFlight overrides the default cap. Zero disables it, which
// restores the previous unbounded behaviour and with it an unbounded overdraft.
func (r *RateLimiter) WithDefaultInFlight(limit int) *RateLimiter {
	if limit < 0 {
		limit = 0
	}
	r.defaultInFlight = limit
	return r
}

// concurrencyAcquireScript atomically removes expired leases and acquires one
// slot. Re-acquiring the same request ID is idempotent.
var concurrencyAcquireScript = redis.NewScript(`
local key       = KEYS[1]
local now_ms    = tonumber(ARGV[1])
local expires_ms = tonumber(ARGV[2])
local limit     = tonumber(ARGV[3])
local member    = ARGV[4]
local ttl_ms    = tonumber(ARGV[5])

redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms)
if redis.call('ZSCORE', key, member) then
    redis.call('PEXPIRE', key, ttl_ms)
    return {redis.call('ZCARD', key), 1}
end
local current = redis.call('ZCARD', key)
if current >= limit then
    return {current, 0}
end
redis.call('ZADD', key, expires_ms, member)
redis.call('PEXPIRE', key, ttl_ms)
return {current + 1, 1}
`)

type concurrencyLease struct {
	redis     *redis.Client
	requestID string
	keys      []string
}

func (l *concurrencyLease) Release(ctx context.Context) {
	if l == nil || l.redis == nil || len(l.keys) == 0 {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	pipe := l.redis.Pipeline()
	for _, key := range l.keys {
		pipe.ZRem(releaseCtx, key, l.requestID)
	}
	_, _ = pipe.Exec(releaseCtx)
}

// Acquire loads active policies and acquires every configured concurrency slot.
// Policy/Redis failures fail closed because admissibility cannot be inferred.
func (r *RateLimiter) Acquire(ctx context.Context, req *serving.Request) (serving.RateLimitLease, error) {
	subject := req.RuntimeSubject()
	if subject == nil {
		return nil, nil
	}
	// Only API-key callers carry an API-key scope. JWT web calls leave it
	// empty so the api_key scope branch of the policy query cannot match.
	isAPIKeyAuth := subject.AuthMethod == coreidentity.AuthMethodAPIKey
	apiKeyScope := ""
	if isAPIKeyAuth {
		apiKeyScope = subject.APIKeyID
	}

	policies, err := r.q.ListActiveRuntimeLimitPolicies(ctx, dbgen.ListActiveRuntimeLimitPoliciesParams{
		ScopeID:   subject.TenantID,
		ScopeID_2: subject.UserID,
		ScopeID_3: apiKeyScope,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: load policies: %v", serving.ErrRateLimiterUnavailable, err)
	}

	nowMs := time.Now().UnixMilli()
	requestID := req.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("anonymous-%d-%d", nowMs, rand.Int63())
	}
	lease := &concurrencyLease{redis: r.redis, requestID: requestID}
	releaseOnError := func(err error) (serving.RateLimitLease, error) {
		lease.Release(ctx)
		return nil, err
	}

	acquire := func(key, scopeType, scopeID string, limit int64) error {
		ttl := concurrencyTTL(req)
		result, err := concurrencyAcquireScript.Run(ctx, r.redis, []string{key},
			nowMs, nowMs+ttl.Milliseconds(), limit, requestID,
			ttl.Milliseconds()+time.Minute.Milliseconds(),
		).Int64Slice()
		if err != nil {
			return fmt.Errorf("%w: concurrency %s %q: %v", serving.ErrRateLimiterUnavailable, scopeType, scopeID, err)
		}
		if len(result) != 2 || result[1] == 0 {
			return fmt.Errorf("%w: %s scope %q concurrency=%d",
				serving.ErrRateLimitExceeded, scopeType, scopeID, limit)
		}
		lease.keys = append(lease.keys, key)
		return nil
	}

	// An explicit policy replaces the default for that scope, so an operator who
	// has deliberately set a limit never also gets an implicit one on top.
	configured := map[string]bool{}
	for _, p := range policies {
		if p.ConcurrencyLimit.Valid && p.ConcurrencyLimit.Int32 > 0 {
			configured[p.ScopeType] = true
		}
	}
	if r.defaultInFlight > 0 && subject.TenantID != "" {
		if !configured["tenant"] {
			if err := acquire(
				fmt.Sprintf("ratelimit:default:tenant:%s:concurrency", subject.TenantID),
				"tenant", subject.TenantID, int64(r.defaultInFlight),
			); err != nil {
				return releaseOnError(err)
			}
		}
		if subject.UserID != "" && !configured["user"] {
			if err := acquire(
				fmt.Sprintf("ratelimit:default:user:%s:concurrency", subject.UserID),
				"user", subject.UserID, int64(r.defaultInFlight),
			); err != nil {
				return releaseOnError(err)
			}
		}
	}

	for _, p := range policies {
		// Web chat must never be throttled by an API-key scope policy, even
		// if one was mis-configured with an empty scope_id.
		if p.ScopeType == "api_key" && !isAPIKeyAuth {
			continue
		}
		id := uuidString(p.ID)

		if p.ConcurrencyLimit.Valid && p.ConcurrencyLimit.Int32 > 0 {
			key := fmt.Sprintf("ratelimit:%s:concurrency", id)
			if err := acquire(key, p.ScopeType, p.ScopeID, int64(p.ConcurrencyLimit.Int32)); err != nil {
				return releaseOnError(err)
			}
		}
	}
	if len(lease.keys) == 0 {
		return nil, nil
	}
	return lease, nil
}

func concurrencyTTL(req *serving.Request) time.Duration {
	return serving.RequestLeaseTTL(req)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
