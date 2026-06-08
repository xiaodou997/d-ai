package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
)

const (
	cacheKeyPrefix = "uni-ai:apikey:v1:"
	cacheTTL       = 60 * time.Second
)

// cachedAPIKey mirrors GetAPIKeyByHashRow with AllowedModels as json.RawMessage
// to prevent []byte → base64 encoding when stored in Redis.
type cachedAPIKey struct {
	ID            pgtype.UUID        `json:"id"`
	OwnerType     string             `json:"ot"`
	TenantID      string             `json:"tid"`
	UserID        pgtype.Text        `json:"uid"`
	LastFour      pgtype.Text        `json:"lf"`
	Name          string             `json:"name"`
	QuotaLimit    pgtype.Int8        `json:"ql"`
	QuotaUsed     int64              `json:"qu"`
	QuotaReserved int64              `json:"qr"`
	AllowedModels json.RawMessage    `json:"am"`
	Status        string             `json:"st"`
	ExpiresAt     pgtype.Timestamptz `json:"ea"`
	LastUsedAt    pgtype.Timestamptz `json:"lua"`
	CreatedBy     pgtype.Text        `json:"cb"`
	CreatedAt     pgtype.Timestamptz `json:"cat"`
	UpdatedAt     pgtype.Timestamptz `json:"uat"`
}

func rowToCache(r dbgen.GetAPIKeyByHashRow) cachedAPIKey {
	return cachedAPIKey{
		ID:            r.ID,
		OwnerType:     r.OwnerType,
		TenantID:      r.TenantID,
		UserID:        r.UserID,
		LastFour:      r.LastFour,
		Name:          r.Name,
		QuotaLimit:    r.QuotaLimit,
		QuotaUsed:     r.QuotaUsed,
		QuotaReserved: r.QuotaReserved,
		AllowedModels: json.RawMessage(r.AllowedModels),
		Status:        r.Status,
		ExpiresAt:     r.ExpiresAt,
		LastUsedAt:    r.LastUsedAt,
		CreatedBy:     r.CreatedBy,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func cacheToRow(c cachedAPIKey) dbgen.GetAPIKeyByHashRow {
	return dbgen.GetAPIKeyByHashRow{
		ID:            c.ID,
		OwnerType:     c.OwnerType,
		TenantID:      c.TenantID,
		UserID:        c.UserID,
		LastFour:      c.LastFour,
		Name:          c.Name,
		QuotaLimit:    c.QuotaLimit,
		QuotaUsed:     c.QuotaUsed,
		QuotaReserved: c.QuotaReserved,
		AllowedModels: []byte(c.AllowedModels),
		Status:        c.Status,
		ExpiresAt:     c.ExpiresAt,
		LastUsedAt:    c.LastUsedAt,
		CreatedBy:     c.CreatedBy,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
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
	return c.rdb.Set(ctx, cacheKeyPrefix+keyHash, data, cacheTTL).Err()
}

// Del removes the cache entry for the given key_hash.
// Returns an error if the DEL fails (excluding key-not-found, which is silently ignored).
func (c *Cache) Del(ctx context.Context, keyHash string) error {
	if err := c.rdb.Del(ctx, cacheKeyPrefix+keyHash).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("invalidate api key cache: %w", err)
	}
	return nil
}
