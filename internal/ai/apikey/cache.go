package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	dbgen "xiaodou/dai/internal/ai/db/gen"
)

const (
	cacheKeyPrefix = "uni-ai:apikey:v1:"
	cacheIDPrefix  = "uni-ai:apikey-id:v1:"
	cacheTTL       = 60 * time.Second
)

// cachedAPIKey mirrors GetAPIKeyByHashRow for Redis storage.
type cachedAPIKey struct {
	ID         pgtype.UUID        `json:"id"`
	OwnerType  string             `json:"ot"`
	TenantID   string             `json:"tid"`
	UserID     pgtype.Text        `json:"uid"`
	GroupID    pgtype.UUID        `json:"gid"`
	LastFour   pgtype.Text        `json:"lf"`
	Name       string             `json:"name"`
	QuotaLimit pgtype.Int8        `json:"ql"`
	QuotaUsed  int64              `json:"qu"`
	Status     string             `json:"st"`
	ExpiresAt  pgtype.Timestamptz `json:"ea"`
	LastUsedAt pgtype.Timestamptz `json:"lua"`
	CreatedBy  pgtype.Text        `json:"cb"`
	CreatedAt  pgtype.Timestamptz `json:"cat"`
	UpdatedAt  pgtype.Timestamptz `json:"uat"`
}

func rowToCache(r dbgen.GetAPIKeyByHashRow) cachedAPIKey {
	return cachedAPIKey{
		ID:         r.ID,
		OwnerType:  r.OwnerType,
		TenantID:   r.TenantID,
		UserID:     r.UserID,
		GroupID:    r.GroupID,
		LastFour:   r.LastFour,
		Name:       r.Name,
		QuotaLimit: r.QuotaLimit,
		QuotaUsed:  r.QuotaUsed,
		Status:     r.Status,
		ExpiresAt:  r.ExpiresAt,
		LastUsedAt: r.LastUsedAt,
		CreatedBy:  r.CreatedBy,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

func cacheToRow(c cachedAPIKey) dbgen.GetAPIKeyByHashRow {
	return dbgen.GetAPIKeyByHashRow{
		ID:         c.ID,
		OwnerType:  c.OwnerType,
		TenantID:   c.TenantID,
		UserID:     c.UserID,
		GroupID:    c.GroupID,
		LastFour:   c.LastFour,
		Name:       c.Name,
		QuotaLimit: c.QuotaLimit,
		QuotaUsed:  c.QuotaUsed,
		Status:     c.Status,
		ExpiresAt:  c.ExpiresAt,
		LastUsedAt: c.LastUsedAt,
		CreatedBy:  c.CreatedBy,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

// Cache is a Redis-backed cache for API key lookups.
type Cache struct {
	rdb *redis.Client
}

func NewCache(rdb *redis.Client) *Cache {
	return &Cache{rdb: rdb}
}

// Get returns the cached API key row for the given key_hash, or (_, false) on miss.
func (c *Cache) Get(ctx context.Context, keyHash string) (dbgen.GetAPIKeyByHashRow, bool) {
	data, err := c.rdb.Get(ctx, cacheKeyPrefix+keyHash).Bytes()
	if err != nil {
		return dbgen.GetAPIKeyByHashRow{}, false
	}
	var cached cachedAPIKey
	if err := json.Unmarshal(data, &cached); err != nil {
		return dbgen.GetAPIKeyByHashRow{}, false
	}
	return cacheToRow(cached), true
}

// Set writes the API key row to cache under the given key_hash with a 60s TTL.
func (c *Cache) Set(ctx context.Context, keyHash string, row dbgen.GetAPIKeyByHashRow) error {
	data, err := json.Marshal(rowToCache(row))
	if err != nil {
		return fmt.Errorf("marshal api key for cache: %w", err)
	}
	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, cacheKeyPrefix+keyHash, data, cacheTTL)
	pipe.Set(ctx, cacheIDPrefix+apiKeyUUIDString(row.ID), keyHash, cacheTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// Del removes the cache entry for the given key_hash.
// Returns an error if the DEL fails (excluding key-not-found, which is silently ignored).
func (c *Cache) Del(ctx context.Context, keyHash string) error {
	if err := c.rdb.Del(ctx, cacheKeyPrefix+keyHash).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("invalidate api key cache: %w", err)
	}
	return nil
}

// DelByID invalidates quota-bearing cache state after financial completion.
func (c *Cache) DelByID(ctx context.Context, apiKeyID string) error {
	if c == nil || c.rdb == nil || apiKeyID == "" {
		return nil
	}
	idKey := cacheIDPrefix + apiKeyID
	keyHash, err := c.rdb.Get(ctx, idKey).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve api key cache hash: %w", err)
	}
	if err := c.rdb.Del(ctx, cacheKeyPrefix+keyHash, idKey).Err(); err != nil {
		return fmt.Errorf("invalidate api key cache by id: %w", err)
	}
	return nil
}

func apiKeyUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
