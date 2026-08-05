// Package gateway is the runtime serving plane: the OpenAI/Anthropic/Gemini
// native endpoints under /v1 and /v1beta, plus explicit versionless OpenAI
// compatibility aliases. It authenticates with API keys, drives the serving
// pipeline, and emits upstream-style (non-enveloped) errors.
package gateway

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/apikey"
	"xiaodou/dai/internal/ai/asynctask"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

type runRuntimeInvokeExpander interface {
	ExpandByKeyHash(ctx context.Context, keyHash string, req coreruntime.Request) (coreruntime.InvokeExpansion, error)
}

// BanChecker reports whether a user or tenant is currently banned/disabled in
// URM. Implemented by *banstate.Checker (direct Redis key read; see
// libs/go/banstate).
type BanChecker interface {
	IsBanned(ctx context.Context, userID string) (bool, error)
	IsTenantBanned(ctx context.Context, tenantID string) (bool, error)
}

// Deps are the runtime-plane dependencies assembled in cmd/server.
type Deps struct {
	Logger        *zap.Logger
	Postgres      *pgxpool.Pool
	Pipeline      *serving.Pipeline
	Queries       *dbgen.Queries
	APIKeyCache   *apikey.Cache // optional; nil disables the key cache
	BanChecker    BanChecker    // optional; nil disables ban enforcement
	ServiceAccess interface {
		Check(context.Context, int, string, string, string, string) error
	}
	ExpectedClientID      string
	RuntimeInvokeExpander runRuntimeInvokeExpander
	RuntimeEngine         coreruntime.Engine
	AsyncTasks            *asynctask.Engine
	TaskAdmission         *serving.AdmissionGate
	TaskInvokeExpander    taskRuntimeInvokeExpander

	// JWKSValidator 校验 URM 委托 Token（内部 OBO 任务入口用）；nil 则禁用委托入口。
	JWKSValidator delegatedTokenValidator
	// DelegationAllowedClients 允许经 OBO 调用内部任务入口的服务 client_id 白名单。
	DelegationAllowedClients []string
}

// Gateway serves the runtime API. Steps are stateless and shared across
// requests through the serving pipeline.
type Gateway struct {
	logger        *zap.Logger
	postgres      *pgxpool.Pool
	queries       *dbgen.Queries
	pipeline      *serving.Pipeline
	apiKeyCache   *apikey.Cache
	banChecker    BanChecker
	serviceAccess interface {
		Check(context.Context, int, string, string, string, string) error
	}
	expectedClientID      string
	runtimeInvokeExpander runRuntimeInvokeExpander
	runtimeEngine         coreruntime.Engine
	asyncTasks            *asynctask.Engine
	taskAdmission         *serving.AdmissionGate
	taskInvokeExpander    taskRuntimeInvokeExpander

	jwksValidator            delegatedTokenValidator
	delegationAllowedClients []string
}

func New(deps Deps) *Gateway {
	if deps.Logger == nil {
		panic("gateway: logger is required")
	}
	return &Gateway{
		logger:                   deps.Logger,
		postgres:                 deps.Postgres,
		queries:                  deps.Queries,
		pipeline:                 deps.Pipeline,
		apiKeyCache:              deps.APIKeyCache,
		banChecker:               deps.BanChecker,
		serviceAccess:            deps.ServiceAccess,
		expectedClientID:         deps.ExpectedClientID,
		runtimeInvokeExpander:    deps.RuntimeInvokeExpander,
		runtimeEngine:            deps.RuntimeEngine,
		asyncTasks:               deps.AsyncTasks,
		taskAdmission:            deps.TaskAdmission,
		taskInvokeExpander:       deps.TaskInvokeExpander,
		jwksValidator:            deps.JWKSValidator,
		delegationAllowedClients: deps.DelegationAllowedClients,
	}
}

// Routes registers the runtime endpoints on r. The runtimeAuth middleware
// accepts OpenAI Bearer and Anthropic x-api-key authentication on every route.
func (s *Gateway) Routes(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		r.Post("/run", s.handleRunRuntime)
		r.Post("/tasks", s.handleCreateTask)
		r.Get("/tasks", s.handleListTasks)
		r.Get("/tasks/{taskID}", s.handleGetTask)
		r.Post("/tasks/{taskID}/cancel", s.handleCancelTask)
		r.Group(func(r chi.Router) {
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
	})
	// Some OpenAI-compatible clients append endpoint names directly to the
	// configured base URL. Keep the public base URL versionless for those
	// clients while presenting one canonical /v1 path to the runtime pipeline.
	r.Group(func(r chi.Router) {
		r.Use(s.runtimeAuth)
		r.With(canonicalRuntimePath("/v1/models")).Get("/models", s.handleListModels)
		r.With(canonicalRuntimePath("/v1/chat/completions")).Post("/chat/completions", s.handleRuntime(domain.CapabilityChat))
		r.With(canonicalRuntimePath("/v1/responses")).Post("/responses", s.handleRuntime(domain.CapabilityChat))
		r.With(canonicalRuntimePath("/v1/embeddings")).Post("/embeddings", s.handleRuntime(domain.CapabilityEmbedding))
		r.With(canonicalRuntimePath("/v1/images/generations")).Post("/images/generations", s.handleRuntime(domain.CapabilityImage))
		r.With(canonicalRuntimePath("/v1/images/edits")).Post("/images/edits", s.handleRuntime(domain.CapabilityImage))
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

func canonicalRuntimePath(path string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			canonicalRequest := r.Clone(r.Context())
			canonicalURL := *r.URL
			canonicalURL.Path = path
			canonicalURL.RawPath = ""
			canonicalRequest.URL = &canonicalURL
			next.ServeHTTP(w, canonicalRequest)
		})
	}
}

// rejectIfBanned checks tenantID (always) and userID (when non-empty)
// against s.banChecker, tenant first, and writes the matching 403 response.
// Returns true if it wrote a response (caller must stop handling the
// request). No-op (returns false) when banChecker is nil.
func (s *Gateway) rejectIfBanned(w http.ResponseWriter, ctx context.Context, tenantID, userID string) bool {
	if s.banChecker == nil {
		return false
	}
	if tenantID != "" {
		banned, err := s.banChecker.IsTenantBanned(ctx, tenantID)
		if err != nil {
			s.logger.Warn("runtime tenant ban check failed",
				gatewayLogFields(ctx, tenantID, userID, zap.Error(err))...,
			)
			writeOpenAIError(w, http.StatusServiceUnavailable, "Unable to verify account status.", "service_unavailable", "service_unavailable")
			return true
		}
		if banned {
			writeOpenAIError(w, http.StatusForbidden, "Tenant is disabled.", "invalid_request_error", "tenant_banned")
			return true
		}
	}
	if userID != "" {
		banned, err := s.banChecker.IsBanned(ctx, userID)
		if err != nil {
			s.logger.Warn("runtime user ban check failed",
				gatewayLogFields(ctx, tenantID, userID, zap.Error(err))...,
			)
			writeOpenAIError(w, http.StatusServiceUnavailable, "Unable to verify account status.", "service_unavailable", "service_unavailable")
			return true
		}
		if banned {
			writeOpenAIError(w, http.StatusForbidden, "Account is disabled.", "invalid_request_error", "account_banned")
			return true
		}
	}
	return false
}

// writeJSON encodes v as JSON with the given status. Local to the gateway so
// the runtime plane stays decoupled from the management envelope.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
