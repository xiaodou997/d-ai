// Package authn 提供 UniHub 跨服务共用的鉴权原语：从 URM 拉取并缓存 JWKS 公钥、
// 校验 URM 颁发的 RS256 JWT（用户令牌与 principal_type=service 的服务令牌同源），
// 并以 chi 中间件形式把身份注入请求上下文。
package authn

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/libs/go/jwks"
)

// JWKSManager 从 URM 的 JWKS 端点拉取 RSA 公钥并按 kid 缓存，支持后台定时刷新与
// 缓存未命中时的按需刷新（含刷新失败时的降级使用旧缓存）。
type JWKSManager struct {
	url             string
	refreshInterval time.Duration
	client          *http.Client
	log             *zap.Logger

	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

// Option 配置 JWKSManager。
type Option func(*JWKSManager)

// WithRefreshInterval 设置后台刷新周期（默认 24h）。
func WithRefreshInterval(d time.Duration) Option {
	return func(m *JWKSManager) {
		if d > 0 {
			m.refreshInterval = d
		}
	}
}

// WithLogger 注入结构化日志器（默认 zap.NewNop()）。
func WithLogger(l *zap.Logger) Option {
	return func(m *JWKSManager) {
		if l != nil {
			m.log = l
		}
	}
}

// WithHTTPClient 自定义 HTTP 客户端。
func WithHTTPClient(c *http.Client) Option {
	return func(m *JWKSManager) {
		if c != nil {
			m.client = c
		}
	}
}

// NewJWKSManager 创建管理器。jwksURL 为完整的 JWKS 端点 URL（如
// https://urm.example.com/public/jwks.json）。
func NewJWKSManager(jwksURL string, opts ...Option) *JWKSManager {
	m := &JWKSManager{
		url:             jwksURL,
		refreshInterval: 24 * time.Hour,
		client:          &http.Client{Timeout: 30 * time.Second},
		log:             zap.NewNop(),
		keys:            make(map[string]*rsa.PublicKey),
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Start 立即拉取一次公钥（失败不阻塞启动，仅告警），并在后台按周期刷新，直到 ctx
// 取消。
func (m *JWKSManager) Start(ctx context.Context) {
	if err := m.Refresh(ctx); err != nil {
		m.log.Warn("初始拉取 JWKS 失败，将以空缓存启动（JWT 校验在公钥就绪前会失败）", zap.Error(err))
	}
	go m.refreshLoop(ctx)
}

func (m *JWKSManager) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(m.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Refresh(ctx); err != nil {
				m.log.Warn("定时刷新 JWKS 失败", zap.Error(err))
			}
		}
	}
}

// Refresh 从端点拉取最新 JWKS 并整体替换缓存（仅保留 RSA/RS256 公钥）。
func (m *JWKSManager) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.url, nil)
	if err != nil {
		return fmt.Errorf("build jwks request: %w", err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks endpoint returned %d", resp.StatusCode)
	}

	var set jwks.Set
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		pub, err := jwks.ParseRSAPublicKey(k)
		if err != nil {
			m.log.Warn("解析 JWK 失败，跳过", zap.String("kid", k.Kid), zap.Error(err))
			continue
		}
		newKeys[k.Kid] = pub
	}

	m.mu.Lock()
	m.keys = newKeys
	m.mu.Unlock()
	m.log.Debug("JWKS 已刷新", zap.Int("keys", len(newKeys)))
	return nil
}

// GetPublicKey 返回指定 kid 的公钥；缓存未命中时尝试刷新一次，刷新失败仍返回旧缓存
// （若有）。
func (m *JWKSManager) GetPublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	m.mu.RLock()
	pub, ok := m.keys[kid]
	m.mu.RUnlock()
	if ok {
		return pub, nil
	}

	if err := m.Refresh(ctx); err != nil {
		m.mu.RLock()
		pub, ok = m.keys[kid]
		m.mu.RUnlock()
		if ok {
			m.log.Warn("JWKS 刷新失败，降级使用旧缓存公钥", zap.String("kid", kid), zap.Error(err))
			return pub, nil
		}
		return nil, fmt.Errorf("refresh jwks for kid %s: %w", kid, err)
	}

	m.mu.RLock()
	pub, ok = m.keys[kid]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown kid: %s", kid)
	}
	return pub, nil
}
