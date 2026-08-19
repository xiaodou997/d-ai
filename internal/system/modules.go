package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/privacy"
)

const (
	ModuleNotification = "notification"
	ModuleProxyEgress  = "proxy_egress"
	ModulePII          = "pii_protection"
	piiConfigKey       = "module.pii_protection.config"
)

type Definition struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Category    string `json:"category"`
	AdminRoute  string `json:"adminRoute"`
	Order       int    `json:"order"`
}

var definitions = []Definition{
	{Name: ModuleNotification, DisplayName: "统一通知服务", Description: "统一管理站内通知、Webhook 发送和系统事件提醒。", Category: "integration", AdminRoute: "/admin/system-modules", Order: 10},
	{Name: ModuleProxyEgress, DisplayName: "代理出口节点", Description: "管理 AI 上游的 HTTP / SOCKS5 代理出口和健康状态。", Category: "integration", AdminRoute: "/admin/system-modules", Order: 20},
	{Name: ModulePII, DisplayName: "敏感信息保护", Description: "发送给 AI 上游前替换敏感信息，返回客户端前恢复原文。", Category: "security", AdminRoute: "/admin/system-modules/pii-protection", Order: 30},
}

var ErrUnknownModule = errors.New("unknown module")
var ErrModuleConfigInvalid = errors.New("module configuration is invalid")

type Status struct {
	Definition
	Available       bool    `json:"available"`
	Enabled         bool    `json:"enabled"`
	Active          bool    `json:"active"`
	ConfigValidated bool    `json:"configValidated"`
	ConfigError     *string `json:"configError,omitempty"`
	Health          string  `json:"health"`
}

type PIIConfig struct {
	Enabled           bool                 `json:"enabled"`
	Rules             []privacy.RuleConfig `json:"rules"`
	PlaceholderPrefix string               `json:"placeholderPrefix"`
}

type Service struct {
	pool *pgxpool.Pool

	piiMu        sync.RWMutex
	piiCacheKey  string
	piiProtector *privacy.Protector
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func Definitions() []Definition {
	return append([]Definition(nil), definitions...)
}

func DefaultPIIConfig() PIIConfig {
	config := privacy.DefaultConfig()
	return PIIConfig{Rules: config.Rules, PlaceholderPrefix: config.PlaceholderPrefix}
}

func (s *Service) List(ctx context.Context) ([]Status, error) {
	result := make([]Status, 0, len(definitions))
	for _, definition := range definitions {
		status, err := s.status(ctx, definition)
		if err != nil {
			return nil, err
		}
		result = append(result, status)
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, name string) (Status, error) {
	definition, ok := definitionByName(name)
	if !ok {
		return Status{}, ErrUnknownModule
	}
	return s.status(ctx, definition)
}

func (s *Service) SetEnabled(ctx context.Context, name string, enabled bool, actor string) (Status, error) {
	definition, ok := definitionByName(name)
	if !ok {
		return Status{}, ErrUnknownModule
	}
	if enabled {
		valid, message, err := s.validate(ctx, definition.Name)
		if err != nil {
			return Status{}, err
		}
		if !valid {
			return Status{}, fmt.Errorf("%w: %s", ErrModuleConfigInvalid, message)
		}
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO sys_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
	`, enabledKey(name), json.RawMessage(fmt.Sprintf("%t", enabled)), actor); err != nil {
		return Status{}, err
	}
	return s.status(ctx, definition)
}

func (s *Service) IsActive(ctx context.Context, name string) (bool, error) {
	status, err := s.Get(ctx, name)
	return status.Active, err
}

func (s *Service) GetPIIConfig(ctx context.Context) (PIIConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT key, value
		FROM sys_settings
		WHERE key IN ($1, $2)
	`, enabledKey(ModulePII), piiConfigKey)
	if err != nil {
		return PIIConfig{}, err
	}
	defer rows.Close()

	result := DefaultPIIConfig()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return PIIConfig{}, err
		}
		switch key {
		case enabledKey(ModulePII):
			if err := json.Unmarshal(raw, &result.Enabled); err != nil {
				return PIIConfig{}, fmt.Errorf("decode module setting %q: %w", ModulePII, err)
			}
		case piiConfigKey:
			var config privacy.Config
			if err := json.Unmarshal(raw, &config); err != nil {
				return PIIConfig{}, fmt.Errorf("%w: 无法解析敏感信息保护配置", ErrModuleConfigInvalid)
			}
			normalized, err := privacy.ValidateConfig(config)
			if err != nil {
				return PIIConfig{}, fmt.Errorf("%w: %v", ErrModuleConfigInvalid, err)
			}
			result.Rules = normalized.Rules
			result.PlaceholderPrefix = normalized.PlaceholderPrefix
		}
	}
	if err := rows.Err(); err != nil {
		return PIIConfig{}, err
	}
	return result, nil
}

