package catalog

import (
	"time"

	"xiaodou/dai/internal/ai/core/surface"
)

// Capability is the primary business capability of an AI model or request.
type Capability string

const (
	CapabilityChat            Capability = "chat"
	CapabilityEmbedding       Capability = "embedding"
	CapabilityImageGeneration Capability = "image_generation"
	CapabilityImageEdit       Capability = "image_edit"
	CapabilityAudioTTS        Capability = "audio_tts"
	CapabilityAudioSTT        Capability = "audio_stt"
	CapabilityVideoGeneration Capability = "video_generation"
	CapabilityWorkflow        Capability = "workflow"
)

// ProviderFamily describes the vendor family a model naturally belongs to.
type ProviderFamily string

const (
	ProviderFamilyOpenAICompatible ProviderFamily = "openai_compatible"
	ProviderFamilyAnthropic        ProviderFamily = "anthropic"
	ProviderFamilyGoogle           ProviderFamily = "google"
	ProviderFamilyOther            ProviderFamily = "other"
)

// Status is reused across catalog entities during the rebuild.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// Model is the catalog-first internal model definition.
type Model struct {
	ID                    string
	Code                  string
	DisplayName           string
	Description           string
	DefaultProviderFamily ProviderFamily
	Capabilities          []Capability
	Metadata              map[string]any
	Status                Status
	SortOrder             int
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ModelSurfaceAudience indicates whether a surface is exposed to clients or
// used only for upstream matching.
type ModelSurfaceAudience string

const (
	ModelSurfaceAudienceClient   ModelSurfaceAudience = "client"
	ModelSurfaceAudienceUpstream ModelSurfaceAudience = "upstream"
)

// ModelSurface declares an allowed surface for a catalog model.
type ModelSurface struct {
	ID        string
	ModelID   string
	Audience  ModelSurfaceAudience
	Surface   surface.ID
	IsDefault bool
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}
