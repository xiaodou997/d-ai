package urm

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	channelUserBanned  = "urc:user:banned"
	channelUserUpdated = "urc:user:updated"
	banCacheTTL        = 24 * time.Hour
)

// BanSubscriber 订阅 URM Pub/Sub 频道，维护本地封禁用户缓存
type BanSubscriber struct {
	rdb    *redis.Client
	logger *slog.Logger

	mu     sync.RWMutex
	banned map[string]time.Time // userID → 过期时间
}

func NewBanSubscriber(rdb *redis.Client, logger *slog.Logger) *BanSubscriber {
	if logger == nil {
		logger = slog.Default()
	}
	return &BanSubscriber{
		rdb:    rdb,
		logger: logger,
		banned: make(map[string]time.Time),
	}
}

// Start 在后台 goroutine 中持续订阅，ctx 取消时退出
func (s *BanSubscriber) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *BanSubscriber) run(ctx context.Context) {
	pubsub := s.rdb.Subscribe(ctx, channelUserBanned, channelUserUpdated)
	defer pubsub.Close()

	s.logger.Info("ban subscriber started",
		"channels", []string{channelUserBanned, channelUserUpdated})

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("ban subscriber stopped")
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			s.handleMessage(msg)
		}
	}
}

func (s *BanSubscriber) handleMessage(msg *redis.Message) {
	var payload struct {
		UserID string `json:"userId"`
		Event  string `json:"event"`
	}
	if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil || payload.UserID == "" {
		return
	}

	switch msg.Channel {
	case channelUserBanned:
		s.mu.Lock()
		s.banned[payload.UserID] = time.Now().Add(banCacheTTL)
		s.mu.Unlock()
		s.logger.Info("user banned, added to local cache", "userId", payload.UserID)
	case channelUserUpdated:
		s.mu.Lock()
		delete(s.banned, payload.UserID)
		s.mu.Unlock()
	}
}

// IsBanned 检查用户是否在本地封禁缓存中
func (s *BanSubscriber) IsBanned(userID string) bool {
	s.mu.RLock()
	exp, ok := s.banned[userID]
	s.mu.RUnlock()

	if !ok {
		return false
	}
	if time.Now().After(exp) {
		s.mu.Lock()
		delete(s.banned, userID)
		s.mu.Unlock()
		return false
	}
	return true
}
