package domain

import "time"

// SettingRiskControlConfig is the ai_settings key holding the serialized
// RiskControlConfig JSON. See init.sql for the default seed value.
const SettingRiskControlConfig = "risk_control_config"

// Risk control operating modes.
const (
	RiskControlModeOff      = "off"
	RiskControlModeObserve  = "observe"
	RiskControlModePreBlock = "pre_block"
)

// Per-log moderation actions.
const (
	RiskControlActionAllow        = "allow"
	RiskControlActionBlock        = "block"
	RiskControlActionKeywordBlock = "keyword_block"
	RiskControlActionError        = "error"
)

// HitLayer values record which detection layer produced a moderation result.
const (
	HitLayerCache   = "cache"   // L0 verdict cache hit
	HitLayerKeyword = "keyword" // L1 AC automaton match (normalized text)
	HitLayerPinyin  = "pinyin"  // L1 pinyin match
	HitLayerAPI     = "api"     // remote moderation API
)

// Keyword entry severity levels.
const (
	KeywordLevelBlock   = "block"   // direct block in pre_block mode
	KeywordLevelSuspect = "suspect" // log only, no block (L3 review pending)
)

// Risk event lifecycle states.
const (
	RiskEventStatusOpen         = "open"
	RiskEventStatusAcknowledged = "acknowledged"
	RiskEventStatusResolved     = "resolved"
	RiskEventStatusDismissed    = "dismissed"
)

// Risk event severities.
const (
	RiskEventSeverityLow    = "low"
	RiskEventSeverityMedium = "medium"
	RiskEventSeverityHigh   = "high"
)

// RiskControlProviderConfig points at the OpenAI-moderation-protocol
// endpoint used for API-based checks. APIKeyCiphertext is encrypted with
// the same secret master key as upstream account credentials.
type RiskControlProviderConfig struct {
	BaseURL          string `json:"base_url"`
	Model            string `json:"model"`
	APIKeyCiphertext string `json:"api_key_ciphertext"`
	TimeoutMs        int    `json:"timeout_ms"`
}

// KeywordEntry is a single keyword in the keyword library. Level controls
// whether a match blocks the request or only logs; RequireWith optionally
// requires co-occurring words to all appear before the entry is considered
// a hit (reduces false positives like "枪" matching "枪版电影").
type KeywordEntry struct {
	Word        string   `json:"word"`
	Level       string   `json:"level"`        // block | suspect
	RequireWith []string `json:"require_with"` // co-occurrence words; empty = no constraint
	Note        string   `json:"note"`
}

// PinyinConfig holds the pinyin-matching sub-library. It is independent
// from the main keyword entries so admins can selectively add only words
// prone to homophone bypass (brand names, drug names) without polluting
// the main library and triggering polyphone false positives.
type PinyinConfig struct {
	Enabled         bool           `json:"enabled"`
	Entries         []KeywordEntry `json:"entries"`
	IncludeInitials bool           `json:"include_initials"` // reserved; P0 always false
}

// KeywordConfig is the L1 keyword detection configuration.
type KeywordConfig struct {
	Enabled           bool              `json:"enabled"`
	Entries           []KeywordEntry    `json:"entries"`
	HomoglyphMapExtra map[string]string `json:"homoglyph_map_extra"` // site-specific homoglyph overrides
	Pinyin            PinyinConfig      `json:"pinyin"`
}

