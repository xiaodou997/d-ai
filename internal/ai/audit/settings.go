package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const RecordingSettingKey = "request_recording"

var ErrInvalidRecordingSettings = errors.New("invalid request recording settings")

type RecordingSettings struct {
	Level            string   `json:"level" enum:"basic,headers,full"`
	SensitiveHeaders []string `json:"sensitive_headers"`
}

var mandatorySensitiveHeaders = []string{"authorization", "proxy-authorization", "x-api-key", "api-key", "cookie", "set-cookie"}

func DefaultRecordingSettings() RecordingSettings {
	return RecordingSettings{Level: "basic", SensitiveHeaders: append([]string(nil), mandatorySensitiveHeaders...)}
}

type RecordingSettingsRepository interface {
	GetSetting(context.Context, string) (json.RawMessage, error)
	UpsertSetting(context.Context, string, json.RawMessage) error
}

// RecordingSettingsService persists administrator settings in ai_settings.
// There is deliberately no environment-variable override.
type RecordingSettingsService struct {
	repo    RecordingSettingsRepository
	mu      sync.Mutex
	cached  RecordingSettings
	expires time.Time
}

func NewRecordingSettingsService(repo RecordingSettingsRepository) *RecordingSettingsService {
	return &RecordingSettingsService{repo: repo}
}

func normalizeRecordingSettings(c RecordingSettings) (RecordingSettings, error) {
	if c.Level != "basic" && c.Level != "headers" && c.Level != "full" {
		return RecordingSettings{}, fmt.Errorf("%w: invalid recording level %q", ErrInvalidRecordingSettings, c.Level)
	}
	if len(c.SensitiveHeaders) > 100 {
		return RecordingSettings{}, fmt.Errorf("%w: too many sensitive headers", ErrInvalidRecordingSettings)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(c.SensitiveHeaders)+len(mandatorySensitiveHeaders))
	for _, name := range append(append([]string(nil), mandatorySensitiveHeaders...), c.SensitiveHeaders...) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if len(name) > 128 {
			return RecordingSettings{}, fmt.Errorf("%w: header name too long", ErrInvalidRecordingSettings)
		}
		for _, ch := range name {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", ch)) {
				return RecordingSettings{}, fmt.Errorf("%w: invalid sensitive header name", ErrInvalidRecordingSettings)
			}
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	c.SensitiveHeaders = out
	return c, nil
}

// ParseRecordingSettings decodes and normalizes the persisted JSON value. A
// missing field keeps the default, while malformed values are rejected so the
// caller can fail closed to basic recording.
func ParseRecordingSettings(raw json.RawMessage) (RecordingSettings, error) {
	c := DefaultRecordingSettings()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &c); err != nil {
			return RecordingSettings{}, err
		}
	}
	return normalizeRecordingSettings(c)
}

func (s *RecordingSettingsService) Get(ctx context.Context) (RecordingSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.expires) {
		c := s.cached
		c.SensitiveHeaders = append([]string(nil), c.SensitiveHeaders...)
		return c, nil
	}
	raw, err := s.repo.GetSetting(ctx, RecordingSettingKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return RecordingSettings{}, err
	}
	if err != nil {
		raw = nil
	}
	c, err := ParseRecordingSettings(raw)
	if err != nil {
		return RecordingSettings{}, err
	}
	s.cached = c
	s.expires = time.Now().Add(5 * time.Second)
	c.SensitiveHeaders = append([]string(nil), c.SensitiveHeaders...)
	return c, nil
}

func (s *RecordingSettingsService) Update(ctx context.Context, c RecordingSettings) (RecordingSettings, error) {
	c, err := normalizeRecordingSettings(c)
	if err != nil {
		return RecordingSettings{}, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return RecordingSettings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.repo.UpsertSetting(ctx, RecordingSettingKey, raw); err != nil {
		return RecordingSettings{}, err
	}
	s.cached = c
	s.cached.SensitiveHeaders = append([]string(nil), c.SensitiveHeaders...)
	s.expires = time.Now().Add(5 * time.Second)
	c.SensitiveHeaders = append([]string(nil), c.SensitiveHeaders...)
	return c, nil
}
