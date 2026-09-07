package riskcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sync"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

// configCacheTTL bounds how long a fetched RiskControlConfig is reused
// before re-reading ai_settings. Mirrors other short-lived settings caches in
// pricebook_billing.go, shorter because admins expect config edits (e.g.
// flipping the kill switch) to take effect quickly.
const configCacheTTL = 15 * time.Second

// defaultVerdictCacheCapacity is the L0 verdict cache size (entries).
const defaultVerdictCacheCapacity = 10000

var configReadFailures = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "dai_risk_control_config_read_failures_total",
	Help: "Risk control configuration failures, by storage or decode stage.",
}, []string{"stage"})

// SettingRepository owns configuration persistence and atomic revision allocation.
type SettingRepository interface {
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
	UpsertVersionedSetting(ctx context.Context, key string, value json.RawMessage, expectedRevision int64) (json.RawMessage, error)
}

// ConfigService caches and serves the risk-control configuration. Every
// pipeline request reads through Get(); a short TTL keeps that off the DB
// hot path while still picking up admin edits within seconds.
type ConfigService struct {
	repo SettingRepository

	mu         sync.RWMutex
	cached     *domain.RiskControlConfig
	cachedAt   time.Time
	generation uint64
}

func NewConfigService(repo SettingRepository) *ConfigService {
	return &ConfigService{repo: repo}
}

// Get returns the current config, using the cached copy when fresh.
// Only a missing setting uses the disabled default. Dependency and decoding
// failures remain errors so callers can apply an explicit failure policy.
func (s *ConfigService) Get(ctx context.Context) (domain.RiskControlConfig, error) {
	s.mu.RLock()
	if s.cached != nil && time.Since(s.cachedAt) < configCacheTTL {
		cfg := *s.cached
		s.mu.RUnlock()
		return cfg, nil
	}
	generation := s.generation
	s.mu.RUnlock()

	raw, err := s.repo.GetSetting(ctx, domain.SettingRiskControlConfig)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.RiskControlConfig{Mode: domain.RiskControlModeOff}, nil
	}
	if err != nil {
		configReadFailures.WithLabelValues("storage").Inc()
		return domain.RiskControlConfig{}, fmt.Errorf("load risk control config: %w", err)
	}
	var cfg domain.RiskControlConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		configReadFailures.WithLabelValues("decode").Inc()
		return domain.RiskControlConfig{}, fmt.Errorf("decode risk control config: %w", err)
	}
	backfillConfig(&cfg)

	s.mu.Lock()
	if generation == s.generation {
		s.cached = &cfg
		s.cachedAt = time.Now()
	}
	s.mu.Unlock()

	out := cfg
	return out, nil
}

// Update validates and persists a new config, then invalidates the cache
// so the next Get() re-reads from Postgres. ConfigRevision is bumped
// automatically to invalidate the L0 verdict cache.
func (s *ConfigService) Update(ctx context.Context, cfg domain.RiskControlConfig) (domain.RiskControlConfig, error) {
	if err := validateConfig(&cfg); err != nil {
		return domain.RiskControlConfig{}, err
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return domain.RiskControlConfig{}, err
	}
	saved, err := s.repo.UpsertVersionedSetting(ctx, domain.SettingRiskControlConfig, raw, cfg.ConfigRevision)
	if err != nil && !errors.Is(err, domain.ErrConflict) {
		return domain.RiskControlConfig{}, err
	}
	s.mu.Lock()
	s.generation++
	s.cached = nil
	s.mu.Unlock()
	if err != nil {
		return domain.RiskControlConfig{}, err
	}
	if err := json.Unmarshal(saved, &cfg); err != nil {
		return domain.RiskControlConfig{}, fmt.Errorf("decode saved risk control config: %w", err)
	}
	return cfg, nil
}

