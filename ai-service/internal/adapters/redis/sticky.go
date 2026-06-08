// Package redis provides Redis-backed routing store implementations.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"xiaodou/unihub/ai-service/internal/routing"
)

const (
	stickyKeyPrefix = "uni_ai_api:conv"
	stickyTTL       = 24 * time.Hour
)

// stickyJSON is the on-wire JSON shape for a sticky binding stored in Redis.
type stickyJSON struct {
	TargetKind   string `json:"target_kind"`
	DeploymentID string `json:"deployment_id,omitempty"`
	EndpointID   string `json:"endpoint_id,omitempty"`
	CredentialID string `json:"credential_id,omitempty"`
	RouteID      string `json:"route_id,omitempty"`
}

// RedisSticky implements routing.StickyStore using Redis.
type RedisSticky struct {
	rdb *redis.Client
}

// NewRedisSticky creates a RedisSticky backed by rdb.
func NewRedisSticky(rdb *redis.Client) *RedisSticky {
	return &RedisSticky{rdb: rdb}
}

func (s *RedisSticky) key(tenantID, identity, model, convID string) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", stickyKeyPrefix, tenantID, identity, model, convID)
}

// GetBinding returns the stored binding, or (nil, nil) when not found.
func (s *RedisSticky) GetBinding(ctx context.Context, tenantID, identity, model, convID string) (*routing.StickyBinding, error) {
	raw, err := s.rdb.Get(ctx, s.key(tenantID, identity, model, convID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j stickyJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	return &routing.StickyBinding{
		TargetKind:   j.TargetKind,
		DeploymentID: j.DeploymentID,
		EndpointID:   j.EndpointID,
		CredentialID: j.CredentialID,
		RouteID:      j.RouteID,
	}, nil
}

// SetBinding persists a binding for stickyTTL (24 h).
func (s *RedisSticky) SetBinding(ctx context.Context, tenantID, identity, model, convID string, b *routing.StickyBinding) error {
	j := stickyJSON{
		TargetKind:   b.TargetKind,
		DeploymentID: b.DeploymentID,
		EndpointID:   b.EndpointID,
		CredentialID: b.CredentialID,
		RouteID:      b.RouteID,
	}
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.key(tenantID, identity, model, convID), raw, stickyTTL).Err()
}

// DeleteBinding removes the binding for a conversation.
func (s *RedisSticky) DeleteBinding(ctx context.Context, tenantID, identity, model, convID string) error {
	return s.rdb.Del(ctx, s.key(tenantID, identity, model, convID)).Err()
}
