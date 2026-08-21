package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

// ============================================================================
// DTOs
// ============================================================================

type riskControlProviderDTO struct {
	BaseURL   string `json:"base_url" doc:"审核 API Base URL，如 https://api.openai.com"`
	Model     string `json:"model" doc:"审核模型，如 omni-moderation-latest"`
	HasAPIKey bool   `json:"has_api_key" doc:"是否已配置审核 API Key（出于安全考虑不回显明文/密文）"`
	TimeoutMs int    `json:"timeout_ms" doc:"审核 API 调用超时（毫秒）"`
}

type riskControlProviderWriteDTO struct {
	BaseURL   string  `json:"base_url"`
	Model     string  `json:"model"`
	APIKey    *string `json:"api_key,omitempty" doc:"传入以更新审核 API Key 明文；省略则保留原值，传空字符串则清空"`
	TimeoutMs int     `json:"timeout_ms"`
}

type keywordEntryDTO struct {
	Word        string   `json:"word"`
	Level       string   `json:"level" doc:"block | suspect"`
	RequireWith []string `json:"require_with" doc:"共现词；全部出现才算命中"`
	Note        string   `json:"note"`
}

type pinyinConfigDTO struct {
	Enabled         bool              `json:"enabled"`
	Entries         []keywordEntryDTO `json:"entries"`
	IncludeInitials bool              `json:"include_initials" doc:"首字母缩写匹配（预留，P0 固定 false）"`
}

type keywordConfigDTO struct {
	Enabled           bool              `json:"enabled"`
	Entries           []keywordEntryDTO `json:"entries"`
	HomoglyphMapExtra map[string]string `json:"homoglyph_map_extra" doc:"站点自定义同形字映射"`
	Pinyin            pinyinConfigDTO   `json:"pinyin"`
}

type riskControlConfigDTO struct {
	Enabled                bool                   `json:"enabled"`
	Mode                   string                 `json:"mode" doc:"off | observe | pre_block"`
	ConfigRevision         int64                  `json:"config_revision" doc:"配置版本号，每次更新自增"`
	Keyword                keywordConfigDTO       `json:"keyword"`
	Provider               riskControlProviderDTO `json:"provider"`
	Thresholds             map[string]float64     `json:"thresholds"`
	SampleRate             float64                `json:"sample_rate" doc:"0~1，审核 API 采样率"`
	VerdictCacheTTLSeconds int                    `json:"verdict_cache_ttl_seconds" doc:"L0 裁决缓存 TTL，默认 600"`
	ScopeGroupIDs          []string               `json:"scope_group_ids" doc:"预留"`
	ViolationWindowHours   int                    `json:"violation_window_hours"`
	RiskEventThreshold     int                    `json:"risk_event_threshold"`
	RecordNonHits          bool                   `json:"record_non_hits"`
	BlockStatusCode        int                    `json:"block_status_code"`
	BlockMessage           string                 `json:"block_message"`
}

type riskControlConfigWriteDTO struct {
	Enabled                bool                        `json:"enabled"`
	Mode                   string                      `json:"mode" enum:"off,observe,pre_block"`
	Keyword                keywordConfigDTO            `json:"keyword"`
	Provider               riskControlProviderWriteDTO `json:"provider"`
	Thresholds             map[string]float64          `json:"thresholds"`
	SampleRate             float64                     `json:"sample_rate"`
	VerdictCacheTTLSeconds int                         `json:"verdict_cache_ttl_seconds"`
	ScopeGroupIDs          []string                    `json:"scope_group_ids,omitempty" doc:"预留"`
	ViolationWindowHours   int                         `json:"violation_window_hours"`
	RiskEventThreshold     int                         `json:"risk_event_threshold"`
	RecordNonHits          bool                        `json:"record_non_hits"`
	BlockStatusCode        int                         `json:"block_status_code"`
	BlockMessage           string                      `json:"block_message"`
}

type riskControlConfigOutput struct {
	Body riskControlConfigDTO
}

type updateRiskControlConfigInput struct {
	Body riskControlConfigWriteDTO
}

type testRiskControlInput struct {
	Body struct {
		Text string `json:"text" doc:"待检测文本"`
	}
}

type testRiskControlOutput struct {
	Body struct {
		Flagged         bool               `json:"flagged"`
		MatchedKeyword  *string            `json:"matched_keyword,omitempty"`
		HitLayer        *string            `json:"hit_layer,omitempty" doc:"cache | keyword | pinyin | api"`
		FromCache       bool               `json:"from_cache"`
		HighestCategory *string            `json:"highest_category,omitempty"`
		HighestScore    *float64           `json:"highest_score,omitempty"`
		CategoryScores  map[string]float64 `json:"category_scores,omitempty"`
		Error           *string            `json:"error,omitempty"`
	}
}

