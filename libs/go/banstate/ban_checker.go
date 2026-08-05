// Package banstate reads user/tenant ban state directly from the Redis keys
// urm-service writes (SET on ban, DEL on unban — no TTL, no pub/sub). Redis
// itself is the single source of truth shared by every consuming service and
// every replica, so there is no local cache to go stale on restart or drop
// an event: a plain Redis GET is already sub-millisecond and cheap enough to
// do on every request. Shared by every service that authenticates requests
// (ai-service, proxy-service) so the key format only needs to change in one
// place.
package banstate

import (
	"context"

	"github.com/redis/go-redis/v9"
)

const (
	banUserKeyPrefix   = "urc:banned:user:"
	banTenantKeyPrefix = "urc:banned:tenant:"
)

// Checker reads ban state from Redis.
type Checker struct {
	rdb *redis.Client
}

// NewChecker constructs a checker. rdb may be nil, in which case every check
// reports "not banned" (matches how other Redis-optional features degrade,
// e.g. an API-key or balance cache backed by the same Redis instance).
func NewChecker(rdb *redis.Client) *Checker {
	return &Checker{rdb: rdb}
}

// IsBanned reports whether userID is currently banned. Redis errors are
// returned to the caller so auth layers can fail closed instead of
// accidentally allowing banned users through during a transient outage.
func (c *Checker) IsBanned(ctx context.Context, userID string) (bool, error) {
	if c == nil || c.rdb == nil || userID == "" {
		return false, nil
	}
	n, err := c.rdb.Exists(ctx, banUserKeyPrefix+userID).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// IsTenantBanned reports whether tenantID is currently banned. Redis errors
// are returned to the caller so auth layers can fail closed.
func (c *Checker) IsTenantBanned(ctx context.Context, tenantID string) (bool, error) {
	if c == nil || c.rdb == nil || tenantID == "" {
		return false, nil
	}
	n, err := c.rdb.Exists(ctx, banTenantKeyPrefix+tenantID).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