// RiskControlConfig is the full risk-control-center configuration,
// persisted as ai_settings[SettingRiskControlConfig].
type RiskControlConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // off | observe | pre_block

	// ConfigRevision is a monotonically increasing integer bumped on every
	// Update. The L0 verdict cache uses it to invalidate stale entries when
	// keywords/thresholds change.
	ConfigRevision int64 `json:"config_revision"`

	Keyword  KeywordConfig             `json:"keyword"`
	Provider RiskControlProviderConfig `json:"provider"`

	Thresholds map[string]float64 `json:"thresholds"`
	SampleRate float64            `json:"sample_rate"` // gates the Provider API stage only

	// VerdictCacheTTLSeconds controls the L0 verdict cache entry lifetime.
	// Default 600 (10 min). Set to 0 to disable caching.
	VerdictCacheTTLSeconds int `json:"verdict_cache_ttl_seconds"`

	// ScopeGroupIDs reserves per-group scoping for a later iteration.
	ScopeGroupIDs []string `json:"scope_group_ids"`

	ViolationWindowHours int    `json:"violation_window_hours"`
	RiskEventThreshold   int    `json:"risk_event_threshold"`
	RecordNonHits        bool   `json:"record_non_hits"`
	BlockStatusCode      int    `json:"block_status_code"`
	BlockMessage         string `json:"block_message"`
}

// DefaultRiskControlThresholds mirrors the OpenAI omni-moderation-latest
// category set. Used to backfill any category missing from stored config.
func DefaultRiskControlThresholds() map[string]float64 {
	return map[string]float64{
		"harassment":             0.7,
		"harassment/threatening": 0.7,
		"hate":                   0.7,
		"hate/threatening":       0.7,
		"illicit":                0.7,
		"illicit/violent":        0.7,
		"self-harm":              0.7,
		"self-harm/intent":       0.7,
		"self-harm/instructions": 0.7,
		"sexual":                 0.8,
		"sexual/minors":          0.5,
		"violence":               0.8,
		"violence/graphic":       0.8,
	}
}

// ContentModerationLog is a row in ai_content_moderation_logs: one detection
// result per checked request (keyword and/or API stage).
type ContentModerationLog struct {
	ID                string
	RequestID         string
	TenantID          string
	UserID            string
	APIKeyID          string
	ModelCode         string
	CapabilityType    string
	Mode              string
	Action            string
	Flagged           bool
	MatchedKeyword    string
	HighestCategory   string
	HighestScore      *float64
	CategoryScores    []byte // raw JSON
	ThresholdSnapshot []byte // raw JSON
	InputExcerpt      string
	UpstreamLatencyMs *int32
	Error             string
	HitLayer          string // cache | keyword | pinyin | api
	CreatedAt         time.Time
}

// ContentModerationLogFilter scopes moderation-log queries. Zero values mean
// "no filter".
type ContentModerationLogFilter struct {
	TenantID string
	UserID   string
	Mode     string
	Action   string
	Flagged  *bool
	HitLayer string
	DateFrom *time.Time
	DateTo   *time.Time
}

// RiskControlDetection is the pure moderation outcome before persistence or
// follow-up event creation.
type RiskControlDetection struct {
	Flagged           bool
	MatchedKeyword    string
	HighestCategory   string
	HighestScore      *float64
	CategoryScores    map[string]float64
	APIError          string
	UpstreamLatencyMs *int32
	HitLayer          string
	FromCache         bool
}

// ContentModerationLogPage is a paginated moderation log projection.
type ContentModerationLogPage struct {
	Items []ContentModerationLog
	Total int64
}

// RiskEvent is a row in ai_risk_events: a human-in-the-loop item generated
// when violations accumulate past RiskControlConfig.RiskEventThreshold
// within the rolling window. Risk handling never auto-mutates account status;
// an admin resolves the event and, if needed, separately bans the user.
type RiskEvent struct {
	ID             string
	EventType      string
	Severity       string
	TenantID       string
	UserID         string
	SourceLogID    string
	Summary        string
	Detail         []byte // raw JSON
	Status         string
	ResolvedBy     string
	ResolvedAt     *time.Time
	ResolutionNote string
	CreatedAt      time.Time
}

// RiskEventFilter scopes risk-event queries. Zero values mean "no filter".
type RiskEventFilter struct {
	Status   string
	TenantID string
	UserID   string
}

// RiskEventPage is a paginated human-review event projection.
type RiskEventPage struct {
	Items []RiskEvent
	Total int64
}
