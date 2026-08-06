package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ==================== Redis 键命名规范 ====================
// 格式：dai:{type}:{id}
// 统一前缀便于监控和管理，避免键名冲突
const (
	KeyPrefix     = "dai:"                    // 系统前缀
	KeyNonce      = KeyPrefix + "nonce:"      // Nonce 防重放
	KeyIdempotent = KeyPrefix + "idempotent:" // 幂等性缓存
	KeyUser       = KeyPrefix + "user:"       // 用户缓存
	KeyLogoutTime = KeyPrefix + "logout:"     // 登出时间戳
)

// 键 TTL 配置（生产环境可根据实际情况调整）
const (
	TTLNonce      = 10 * time.Minute // Nonce 存储 10 分钟（防重放窗口）
	TTLIdempotent = 1 * time.Hour    // 幂等性结果缓存 1 小时
	TTLUser       = 1 * time.Hour    // 用户缓存 1 小时
)

// RedisService Redis 缓存服务
type RedisService struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisService 创建 Redis 服务
func NewRedisService(addr, password string, db int, logger *zap.Logger) (*RedisService, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect redis: %w", err)
	}

	return &RedisService{client: client, logger: logger}, nil
}

// SetIdempotentKey 设置幂等键（24小时过期）
func (s *RedisService) SetIdempotentKey(requestID string, result interface{}) error {
	ctx := context.Background()
	key := fmt.Sprintf("idempotent:deduct:%s", requestID)

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, key, data, 24*time.Hour).Err()
}

// GetIdempotentKey 获取幂等键
func (s *RedisService) GetIdempotentKey(requestID string, result interface{}) (bool, error) {
	ctx := context.Background()
	key := KeyIdempotent + requestID

	data, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal([]byte(data), result); err != nil {
		return false, err
	}

	return true, nil
}

// SetNonce 防重放 Nonce（5分钟过期）
func (s *RedisService) SetNonce(nonce string) error {
	ctx := context.Background()
	key := KeyNonce + nonce
	return s.client.Set(ctx, key, "1", 5*time.Minute).Err()
}

// ExistsNonce 检查 Nonce 是否存在
func (s *RedisService) ExistsNonce(nonce string) (bool, error) {
	ctx := context.Background()
	key := KeyNonce + nonce
	result, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// Close 关闭连接
func (s *RedisService) Close() error {
	return s.client.Close()
}

// IsAvailable 检查 Redis 是否可用
func (s *RedisService) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	return s.client.Ping(ctx).Err() == nil
}

// GetClient 获取 Redis 客户端
func (s *RedisService) GetClient() *redis.Client {
	return s.client
}

// Publish 发布消息（用于 Pub/Sub）
func (s *RedisService) Publish(ctx context.Context, channel string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, channel, data).Err()
}

// ==================== 登录限流相关方法 ====================

// IncrementLoginFailCount 增加登录失败次数
// 参数:
//   - username: 用户名
//   - expiration: 过期时间（通常 15 分钟）
//
// 返回:
//   - int64: 当前失败次数
//   - error: 错误信息
func (s *RedisService) IncrementLoginFailCount(username string, expiration time.Duration) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("login:failed:%s", username)

	count, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// 如果是第一次失败，设置过期时间
	if count == 1 {
		s.client.Expire(ctx, key, expiration)
	}

	return count, nil
}

// GetLoginFailCount 获取登录失败次数
func (s *RedisService) GetLoginFailCount(username string) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("login:failed:%s", username)
	return s.client.Get(ctx, key).Int64()
}

// ClearLoginFailCount 清除登录失败记录（登录成功后调用）
func (s *RedisService) ClearLoginFailCount(username string) error {
	ctx := context.Background()
	key := fmt.Sprintf("login:failed:%s", username)
	return s.client.Del(ctx, key).Err()
}

// SetUserLock 锁定用户（失败次数过多时调用）
// 参数:
//   - username: 用户名
//   - lockDuration: 锁定时长（通常 30 分钟）
func (s *RedisService) SetUserLock(username string, lockDuration time.Duration) error {
	ctx := context.Background()
	key := fmt.Sprintf("login:locked:%s", username)
	return s.client.Set(ctx, key, time.Now().Unix(), lockDuration).Err()
}

// IsUserLocked 检查用户是否被锁定
func (s *RedisService) IsUserLocked(username string) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("login:locked:%s", username)
	_, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// IncrementIPRateLimit 增加 IP 请求次数
// 参数:
//   - ip: IP 地址
//   - expiration: 过期时间（通常 1 分钟）
//
// 返回:
//   - int64: 当前请求次数
//   - error: 错误信息
func (s *RedisService) IncrementIPRateLimit(ip string, expiration time.Duration) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("login:ip:%s", ip)

	count, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// 如果是第一次请求，设置过期时间
	if count == 1 {
		s.client.Expire(ctx, key, expiration)
	}

	return count, nil
}

// GetIPRateLimit 获取 IP 请求次数
func (s *RedisService) GetIPRateLimit(ip string) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("login:ip:%s", ip)
	return s.client.Get(ctx, key).Int64()
}
