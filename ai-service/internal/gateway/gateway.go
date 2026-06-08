// Package gateway is the runtime serving plane: the OpenAI/Anthropic/Gemini
// native endpoints under /v1 and /v1beta. It authenticates with API keys,
// drives the serving pipeline, and emits upstream-style (non-enveloped) errors.
package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"xiaodou/unihub/ai-service/internal/apikey"
	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	"xiaodou/unihub/ai-service/internal/serving"
)

// Deps are the runtime-plane dependencies assembled in cmd/server.
type Deps struct {
	Logger      *zap.Logger
	Pipeline    *serving.Pipeline
	Queries     *dbgen.Queries
	APIKeyCache *apikey.Cache // optional; nil disables the key cache
}

// Gateway serves the runtime API. Steps are stateless and shared across
// requests through the serving pipeline.
type Gateway struct {
	logger      *zap.Logger
	queries     *dbgen.Queries
	pipeline    *serving.Pipeline
	apiKeyCache *apikey.Cache
}

func New(deps Deps) *Gateway {
	if deps.Logger == nil {
		panic("gateway: logger is required")
	}
	return &Gateway{
		logger:      deps.Logger,
		queries:     deps.Queries,
		pipeline:    deps.Pipeline,
		apiKeyCache: deps.APIKeyCache,
	}
}

// Routes registers the runtime endpoints on r. The runtimeAuth middleware
// (API-key bearer auth) guards every route.
func (s *Gateway) Routes(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		r.Use(s.runtimeAuth)
		r.Get("/models", s.handleListModels)
		r.Post("/chat/completions", s.handleRuntime(domain.CapabilityChat))
		r.Post("/responses", s.handleRuntime(domain.CapabilityChat)) // Responses API uses chat capability
		r.Post("/embeddings", s.handleRuntime(domain.CapabilityEmbedding))
		r.Post("/images/generations", s.handleRuntime(domain.CapabilityImage))
		r.Post("/images/edits", s.handleRuntime(domain.CapabilityImage))
		r.Post("/messages", s.handleRuntime(domain.CapabilityChat)) // Native Anthropic client path
		r.Post("/messages/count_tokens", s.handleCountTokens)       // Anthropic count_tokens API
	})
	// Native Gemini client endpoints. Chi captures the last URL segment
	// (e.g. "gemini-pro:generateContent") whole; the handler splits on ":" to
	// derive the model name and action. Required so strict 1:1 routing can
	// match gemini_generate / gemini_embeddings deployments without a Chat-API
	// cross-protocol bridge.
	r.Route("/v1beta", func(r chi.Router) {
		r.Use(s.runtimeAuth)
		r.Post("/models/{modelAction}", s.handleGeminiRuntime)
	})
}

// writeJSON encodes v as JSON with the given status. Local to the gateway so
// the runtime plane stays decoupled from the management envelope.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
