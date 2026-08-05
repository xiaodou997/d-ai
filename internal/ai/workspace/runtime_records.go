package workspace

import (
	"time"

	"xiaodou/dai/internal/ai/core/identity"
)

type Owner struct {
	Scope    identity.Scope
	TenantID string
	UserID   string
}

type ChatSession struct {
	ID                      string
	Title                   string
	TargetType              string
	AgentID                 string
	AgentName               string
	Variables               map[string]string
	ModelCode               string
	GroupID                 string
	GroupName               string
	EffectiveUserMultiplier float64
	BillingGroupLabel       string
	SelectedProtocol        string
	SelectedRouteID         string
	Status                  string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type ChatMessage struct {
	ID        string
	Role      string
	Content   string
	Protocol  string
	RouteID   string
	Usage     map[string]any
	Error     map[string]any
	CreatedAt time.Time
}

type ImageJob struct {
	ID                   string
	Operation            string
	AgentID              string
	AgentName            string
	ModelCode            string
	Prompt               string
	Status               string
	StoragePolicy        string
	RawImageRetained     bool
	Size                 string
	Quality              string
	Style                string
	ResponseFormat       string
	RequestedOutputCount int
	CallerChargeMicro    int64
	ImageCount           int
	InlineCount          int
	URLCount             int
	RevisedPrompts       []string
	Assets               []ImageAsset
	ErrorMessage         string
	CreatedAt            time.Time
	CompletedAt          *time.Time
}

type ImageAsset struct {
	ID                  string `json:"id,omitempty"`
	Index               int    `json:"index,omitempty"`
	PreviewURL          string `json:"preview_url,omitempty"`
	DisplayURL          string `json:"display_url"`
	OriginalURL         string `json:"original_url,omitempty"`
	OriginalContentType string `json:"content_type,omitempty"`
	OriginalSizeBytes   int64  `json:"size_bytes,omitempty"`
	PreviewContentType  string `json:"preview_content_type,omitempty"`
	PreviewSizeBytes    int64  `json:"preview_size_bytes,omitempty"`
	Width               int    `json:"width,omitempty"`
	Height              int    `json:"height,omitempty"`
	ExpiresAt           int64  `json:"expires_at,omitempty"`
}

type Overview struct {
	RecentChatSessions []ChatSession
	RecentImageJobs    []ImageJob
}

type ChatModel struct {
	GroupID                 string
	GroupName               string
	EffectiveUserMultiplier float64
	BillingGroupLabel       string
	ModelCode               string
	CapabilityType          string
	DefaultProtocol         string
	AvailableProtocols      []string
	SupportsStream          bool
	Status                  string
}

type CreateChatSessionInput struct {
	Title     string
	ModelCode string
	GroupID   string
}
