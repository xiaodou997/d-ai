package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ModuleNotification = "notification"
	ModuleProxyEgress  = "proxy_egress"
	ModulePII          = "pii_protection"
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
	{Name: ModuleNotification, DisplayName: "统一通知服务", Description: "统一管理站内通知、Webhook 发送和系统事件提醒。", Category: "integration", AdminRoute: "/admin/system-modules/notification", Order: 10},
	{Name: ModuleProxyEgress, DisplayName: "代理出口节点", Description: "管理 AI 上游的 HTTP / SOCKS5 代理出口和健康状态。", Category: "integration", AdminRoute: "/admin/system-modules/proxy-egress", Order: 20},
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

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func Definitions() []Definition {
	return append([]Definition(nil), definitions...)
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
	case ModuleNotification, ModulePII:
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
