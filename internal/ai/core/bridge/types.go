package bridge

import (
	"time"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
)

// IRKind scopes bridge registrations by capability-specific canonical shape.
type IRKind string

const (
	IRKindChat      IRKind = "chat"
	IRKindImage     IRKind = "image"
	IRKindEmbedding IRKind = "embedding"
)

// RequestEnvelope carries normalized metadata needed by a bridge without
// pulling in the legacy runtime request type.
type RequestEnvelope struct {
	Capability  catalog.Capability
	Kind        IRKind
	ClientModel string
	TargetModel string
	Source      surface.ID
	Target      surface.ID
	RequestedAt time.Time
	Metadata    map[string]any
	Variables   map[string]string
	Stream      bool
}

// ResponseEnvelope mirrors the request-side bridge metadata for provider
// response conversion.
type ResponseEnvelope struct {
	Capability catalog.Capability
	Kind       IRKind
	Source     surface.ID
	Target     surface.ID
	Model      string
	ReceivedAt time.Time
	Metadata   map[string]any
	Usage      map[string]any
	StatusCode int
}

// PreparedRequest is the output of request preparation. The body is always set;
// path/content-type overrides are optional and only used when the bridge needs
// to retarget the upstream request envelope.
type PreparedRequest struct {
	Body        []byte
	RequestPath string
	ContentType string
}

// ImageStreamResult is the finalized client-facing SSE body for image-stream
// requests plus the provider sync body reconstructed from the raw upstream
// response for usage/audit extraction.
type ImageStreamResult struct {
	ClientStream []byte
	ProviderBody []byte
}

// ChatIR is the canonical request/response shape for text-style generation.
type ChatIR struct {
	Instructions []string
	Messages     []map[string]any
	Tools        []map[string]any
	ToolChoice   map[string]any
	Options      map[string]any
	Extensions   map[string]any
}

// ImageIR is the canonical request/response shape for image generation/editing.
type ImageIR struct {
	Operation    string
	Prompt       string
	InputImages  []map[string]any
	Mask         map[string]any
	Size         string
	AspectRatio  string
	OutputFormat string
	Count        int
	Options      map[string]any
	Extensions   map[string]any
}

// EmbeddingIR is the canonical request/response shape for embeddings.
type EmbeddingIR struct {
	Input      []any
	Dimensions *int
	Options    map[string]any
	Extensions map[string]any
}

// Definition declares one bridge pair. Concrete implementations will be added
// in later phases when the runtime kernel is switched over.
type Definition struct {
	Kind   IRKind
	Source surface.ID
	Target surface.ID
}
