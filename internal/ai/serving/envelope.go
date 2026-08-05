package serving

import (
	"net/http"

	"xiaodou/dai/internal/ai/domain"
)

// RequestEnvelope carries the HTTP transport layer (ResponseWriter + Request)
// outside of the pipeline Request struct. It is owned by the HTTP handler and
// passed through to the ExecuteStep as a separate input — this keeps the
// pipeline Request a pure data carrier and allows retries / fan-out
// experiments to operate on Request without touching the wire.
//
// Execute step reads ClientProtocol to choose error-envelope shape and
// reads IsStream to dispatch sync vs streaming write paths.
type RequestEnvelope struct {
	W              http.ResponseWriter
	R              *http.Request
	ClientProtocol domain.UpstreamProtocol
	IsStream       bool
	// ClientBody holds the raw request body bytes. Set by the HTTP handler and
	// used by the payload store to persist the original request for replay.
	ClientBody []byte
}

// Flusher returns the response writer's flush function, or a no-op if the
// underlying writer does not support flushing. Used by streaming relays.
func (e *RequestEnvelope) Flusher() func() {
	if f, ok := e.W.(http.Flusher); ok {
		return f.Flush
	}
	return func() {}
}
