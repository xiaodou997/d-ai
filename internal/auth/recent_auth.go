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

func (s *RecentAuthService) Mark(ctx context.Context, userID, sessionID, method string) error {
	if s == nil || s.redis == nil {
		return fmt.Errorf("recent authentication redis is unavailable")
	}
	if userID == "" || sessionID == "" {
		return fmt.Errorf("recent authentication session is missing")
	}
	return s.redis.Set(ctx, recentAuthKey(userID, sessionID), method, s.ttl).Err()
}

func (s *RecentAuthService) Check(ctx context.Context, userID, sessionID string) (bool, error) {
	if s == nil || s.redis == nil {
		return false, fmt.Errorf("recent authentication redis is unavailable")
	}
	if userID == "" || sessionID == "" {
		return false, fmt.Errorf("recent authentication session is missing")
	}
	value, err := s.redis.Exists(ctx, recentAuthKey(userID, sessionID)).Result()
	return value > 0, err
}

func recentAuthKey(userID, sessionID string) string {
	return "dai:auth:recent:" + userID + ":" + sessionID
}
