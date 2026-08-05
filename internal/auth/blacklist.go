package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// BlacklistService is URM's canonical, single writer for two related but
// distinct pieces of Redis-backed auth state:
//   - JWT token blacklist / user logout markers (AddToBlacklist, LogoutUser)
//   - Ban state for user/tenant accounts (BanUser, BanTenant, ...), read
//     directly by the AI gateway —
//     no pub/sub involved. The Redis key IS the source of truth: a service
//     restart or a Redis reconnect never loses state, and every replica of
//     every consuming service always sees the same answer.
type BlacklistService struct {
	redis   *redis.Client
	enabled bool
	logger  *zap.Logger
}

const (
	// 黑名单 Key 前缀
	blacklistPrefix = "token:blacklist:"
	// 默认 Token 过期时间（用于黑名单 TTL）
	defaultTokenTTL = 24 * time.Hour
	// 封禁状态 Key 前缀（无 TTL，显式 DEL 才会清除）。AI 网关直接 EXISTS
	// 查询这两个前缀，不再通过 pub/sub 通知。
	banUserPrefix   = "urc:banned:user:"
	banTenantPrefix = "urc:banned:tenant:"
)

// NewBlacklistService 创建黑名单服务
func NewBlacklistService(redisClient *redis.Client, logger *zap.Logger) *BlacklistService {
	enabled := redisClient != nil
	if !enabled {
		logger.Warn("Redis not available, token blacklist disabled")
	}

	return &BlacklistService{
		redis:   redisClient,
		enabled: enabled,
		logger:  logger,
	}
}

// AddToBlacklist 将 Token 加入黑名单
// tokenID: Token 的唯一标识（jti claim）
// expiration: Token 剩余有效期
func (s *BlacklistService) AddToBlacklist(tokenID string, expiration time.Duration) error {
	if !s.enabled {
		s.logger.Debug("Blacklist disabled, skip adding token", zap.String("tokenId", tokenID))
		return nil
	}

	if expiration <= 0 {
		expiration = defaultTokenTTL
	}

	key := blacklistPrefix + tokenID
	ctx := context.Background()

	err := s.redis.Set(ctx, key, "1", expiration).Err()
	if err != nil {
		s.logger.Error("Failed to add token to blacklist", zap.String("tokenId", tokenID), zap.Error(err))
		return err
	}

	s.logger.Debug("Token added to blacklist", zap.String("tokenId", tokenID), zap.Duration("ttl", expiration))
	return nil
}

// IsBlacklisted 检查 Token 是否在黑名单中
func (s *BlacklistService) IsBlacklisted(tokenID string) bool {
	if !s.enabled {
		return false
	}

	key := blacklistPrefix + tokenID
	ctx := context.Background()

	exists, err := s.redis.Exists(ctx, key).Result()
	if err != nil {
		s.logger.Error("Failed to check blacklist", zap.String("tokenId", tokenID), zap.Error(err))
		return false
	}

	return exists > 0
}

// LogoutUser 登出用户（将用户所有 Token 加入黑名单）
// 通过记录登出时间实现，所有在此时间之前签发的 Token 都视为无效
func (s *BlacklistService) LogoutUser(userID string) error {
	if !s.enabled {
		s.logger.Debug("Blacklist disabled, skip user logout", zap.String("userId", userID))
		return nil
	}

	key := "user:logout:" + userID
	ctx := context.Background()

	// 记录登出时间
	err := s.redis.Set(ctx, key, time.Now().Unix(), defaultTokenTTL).Err()
	if err != nil {
		s.logger.Error("Failed to logout user", zap.String("userId", userID), zap.Error(err))
		return err
	}

	s.logger.Info("User logged out", zap.String("userId", userID))
	return nil
}

// GetUserLogoutTime 获取用户登出时间
// 返回 0 表示用户未登出
func (s *BlacklistService) GetUserLogoutTime(userID string) int64 {
	if !s.enabled {
		return 0
	}

	key := "user:logout:" + userID
	ctx := context.Background()

	result, err := s.redis.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}

	return result
}

// IsEnabled 检查黑名单服务是否启用
func (s *BlacklistService) IsEnabled() bool {
	return s.enabled
}

// ============================================================================
// Ban state — single source of truth, read directly by the AI gateway (no
// pub/sub, no local cache to go stale on restart).
// ============================================================================

// BanUser marks a user as banned (Redis key, no TTL) and kills its existing
// JWT sessions via LogoutUser. Idempotent.
func (s *BlacklistService) BanUser(ctx context.Context, userID string) error {
	if err := s.LogoutUser(userID); err != nil {
		return err
	}
	if !s.enabled {
		return nil
	}
	if err := s.redis.Set(ctx, banUserPrefix+userID, "1", 0).Err(); err != nil {
		s.logger.Warn("Failed to set user ban key", zap.String("userId", userID), zap.Error(err))
		return err
	}
	return nil
}

// UnbanUser clears a user's ban state.
func (s *BlacklistService) UnbanUser(ctx context.Context, userID string) error {
	if !s.enabled {
		return nil
	}
	if err := s.redis.Del(ctx, banUserPrefix+userID).Err(); err != nil {
		s.logger.Warn("Failed to clear user ban key", zap.String("userId", userID), zap.Error(err))
		return err
	}
	return nil
}

// BanTenant marks a tenant as banned (Redis key, no TTL). Downstream services
// check this in addition to per-user bans, since a tenant-wide disable does
// not iterate and ban every individual user under it — the tenant key alone
// is sufficient and far cheaper.
func (s *BlacklistService) BanTenant(ctx context.Context, tenantID string) error {
	if !s.enabled {
		return nil
	}
	if err := s.redis.Set(ctx, banTenantPrefix+tenantID, "1", 0).Err(); err != nil {
		s.logger.Warn("Failed to set tenant ban key", zap.String("tenantId", tenantID), zap.Error(err))
		return err
	}
	return nil
}

// UnbanTenant clears a tenant's ban state.
func (s *BlacklistService) UnbanTenant(ctx context.Context, tenantID string) error {
	if !s.enabled {
		return nil
	}
	if err := s.redis.Del(ctx, banTenantPrefix+tenantID).Err(); err != nil {
		s.logger.Warn("Failed to clear tenant ban key", zap.String("tenantId", tenantID), zap.Error(err))
		return err
	}
	return nil
}
