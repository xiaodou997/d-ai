package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "urm:session:"

// SessionData SSO Session 中存储的用户信息
type SessionData struct {
	UserID          string `json:"userId"`
	Username        string `json:"username"`
	UserType        int    `json:"userType"`
	UserTypeDisplay string `json:"userTypeDisplay"`
	TenantID        string `json:"tenantId"`
	ClientType      string `json:"clientType,omitempty"`
	ClientID        string `json:"clientId,omitempty"`
	CreatedAt       int64  `json:"createdAt"`
	LastAccessAt    int64  `json:"lastAccessAt"`
}

// SessionService 管理 SSO Session（Redis 存储）
type SessionService struct {
	redis         *redis.Client
	sessionTTL    time.Duration
	sessionMaxTTL time.Duration
}

func NewSessionService(redisClient *redis.Client, sessionTTL, sessionMaxTTL time.Duration) *SessionService {
	if sessionTTL <= 0 {
		sessionTTL = 7 * 24 * time.Hour
	}
	if sessionMaxTTL <= 0 {
		sessionMaxTTL = 30 * 24 * time.Hour
	}
	return &SessionService{
		redis:         redisClient,
		sessionTTL:    sessionTTL,
		sessionMaxTTL: sessionMaxTTL,
	}
}

func (s *SessionService) IsEnabled() bool {
	return s.redis != nil
}

// CreateSession 创建 Session，返回 session ID (UUID)
func (s *SessionService) CreateSession(ctx context.Context, data SessionData) (string, error) {
	if s.redis == nil {
		return "", fmt.Errorf("session service not available (Redis required)")
	}
	now := time.Now().UnixMilli()
	data.CreatedAt = now
	data.LastAccessAt = now

	b, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}

	sessionID := uuid.New().String()
	key := sessionKeyPrefix + sessionID
	if err := s.redis.Set(ctx, key, string(b), s.sessionTTL).Err(); err != nil {
		return "", fmt.Errorf("store session: %w", err)
	}
	return sessionID, nil
}

// GetSession 读取 Session 并滚动刷新 TTL
// 如果 session 已超过 sessionMaxTTL 则视为过期删除
func (s *SessionService) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("session service not available")
	}
	key := sessionKeyPrefix + sessionID
	val, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var data SessionData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}

	// 检查绝对最大 TTL
	if time.Since(time.UnixMilli(data.CreatedAt)) > s.sessionMaxTTL {
		_ = s.redis.Del(ctx, key)
		return nil, nil
	}

	// 滚动刷新活跃窗口
	data.LastAccessAt = time.Now().UnixMilli()
	b, _ := json.Marshal(data)
	_ = s.redis.Set(ctx, key, string(b), s.sessionTTL)

	return &data, nil
}

// DeleteSession 删除 Session
func (s *SessionService) DeleteSession(ctx context.Context, sessionID string) error {
	if s.redis == nil {
		return nil
	}
	return s.redis.Del(ctx, sessionKeyPrefix+sessionID).Err()
}
