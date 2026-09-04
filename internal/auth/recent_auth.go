package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const recentAuthTTL = 30 * time.Minute

type RecentAuthService struct {
	redis *redis.Client
	ttl   time.Duration
}

func NewRecentAuthService(redisClient *redis.Client) *RecentAuthService {
	return &RecentAuthService{redis: redisClient, ttl: recentAuthTTL}
}

func (s *RecentAuthService) Mark(ctx context.Context, userID, method string) error {
	if s == nil || s.redis == nil {
		return fmt.Errorf("recent authentication redis is unavailable")
	}
	return s.redis.Set(ctx, "dai:auth:recent:"+userID, method, s.ttl).Err()
}

func (s *RecentAuthService) Check(ctx context.Context, userID string) (bool, error) {
	if s == nil || s.redis == nil {
		return false, fmt.Errorf("recent authentication redis is unavailable")
	}
	value, err := s.redis.Exists(ctx, "dai:auth:recent:"+userID).Result()
	return value > 0, err
}
