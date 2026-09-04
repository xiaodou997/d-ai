package promptaudit

import "time"

const (
	SettingKey        = "prompt_audit_config"
	DefaultGuardModel = "sileader/qwen3guard:0.6b"

	ModeOff      = "off"
	ModeObserve  = "observe"
	ModeBlocking = "blocking"

	ErrorBlocked         = "prompt_guard_blocked"
	ErrorUnavailable     = "prompt_guard_unavailable"
	ErrorInvalidResponse = "prompt_guard_invalid_response"
)

var ScannerIDs = []string{
	"violent",
	"non_violent_illegal_acts",
	"sexual_content_or_sexual_acts",
	"pii",
	"suicide_and_self_harm",
	"unethical_acts",
	"politically_sensitive_topics",
	"copyright_violation",
	"jailbreak",
}

type Endpoint struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	BaseURL          string `json:"base_url"`
	Model            string `json:"model"`
	APIKeyCiphertext string `json:"api_key_ciphertext,omitempty"`
	TimeoutMS        int    `json:"timeout_ms"`
	InputLimit       int    `json:"input_limit"`
	Enabled          bool   `json:"enabled"`
}

type Config struct {
	Enabled         bool       `json:"enabled"`
	Mode            string     `json:"mode"`
	LatestTurnOnly  bool       `json:"latest_turn_only"`
	StorePassEvents bool       `json:"store_pass_events"`
	WorkerCount     int        `json:"worker_count"`
	QueueCapacity   int        `json:"queue_capacity"`
	Scanners        []string   `json:"scanners"`
	TenantIDs       []string   `json:"tenant_ids"`
	Endpoints       []Endpoint `json:"endpoints"`
	ConfigRevision  int64      `json:"config_revision"`
}

type Input struct {
	RequestID      string
	TenantID       string
	UserID         string
	APIKeyID       string
	ModelCode      string
	CapabilityType string
	Protocol       string
	Body           []byte
}

type Snapshot struct {
	RequestID       string `json:"request_id"`
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id"`
	APIKeyID        string `json:"api_key_id"`
	ModelCode       string `json:"model_code"`
	CapabilityType  string `json:"capability_type"`
	Protocol        string `json:"protocol"`
	PromptHash      string `json:"prompt_hash"`
	RedactedPreview string `json:"redacted_preview"`
	PromptLength    int    `json:"prompt_length"`
	MessageCount    int    `json:"message_count"`
	ScanText        string `json:"-"`
}

type Result struct {
	Decision          string             `json:"decision"`
	RiskLevel         string             `json:"risk_level"`
	Action            string             `json:"action"`
	Safety            string             `json:"safety"`
	Categories        []string           `json:"categories"`
	MatchedScanners   []string           `json:"matched_scanners"`
	ScannerScores     map[string]float64 `json:"scanner_scores"`
	UnknownCategories []string           `json:"unknown_categories,omitempty"`
	EndpointID        string             `json:"endpoint_id"`
	ScannerVersion    string             `json:"scanner_version"`
	ChunkTotal        int                `json:"chunk_total"`
	LatencyMS         int                `json:"latency_ms"`
}

type Event struct {
	Snapshot       Snapshot
	Result         Result
	ConfigRevision int64
	ErrorCode      string
	CreatedAt      time.Time
}

type StoredEvent struct {
	ID              string             `json:"id"`
	Snapshot        Snapshot           `json:"snapshot"`
	Decision        string             `json:"decision"`
	RiskLevel       string             `json:"risk_level"`
	Action          string             `json:"action"`
	Safety          string             `json:"safety"`
	Categories      []string           `json:"categories"`
	MatchedScanners []string           `json:"matched_scanners"`
	ScannerScores   map[string]float64 `json:"scanner_scores"`
	ScannerVersion  string             `json:"scanner_version"`
	EndpointID      string             `json:"endpoint_id"`
	ConfigRevision  int64              `json:"config_revision"`
	ChunkTotal      int                `json:"chunk_total"`
	LatencyMS       int                `json:"latency_ms"`
	ErrorCode       string             `json:"error_code"`
	CreatedAt       time.Time          `json:"created_at"`
}

type EventFilter struct {
	TenantID string
	UserID   string
	Decision string
	Limit    int32
	Offset   int32
}

type EventPage struct {
	Items []StoredEvent `json:"items"`
	Total int64         `json:"total"`
}

type Runtime struct {
	Mode          string `json:"mode"`
	QueueDepth    int    `json:"queue_depth"`
	QueueCapacity int    `json:"queue_capacity"`
	Submitted     int64  `json:"submitted"`
	Dropped       int64  `json:"dropped"`
	Processed     int64  `json:"processed"`
	Failed        int64  `json:"failed"`
	Allowed       int64  `json:"allowed"`
	Flagged       int64  `json:"flagged"`
	Blocked       int64  `json:"blocked"`
	Unavailable   int64  `json:"unavailable"`
	Invalid       int64  `json:"invalid"`
}

type Decision struct {
	Allow     bool
	ErrorCode string
	Result    *Result
}

func DefaultConfig() Config {
	return Config{
		Mode: ModeOff, WorkerCount: 4, QueueCapacity: 4096,
		Scanners: append([]string(nil), ScannerIDs...), Endpoints: []Endpoint{}, TenantIDs: []string{},
		ConfigRevision: 1,
	}
}