// backfillConfig applies default values for any missing/zero fields so
// the rest of the pipeline can assume a well-formed config.
func backfillConfig(cfg *domain.RiskControlConfig) {
	if cfg.Keyword.Mode == "" {
		cfg.Keyword.Mode = cfg.Mode
	}
	if cfg.Provider.Mode == "" {
		cfg.Provider.Mode = cfg.Mode
	}
	if !cfg.Keyword.Enabled && cfg.Enabled {
		cfg.Keyword.Enabled = true
	}
	if !cfg.Provider.Enabled && cfg.Enabled {
		cfg.Provider.Enabled = true
	}
	if cfg.Thresholds == nil {
		cfg.Thresholds = map[string]float64{}
	}
	for category, def := range domain.DefaultRiskControlThresholds() {
		if _, ok := cfg.Thresholds[category]; !ok {
			cfg.Thresholds[category] = def
		}
	}

	// Backfill keyword defaults.
	if cfg.Keyword.Entries == nil {
		cfg.Keyword.Entries = []domain.KeywordEntry{}
	}
	for i := range cfg.Keyword.Entries {
		if cfg.Keyword.Entries[i].Level == "" {
			cfg.Keyword.Entries[i].Level = domain.KeywordLevelBlock
		}
		if cfg.Keyword.Entries[i].RequireWith == nil {
			cfg.Keyword.Entries[i].RequireWith = []string{}
		}
	}
	if cfg.Keyword.Pinyin.Entries == nil {
		cfg.Keyword.Pinyin.Entries = []domain.KeywordEntry{}
	}
	for i := range cfg.Keyword.Pinyin.Entries {
		if cfg.Keyword.Pinyin.Entries[i].Level == "" {
			cfg.Keyword.Pinyin.Entries[i].Level = domain.KeywordLevelBlock
		}
		if cfg.Keyword.Pinyin.Entries[i].RequireWith == nil {
			cfg.Keyword.Pinyin.Entries[i].RequireWith = []string{}
		}
	}

	if cfg.VerdictCacheTTLSeconds == 0 {
		cfg.VerdictCacheTTLSeconds = 600
	}
	if cfg.ScopeGroupIDs == nil {
		cfg.ScopeGroupIDs = []string{}
	}
	if cfg.KeywordUpstreamAccountIDs == nil {
		cfg.KeywordUpstreamAccountIDs = []string{}
	}
	if cfg.ProviderUpstreamAccountIDs == nil {
		cfg.ProviderUpstreamAccountIDs = []string{}
	}
	if cfg.ViolationWindowHours <= 0 {
		cfg.ViolationWindowHours = 24
	}
	if cfg.RiskEventThreshold <= 0 {
		cfg.RiskEventThreshold = 3
	}
	if cfg.BlockStatusCode == 0 {
		cfg.BlockStatusCode = 451
	}
	if cfg.BlockMessage == "" {
		cfg.BlockMessage = "请求内容未通过安全审核"
	}
}

func validateConfig(cfg *domain.RiskControlConfig) error {
	switch cfg.Mode {
	case domain.RiskControlModeOff, domain.RiskControlModeObserve, domain.RiskControlModePreBlock:
	default:
		return domain.NewValidationError("mode", "must be one of off, observe, pre_block")
	}
	if cfg.SampleRate < 0 || cfg.SampleRate > 1 {
		return domain.NewValidationError("sample_rate", "must be between 0 and 1")
	}
	// Validate keyword entries.
	for _, entry := range cfg.Keyword.Entries {
		if entry.Word == "" {
			return domain.NewValidationError("keyword.entries", "word must not be empty")
		}
		switch entry.Level {
		case domain.KeywordLevelBlock, domain.KeywordLevelSuspect:
		default:
			return domain.NewValidationError("keyword.entries", "level must be block or suspect")
		}
	}
	// Validate pinyin entries (same rules).
	for _, entry := range cfg.Keyword.Pinyin.Entries {
		if entry.Word == "" {
			return domain.NewValidationError("keyword.pinyin.entries", "word must not be empty")
		}
		switch entry.Level {
		case domain.KeywordLevelBlock, domain.KeywordLevelSuspect:
		default:
			return domain.NewValidationError("keyword.pinyin.entries", "level must be block or suspect")
		}
	}
	if cfg.VerdictCacheTTLSeconds < 0 {
		return domain.NewValidationError("verdict_cache_ttl_seconds", "must be >= 0")
	}
	backfillConfig(cfg)
	return nil
}
