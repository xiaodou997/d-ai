package runtime

import (
	"context"
	"net/http"

	"xiaodou/dai/internal/ai/core/identity"
)

// ExecutionEnvelope carries the raw transport-layer request/response handles
// needed by runtime execution.
type ExecutionEnvelope struct {
	ResponseWriter http.ResponseWriter
	HTTPRequest    *http.Request
	ClientBody     []byte
}

// ExecutionInput is the complete input to the runtime engine. Route planning is
// deliberately inside the engine so every entrypoint observes the same gates,
// ordering, retry, health and billing semantics.
type ExecutionInput struct {
	Subject  identity.Subject
	Request  Request
	Envelope ExecutionEnvelope
}

// Engine is the single runtime execution seam used by HTTP, run keys, console
// calls and asynchronous replay.
type Engine interface {
	Execute(ctx context.Context, in ExecutionInput) (Result, error)
}