func (s *Service) UpdatePIIConfig(ctx context.Context, input PIIConfig, actor string) (PIIConfig, error) {
	normalized, err := privacy.ValidateConfig(privacy.Config{Rules: input.Rules, PlaceholderPrefix: input.PlaceholderPrefix})
	if err != nil {
		return PIIConfig{}, fmt.Errorf("%w: %v", ErrModuleConfigInvalid, err)
	}
	persisted, err := json.Marshal(normalized)
	if err != nil {
		return PIIConfig{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PIIConfig{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for key, value := range map[string]json.RawMessage{
		enabledKey(ModulePII): json.RawMessage(fmt.Sprintf("%t", input.Enabled)),
		piiConfigKey:          persisted,
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO sys_settings (key, value, updated_by, updated_at)
			VALUES ($1, $2::jsonb, $3, now())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
		`, key, value, actor); err != nil {
			return PIIConfig{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return PIIConfig{}, err
	}
	s.clearPIICache()
	return PIIConfig{Enabled: input.Enabled, Rules: normalized.Rules, PlaceholderPrefix: normalized.PlaceholderPrefix}, nil
}

// PIIProtection resolves the global switch and the compiled rules together so
// the serving path sees one coherent configuration snapshot.
func (s *Service) PIIProtection(ctx context.Context) (bool, *privacy.Protector, error) {
	config, err := s.GetPIIConfig(ctx)
	if err != nil {
		return false, nil, err
	}
	if !config.Enabled {
		return false, nil, nil
	}
	privacyConfig := privacy.Config{Rules: config.Rules, PlaceholderPrefix: config.PlaceholderPrefix}
	cacheBytes, err := json.Marshal(privacyConfig)
	if err != nil {
		return false, nil, err
	}
	cacheKey := string(cacheBytes)

	s.piiMu.RLock()
	if cacheKey == s.piiCacheKey && s.piiProtector != nil {
		protector := s.piiProtector
		s.piiMu.RUnlock()
		return true, protector, nil
	}
	s.piiMu.RUnlock()

	protector, err := privacy.NewProtectorWithConfig(privacyConfig)
	if err != nil {
		return false, nil, fmt.Errorf("compile pii protection config: %w", err)
	}
	s.piiMu.Lock()
	s.piiCacheKey = cacheKey
	s.piiProtector = protector
	s.piiMu.Unlock()
	return true, protector, nil
}

func (s *Service) status(ctx context.Context, definition Definition) (Status, error) {
	enabled, err := s.readEnabled(ctx, definition.Name)
	if err != nil {
		return Status{}, err
	}
	validated, message, err := s.validate(ctx, definition.Name)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Definition:      definition,
		Available:       true,
		Enabled:         enabled,
		Active:          enabled && validated,
		ConfigValidated: validated,
		Health:          "healthy",
	}
	if !validated && message != "" {
		status.ConfigError = &message
		status.Health = "degraded"
	}
	return status, nil
}

func (s *Service) readEnabled(ctx context.Context, name string) (bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM sys_settings WHERE key = $1`, enabledKey(name)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var enabled bool
	if err := json.Unmarshal(raw, &enabled); err != nil {
		return false, fmt.Errorf("decode module setting %q: %w", name, err)
	}
	return enabled, nil
}

func (s *Service) validate(ctx context.Context, name string) (bool, string, error) {
	switch name {
	case ModuleNotification:
		return true, "", nil
	case ModulePII:
		if _, err := s.readPIIPrivacyConfig(ctx); err != nil {
			if errors.Is(err, ErrModuleConfigInvalid) {
				return false, err.Error(), nil
			}
			return false, "", err
		}
		return true, "", nil
	case ModuleProxyEgress:
		var count int
		if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_proxy_nodes WHERE status = 'active'`).Scan(&count); err != nil {
			return false, "", err
		}
		if count == 0 {
			return false, "请先配置至少一个启用的代理出口节点", nil
		}
		return true, "", nil
	default:
		return false, "", ErrUnknownModule
	}
}

func (s *Service) readPIIPrivacyConfig(ctx context.Context) (privacy.Config, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM sys_settings WHERE key = $1`, piiConfigKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return privacy.DefaultConfig(), nil
	}
	if err != nil {
		return privacy.Config{}, err
	}
	var config privacy.Config
	if err := json.Unmarshal(raw, &config); err != nil {
		return privacy.Config{}, fmt.Errorf("%w: 无法解析敏感信息保护配置", ErrModuleConfigInvalid)
	}
	normalized, err := privacy.ValidateConfig(config)
	if err != nil {
		return privacy.Config{}, fmt.Errorf("%w: %v", ErrModuleConfigInvalid, err)
	}
	return normalized, nil
}

func (s *Service) clearPIICache() {
	s.piiMu.Lock()
	s.piiCacheKey = ""
	s.piiProtector = nil
	s.piiMu.Unlock()
}

func definitionByName(name string) (Definition, bool) {
	name = strings.TrimSpace(name)
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

func enabledKey(name string) string { return "module." + name + ".enabled" }
