package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// LoginRateDimensions are deliberately hashed before entering Redis. This keeps
// usernames, email addresses and IP addresses out of key listings and metrics.
type LoginRateDimensions struct {
	Account string
	IP      string
	Tenant  string
}

type RateLimitDecision struct {
	Allowed    bool
	RetryAfter time.Duration
}

type LoginRateLimiter struct {
	redis  *redis.Client
	prefix string
}

const (
	rateLimitFailureWindow = 15 * time.Minute
	rateLimitThreshold     = 5
	rateLimitBaseBackoff   = 5 * time.Second
	rateLimitMaxBackoff    = 15 * time.Minute
)

var rateLimitCheckScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local retry = 0
for _, key in ipairs(KEYS) do
  local blocked_until = redis.call('GET', key .. ':blocked')
  if blocked_until then
    local remaining = tonumber(blocked_until) - now
    if remaining > retry then retry = remaining end
  end
end
return retry
`)

var rateLimitFailureScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])
local base = tonumber(ARGV[4])
local max_backoff = tonumber(ARGV[5])
local retry = 0
for _, key in ipairs(KEYS) do
  local count = redis.call('INCR', key .. ':fail')
  redis.call('EXPIRE', key .. ':fail', window)
  if count >= threshold then
    local exponent = count - threshold
    local seconds = base * (2 ^ exponent)
    if seconds > max_backoff then seconds = max_backoff end
    local blocked_until = now + seconds
    redis.call('SET', key .. ':blocked', blocked_until, 'EX', math.ceil(seconds))
    if seconds > retry then retry = seconds end
  end
end
return retry
`)

func NewLoginRateLimiter(redisClient *redis.Client) *LoginRateLimiter {
	return NewScopedRateLimiter(redisClient, "dai:auth:login:")
}

func NewScopedRateLimiter(redisClient *redis.Client, prefix string) *LoginRateLimiter {
	return &LoginRateLimiter{redis: redisClient, prefix: prefix}
}

func (l *LoginRateLimiter) Check(ctx context.Context, dimensions LoginRateDimensions) (RateLimitDecision, error) {
	keys := l.keys(dimensions)
	if l.redis == nil {
		return RateLimitDecision{}, fmt.Errorf("login rate limiter redis is unavailable")
	}
	result, err := rateLimitCheckScript.Run(ctx, l.redis, keys, time.Now().Unix()).Int64()
	if err != nil {
		return RateLimitDecision{}, fmt.Errorf("check login rate limit: %w", err)
	}
	if result <= 0 {
		return RateLimitDecision{Allowed: true}, nil
	}
	return RateLimitDecision{RetryAfter: time.Duration(result) * time.Second}, nil
}

func (l *LoginRateLimiter) RecordFailure(ctx context.Context, dimensions LoginRateDimensions) (time.Duration, error) {
	keys := l.keys(dimensions)
	if l.redis == nil {
		return 0, fmt.Errorf("login rate limiter redis is unavailable")
	}
	result, err := rateLimitFailureScript.Run(ctx, l.redis, keys,
		time.Now().Unix(), int(rateLimitFailureWindow/time.Second), rateLimitThreshold,
		int(rateLimitBaseBackoff/time.Second), int(rateLimitMaxBackoff/time.Second),
	).Int64()
	if err != nil {
		return 0, fmt.Errorf("record login failure: %w", err)
	}
	return time.Duration(result) * time.Second, nil
}

func (l *LoginRateLimiter) Reset(ctx context.Context, dimensions LoginRateDimensions) error {
	if l.redis == nil {
		return fmt.Errorf("login rate limiter redis is unavailable")
	}
	keys := l.keys(dimensions)
	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		args = append(args, key+":fail", key+":blocked")
	}
	return l.redis.Del(ctx, args...).Err()
}

func (l *LoginRateLimiter) keys(dimensions LoginRateDimensions) []string {
	values := []string{
		"account:" + dimensions.Account,
		"ip:" + dimensions.IP,
		"account-ip:" + dimensions.Account + "|" + dimensions.IP,
	}
	if strings.TrimSpace(dimensions.Tenant) != "" {
		values = append(values, "tenant-ip:"+dimensions.Tenant+"|"+dimensions.IP)
	}
	keys := make([]string, 0, len(values))
	for _, value := range values {
		sum := sha256.Sum256([]byte(value))
		keys = append(keys, l.prefix+hex.EncodeToString(sum[:]))
	}
	return keys
}

func RetryAfterSeconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return int64(math.Ceil(duration.Seconds()))
}