type riskControlLogDTO struct {
	ID                string             `json:"id"`
	RequestID         *string            `json:"request_id,omitempty"`
	TenantID          *string            `json:"tenant_id,omitempty"`
	UserID            *string            `json:"user_id,omitempty"`
	APIKeyID          *string            `json:"api_key_id,omitempty"`
	ModelCode         *string            `json:"model_code,omitempty"`
	CapabilityType    *string            `json:"capability_type,omitempty"`
	Mode              string             `json:"mode"`
	Action            string             `json:"action"`
	Flagged           bool               `json:"flagged"`
	MatchedKeyword    *string            `json:"matched_keyword,omitempty"`
	HitLayer          *string            `json:"hit_layer,omitempty" doc:"cache | keyword | pinyin | api"`
	HighestCategory   *string            `json:"highest_category,omitempty"`
	HighestScore      *float64           `json:"highest_score,omitempty"`
	CategoryScores    map[string]float64 `json:"category_scores,omitempty"`
	InputExcerpt      *string            `json:"input_excerpt,omitempty"`
	UpstreamLatencyMs *int32             `json:"upstream_latency_ms,omitempty"`
	Error             *string            `json:"error,omitempty"`
	CreatedAt         *int64             `json:"created_at,omitempty" doc:"Unix 毫秒"`
}

type riskControlLogsInput struct {
	TenantID string `query:"tenant_id"`
	UserID   string `query:"user_id"`
	Mode     string `query:"mode" doc:"observe | pre_block"`
	Action   string `query:"action" doc:"allow | block | keyword_block | error"`
	Flagged  string `query:"flagged" enum:",true,false" doc:"留空表示不过滤"`
	HitLayer string `query:"hit_layer" doc:"cache | keyword | pinyin | api"`
	DateFrom string `query:"date_from" doc:"开始时间，RFC3339"`
	DateTo   string `query:"date_to" doc:"结束时间，RFC3339"`
	Limit    int32  `query:"limit" default:"50" doc:"返回条数；默认 50，最大 200"`
	Offset   int32  `query:"offset" default:"0"`
}

type riskControlLogsOutput struct {
	Body struct {
		Items []riskControlLogDTO `json:"items"`
		Total int64               `json:"total"`
	}
}

type riskEventDTO struct {
	ID             string  `json:"id"`
	EventType      string  `json:"event_type"`
	Severity       string  `json:"severity"`
	TenantID       *string `json:"tenant_id,omitempty"`
	UserID         *string `json:"user_id,omitempty"`
	SourceLogID    *string `json:"source_log_id,omitempty"`
	Summary        string  `json:"summary"`
	Detail         []byte  `json:"detail,omitempty"`
	Status         string  `json:"status"`
	ResolvedBy     *string `json:"resolved_by,omitempty"`
	ResolvedAt     *int64  `json:"resolved_at,omitempty" doc:"Unix 毫秒"`
	ResolutionNote *string `json:"resolution_note,omitempty"`
	CreatedAt      *int64  `json:"created_at,omitempty" doc:"Unix 毫秒"`
}

type riskEventsInput struct {
	Status   string `query:"status" doc:"open | acknowledged | resolved | dismissed"`
	TenantID string `query:"tenant_id"`
	UserID   string `query:"user_id"`
	Limit    int32  `query:"limit" default:"50" doc:"返回条数；默认 50，最大 200"`
	Offset   int32  `query:"offset" default:"0"`
}

type riskEventsOutput struct {
	Body struct {
		Items []riskEventDTO `json:"items"`
		Total int64          `json:"total"`
	}
}

type riskEventOutput struct {
	Body riskEventDTO
}

type resolveRiskEventInput struct {
	EventID string `path:"eventID"`
	Body    struct {
		Status string `json:"status" enum:"acknowledged,resolved,dismissed"`
		Note   string `json:"note,omitempty"`
	}
}

// ============================================================================
// Registration
// ============================================================================

