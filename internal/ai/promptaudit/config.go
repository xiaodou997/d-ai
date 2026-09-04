package promptaudit

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const configCacheTTL = 10 * time.Second

type SettingRepository interface {
	GetSetting(context.Context, string) (json.RawMessage, error)
	UpsertSetting(context.Context, string, json.RawMessage) error
}

type ConfigService struct {
	repo     SettingRepository
	mu       sync.RWMutex
	cached   *Config
	cachedAt time.Time
}

func NewConfigService(repo SettingRepository) *ConfigService { return &ConfigService{repo: repo} }

func (s *ConfigService) Get(ctx context.Context) (Config, error) {
	if s == nil || s.repo == nil {
		return DefaultConfig(), nil
	}
	s.mu.RLock()
	if s.cached != nil && time.Since(s.cachedAt) < configCacheTTL {
		cfg := cloneConfig(*s.cached)
		s.mu.RUnlock()
		return cfg, nil
	}
	s.mu.RUnlock()
	raw, err := s.repo.GetSetting(ctx, SettingKey)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return DefaultConfig(), nil
		}
		return s.cachedOrDefault(), err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return s.cachedOrDefault(), err
	}
	normalizeConfig(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return s.cachedOrDefault(), err
	}
	s.mu.Lock()
	s.cached = &cfg
	s.cachedAt = time.Now()
	s.mu.Unlock()
	return cloneConfig(cfg), nil
}

func (s *ConfigService) cachedOrDefault() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cached != nil {
		return cloneConfig(*s.cached)
	}
	return DefaultConfig()
}

func (s *ConfigService) Update(ctx context.Context, cfg Config) error {
	if s == nil || s.repo == nil {
		return errors.New("prompt audit setting repository unavailable")
	}
	normalizeConfig(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	current, _ := s.Get(ctx)
	cfg.ConfigRevision = current.ConfigRevision + 1
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := s.repo.UpsertSetting(ctx, SettingKey, raw); err != nil {
		return err
	}
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
	return nil
}

func normalizeConfig(cfg *Config) {
	if cfg.Mode == "" {
		cfg.Mode = ModeOff
	}
	if cfg.WorkerCount == 0 {
		cfg.WorkerCount = 4
	}
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = 4096
	}
	if cfg.ConfigRevision < 1 {
		cfg.ConfigRevision = 1
	}
	if len(cfg.Scanners) == 0 {
		cfg.Scanners = append([]string(nil), ScannerIDs...)
	}
	known := make(map[string]struct{}, len(ScannerIDs))
	for _, id := range ScannerIDs {
		known[id] = struct{}{}
	}
	seen := map[string]struct{}{}
	scanners := make([]string, 0, len(cfg.Scanners))
	for _, raw := range cfg.Scanners {
		id := NormalizeCategory(raw)
		if _, ok := known[id]; ok {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				scanners = append(scanners, id)
			}
		}
	}
	cfg.Scanners = scanners
	for i := range cfg.Endpoints {
		ep := &cfg.Endpoints[i]
		ep.ID = strings.TrimSpace(ep.ID)
		ep.Name = strings.TrimSpace(ep.Name)
		ep.BaseURL = strings.TrimSpace(ep.BaseURL)
		ep.Model = strings.TrimSpace(ep.Model)
		if ep.Model == "" {
			ep.Model = DefaultGuardModel
		}
		if ep.TimeoutMS == 0 {
			ep.TimeoutMS = 3000
		}
		if ep.InputLimit == 0 {
			ep.InputLimit = 4000
		}
	}
	for i := range cfg.TenantIDs {
		cfg.TenantIDs[i] = strings.TrimSpace(cfg.TenantIDs[i])
	}
	sort.Strings(cfg.TenantIDs)
}

func ValidateConfig(cfg Config) error {
	switch cfg.Mode {
	case ModeOff, ModeObserve, ModeBlocking:
	default:
		return errors.New("prompt audit mode must be off, observe or blocking")
	}
	if cfg.Enabled && cfg.Mode == ModeOff {
		return errors.New("enabled prompt audit requires observe or blocking mode")
	}
	if !cfg.Enabled && cfg.Mode != ModeOff {
		return errors.New("disabled prompt audit must use off mode")
	}
	if cfg.WorkerCount < 1 || cfg.WorkerCount > 32 {
		return errors.New("prompt audit worker_count must be between 1 and 32")
	}
	if cfg.QueueCapacity < 1 || cfg.QueueCapacity > 100000 {
		return errors.New("prompt audit queue_capacity must be between 1 and 100000")
	}
	if len(cfg.Scanners) == 0 {
		return errors.New("prompt audit requires at least one scanner")
	}
	seen := map[string]struct{}{}
	enabled := 0
	for _, ep := range cfg.Endpoints {
		if ep.ID == "" || ep.Name == "" {
			return errors.New("prompt audit endpoint id and name are required")
		}
		if _, ok := seen[ep.ID]; ok {
			return errors.New("prompt audit endpoint id must be unique")
		}
		seen[ep.ID] = struct{}{}
		if _, err := NormalizeBaseURL(ep.BaseURL); err != nil {
			return err
		}
		if ep.TimeoutMS < 100 || ep.TimeoutMS > 30000 {
			return errors.New("prompt audit endpoint timeout_ms must be between 100 and 30000")
		}
		if ep.InputLimit < 128 || ep.InputLimit > 100000 {
			return errors.New("prompt audit endpoint input_limit must be between 128 and 100000")
		}
		if ep.Enabled {
			enabled++
		}
	}
	if cfg.Enabled && enabled == 0 {
		return errors.New("enabled prompt audit requires an enabled endpoint")
	}
	return nil
}

func (cfg Config) IncludesTenant(id string) bool {
	if len(cfg.TenantIDs) == 0 {
		return true
	}
	id = strings.TrimSpace(id)
	i := sort.SearchStrings(cfg.TenantIDs, id)
	return i < len(cfg.TenantIDs) && cfg.TenantIDs[i] == id
}

func (cfg Config) EnabledEndpoints() []Endpoint {
	out := make([]Endpoint, 0, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		if ep.Enabled {
			out = append(out, ep)
		}
	}
	return out
}

func cloneConfig(cfg Config) Config {
	cfg.Scanners = append([]string(nil), cfg.Scanners...)
	cfg.TenantIDs = append([]string(nil), cfg.TenantIDs...)
	cfg.Endpoints = append([]Endpoint(nil), cfg.Endpoints...)
	return cfg
}