func registerRiskControl(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-risk-control-config",
		Method:      http.MethodGet,
		Path:        "/api/v1/risk-control/config",
		Summary:     "风控中心配置",
		Tags:        []string{"risk-control"},
	}, func(ctx context.Context, _ *struct{}) (*riskControlConfigOutput, error) {
		if d.RiskControlConfig == nil {
			return nil, httpx.ErrUnavailable.WithDetail("risk control service is not configured")
		}
		cfg, err := d.RiskControlConfig.Get(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &riskControlConfigOutput{Body: riskControlConfigToDTO(cfg)}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-risk-control-config",
		Method:      http.MethodPut,
		Path:        "/api/v1/risk-control/config",
		Summary:     "更新风控中心配置",
		Tags:        []string{"risk-control"},
	}, func(ctx context.Context, in *updateRiskControlConfigInput) (*riskControlConfigOutput, error) {
		if d.RiskControlConfig == nil {
			return nil, httpx.ErrUnavailable.WithDetail("risk control service is not configured")
		}
		current, err := d.RiskControlConfig.Get(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		cfg, err := riskControlConfigFromWriteDTO(in.Body, current, d.ProviderSecrets)
		if err != nil {
			return nil, err
		}
		if err := d.RiskControlConfig.Update(ctx, cfg); err != nil {
			return nil, mapServiceError(err)
		}
		out := &riskControlConfigOutput{Body: riskControlConfigToDTO(cfg)}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-test-risk-control-moderation",
		Method:      http.MethodPost,
		Path:        "/api/v1/risk-control/test",
		Summary:     "测试风控检测（关键词 + 审核 API），不落库",
		Tags:        []string{"risk-control"},
	}, func(ctx context.Context, in *testRiskControlInput) (*testRiskControlOutput, error) {
		if d.RiskControlConfig == nil || d.RiskControlDetector == nil {
			return nil, httpx.ErrUnavailable.WithDetail("risk control service is not configured")
		}
		text := strings.TrimSpace(in.Body.Text)
		if text == "" {
			return nil, httpx.ErrBadRequest.WithDetail("text is required")
		}
		cfg, err := d.RiskControlConfig.Get(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		det := d.RiskControlDetector.Detect(ctx, cfg, text)
		out := &testRiskControlOutput{}
		out.Body.Flagged = det.Flagged
		out.Body.MatchedKeyword = stringPtrOrNil(det.MatchedKeyword)
		out.Body.HitLayer = stringPtrOrNil(det.HitLayer)
		out.Body.FromCache = det.FromCache
		out.Body.HighestCategory = stringPtrOrNil(det.HighestCategory)
		out.Body.HighestScore = det.HighestScore
		out.Body.CategoryScores = det.CategoryScores
		out.Body.Error = stringPtrOrNil(det.APIError)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-risk-control-logs",
		Method:      http.MethodGet,
		Path:        "/api/v1/risk-control/logs",
		Summary:     "风控审核日志列表",
		Tags:        []string{"risk-control"},
	}, func(ctx context.Context, in *riskControlLogsInput) (*riskControlLogsOutput, error) {
		if d.RiskControlLogs == nil {
			return nil, httpx.ErrUnavailable.WithDetail("risk control service is not configured")
		}
		dateFrom, dateTo, err := parseOptionalRFC3339Window(in.DateFrom, in.DateTo)
		if err != nil {
			return nil, err
		}
		filter := domain.ContentModerationLogFilter{
			TenantID: in.TenantID,
			UserID:   in.UserID,
			Mode:     in.Mode,
			Action:   in.Action,
			Flagged:  parseOptionalBool(in.Flagged),
			HitLayer: in.HitLayer,
			DateFrom: dateFrom,
			DateTo:   dateTo,
		}
		page, err := d.RiskControlLogs.List(ctx, filter, in.Limit, in.Offset)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &riskControlLogsOutput{}
		out.Body.Total = page.Total
		out.Body.Items = make([]riskControlLogDTO, 0, len(page.Items))
		for _, item := range page.Items {
			out.Body.Items = append(out.Body.Items, riskControlLogToDTO(item))
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-risk-events",
		Method:      http.MethodGet,
		Path:        "/api/v1/risk-control/events",
		Summary:     "风险事件列表",
		Description: "内容违规累计命中达到阈值时生成的人工待办；处置只更新事件状态，不修改账号/租户状态。",
		Tags:        []string{"risk-control"},
	}, func(ctx context.Context, in *riskEventsInput) (*riskEventsOutput, error) {
		if d.RiskEvents == nil {
			return nil, httpx.ErrUnavailable.WithDetail("risk control service is not configured")
		}
		page, err := d.RiskEvents.List(ctx, domain.RiskEventFilter{
			Status:   in.Status,
			TenantID: in.TenantID,
			UserID:   in.UserID,
		}, in.Limit, in.Offset)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &riskEventsOutput{}
		out.Body.Total = page.Total
		out.Body.Items = make([]riskEventDTO, 0, len(page.Items))
		for _, item := range page.Items {
			out.Body.Items = append(out.Body.Items, riskEventToDTO(item))
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-resolve-risk-event",
		Method:      http.MethodPost,
		Path:        "/api/v1/risk-control/events/{eventID}/resolve",
		Summary:     "处置风险事件",
		Description: "仅更新事件状态；如需封禁用户或租户，请在用户管理中单独操作。",
		Tags:        []string{"risk-control"},
	}, func(ctx context.Context, in *resolveRiskEventInput) (*riskEventOutput, error) {
		if d.RiskEvents == nil {
			return nil, httpx.ErrUnavailable.WithDetail("risk control service is not configured")
		}
		actor := strings.TrimSpace(claimsUserID(ctx))
		ev, err := d.RiskEvents.Resolve(ctx, in.EventID, in.Body.Status, actor, in.Body.Note)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &riskEventOutput{Body: riskEventToDTO(ev)}
		return out, nil
	})
}

// ============================================================================
// DTO <-> domain conversion
// ============================================================================

func riskControlConfigToDTO(cfg domain.RiskControlConfig) riskControlConfigDTO {
	return riskControlConfigDTO{
		Enabled:        cfg.Enabled,
		Mode:           cfg.Mode,
		ConfigRevision: cfg.ConfigRevision,
		Keyword:        keywordConfigToDTO(cfg.Keyword),
		Provider: riskControlProviderDTO{
			BaseURL:   cfg.Provider.BaseURL,
			Model:     cfg.Provider.Model,
			HasAPIKey: cfg.Provider.APIKeyCiphertext != "",
			TimeoutMs: cfg.Provider.TimeoutMs,
		},
		Thresholds:             cfg.Thresholds,
		SampleRate:             cfg.SampleRate,
		VerdictCacheTTLSeconds: cfg.VerdictCacheTTLSeconds,
		ScopeGroupIDs:          cfg.ScopeGroupIDs,
		ViolationWindowHours:   cfg.ViolationWindowHours,
		RiskEventThreshold:     cfg.RiskEventThreshold,
		RecordNonHits:          cfg.RecordNonHits,
		BlockStatusCode:        cfg.BlockStatusCode,
		BlockMessage:           cfg.BlockMessage,
	}
}

func keywordConfigToDTO(kc domain.KeywordConfig) keywordConfigDTO {
	entries := make([]keywordEntryDTO, 0, len(kc.Entries))
	for _, e := range kc.Entries {
		entries = append(entries, keywordEntryDTO{
			Word:        e.Word,
			Level:       e.Level,
			RequireWith: e.RequireWith,
			Note:        e.Note,
		})
	}
	pinyinEntries := make([]keywordEntryDTO, 0, len(kc.Pinyin.Entries))
	for _, e := range kc.Pinyin.Entries {
		pinyinEntries = append(pinyinEntries, keywordEntryDTO{
			Word:        e.Word,
			Level:       e.Level,
			RequireWith: e.RequireWith,
			Note:        e.Note,
		})
	}
	return keywordConfigDTO{
		Enabled:           kc.Enabled,
		Entries:           entries,
		HomoglyphMapExtra: kc.HomoglyphMapExtra,
		Pinyin: pinyinConfigDTO{
			Enabled:         kc.Pinyin.Enabled,
			Entries:         pinyinEntries,
			IncludeInitials: kc.Pinyin.IncludeInitials,
		},
	}
}

// riskControlConfigFromWriteDTO merges a write DTO onto the current stored
// config. The API key ciphertext is preserved unless Provider.APIKey is
// explicitly set (nil = keep, "" = clear, non-empty = re-encrypt).
func riskControlConfigFromWriteDTO(in riskControlConfigWriteDTO, current domain.RiskControlConfig, providerSecrets ProviderSecretCodec) (domain.RiskControlConfig, error) {
	cfg := domain.RiskControlConfig{
		Enabled: in.Enabled,
		Mode:    in.Mode,
		Keyword: keywordConfigFromDTO(in.Keyword),
		Provider: domain.RiskControlProviderConfig{
			BaseURL:          in.Provider.BaseURL,
			Model:            in.Provider.Model,
			APIKeyCiphertext: current.Provider.APIKeyCiphertext,
			TimeoutMs:        in.Provider.TimeoutMs,
		},
		Thresholds:             in.Thresholds,
		SampleRate:             in.SampleRate,
		VerdictCacheTTLSeconds: in.VerdictCacheTTLSeconds,
		ScopeGroupIDs:          in.ScopeGroupIDs,
		ViolationWindowHours:   in.ViolationWindowHours,
		RiskEventThreshold:     in.RiskEventThreshold,
		RecordNonHits:          in.RecordNonHits,
		BlockStatusCode:        in.BlockStatusCode,
		BlockMessage:           in.BlockMessage,
	}
	if in.Provider.APIKey != nil {
		if *in.Provider.APIKey == "" {
			cfg.Provider.APIKeyCiphertext = ""
		} else {
			if providerSecrets == nil {
				return domain.RiskControlConfig{}, httpx.ErrUnavailable.WithDetail("provider secret codec is not configured")
			}
			ciphertext, err := providerSecrets.Encrypt(*in.Provider.APIKey)
			if err != nil {
				return domain.RiskControlConfig{}, httpx.ErrInternal.WithDetail("failed to encrypt moderation api key").WithCause(err)
			}
			cfg.Provider.APIKeyCiphertext = ciphertext
		}
	}
	return cfg, nil
}

func keywordConfigFromDTO(kc keywordConfigDTO) domain.KeywordConfig {
	entries := make([]domain.KeywordEntry, 0, len(kc.Entries))
	for _, e := range kc.Entries {
		entries = append(entries, domain.KeywordEntry{
			Word:        e.Word,
			Level:       e.Level,
			RequireWith: e.RequireWith,
			Note:        e.Note,
		})
	}
	pinyinEntries := make([]domain.KeywordEntry, 0, len(kc.Pinyin.Entries))
	for _, e := range kc.Pinyin.Entries {
		pinyinEntries = append(pinyinEntries, domain.KeywordEntry{
			Word:        e.Word,
			Level:       e.Level,
			RequireWith: e.RequireWith,
			Note:        e.Note,
		})
	}
	return domain.KeywordConfig{
		Enabled:           kc.Enabled,
		Entries:           entries,
		HomoglyphMapExtra: kc.HomoglyphMapExtra,
		Pinyin: domain.PinyinConfig{
			Enabled:         kc.Pinyin.Enabled,
			Entries:         pinyinEntries,
			IncludeInitials: kc.Pinyin.IncludeInitials,
		},
	}
}

func riskControlLogToDTO(log domain.ContentModerationLog) riskControlLogDTO {
	dto := riskControlLogDTO{
		ID:                log.ID,
		RequestID:         stringPtrOrNil(log.RequestID),
		TenantID:          stringPtrOrNil(log.TenantID),
		UserID:            stringPtrOrNil(log.UserID),
		APIKeyID:          stringPtrOrNil(log.APIKeyID),
		ModelCode:         stringPtrOrNil(log.ModelCode),
		CapabilityType:    stringPtrOrNil(log.CapabilityType),
		Mode:              log.Mode,
		Action:            log.Action,
		Flagged:           log.Flagged,
		MatchedKeyword:    stringPtrOrNil(log.MatchedKeyword),
		HitLayer:          stringPtrOrNil(log.HitLayer),
		HighestCategory:   stringPtrOrNil(log.HighestCategory),
		HighestScore:      log.HighestScore,
		InputExcerpt:      stringPtrOrNil(log.InputExcerpt),
		UpstreamLatencyMs: log.UpstreamLatencyMs,
		Error:             stringPtrOrNil(log.Error),
		CreatedAt:         timeToMillisPtr(log.CreatedAt),
	}
	if len(log.CategoryScores) > 0 {
		_ = json.Unmarshal(log.CategoryScores, &dto.CategoryScores)
	}
	return dto
}

func parseOptionalBool(s string) *bool {
	switch s {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	default:
		return nil
	}
}

func timePtrToMillisPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	return timeToMillisPtr(*t)
}

func riskEventToDTO(ev domain.RiskEvent) riskEventDTO {
	return riskEventDTO{
		ID:             ev.ID,
		EventType:      ev.EventType,
		Severity:       ev.Severity,
		TenantID:       stringPtrOrNil(ev.TenantID),
		UserID:         stringPtrOrNil(ev.UserID),
		SourceLogID:    stringPtrOrNil(ev.SourceLogID),
		Summary:        ev.Summary,
		Detail:         ev.Detail,
		Status:         ev.Status,
		ResolvedBy:     stringPtrOrNil(ev.ResolvedBy),
		ResolvedAt:     timePtrToMillisPtr(ev.ResolvedAt),
		ResolutionNote: stringPtrOrNil(ev.ResolutionNote),
		CreatedAt:      timeToMillisPtr(ev.CreatedAt),
	}
}
