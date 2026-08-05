// Package console serves the web runtime plane and OAuth callback on the
// canonical /runtime/v1/* surface.
package console

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/apikey"
	"xiaodou/dai/internal/ai/asynctask"
	"xiaodou/dai/internal/config"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/filestore"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/tokenrefresh"
	"xiaodou/dai/internal/ai/urm"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
)

// URMExchanger exchanges an OAuth2 authorization code for a token pair.
type URMExchanger interface {
	ExchangeCode(ctx context.Context, code, redirectURI string) (*urm.TokenPairResponse, error)
}

// JWKSValidator validates a URM-issued JWT and returns its claims.
type JWKSValidator interface {
	ValidateToken(ctx context.Context, tokenStr string) (*urm.Claims, error)
}

// BanChecker reports whether a user or tenant has been banned (real-time revocation).
type BanChecker interface {
	IsBanned(ctx context.Context, userID string) (bool, error)
	IsTenantBanned(ctx context.Context, tenantID string) (bool, error)
}

type runtimeGateway interface {
	Replay(context.Context, gateway.ReplayInput) gateway.ReplayResult
	ExecuteRuntime(http.ResponseWriter, *http.Request, domain.CapabilityType, gateway.RuntimeOverride, *coreidentity.Subject, bool) coreruntime.Result
}

// Deps are the console-plane dependencies assembled in cmd/server.
type Deps struct {
	Postgres       *pgxpool.Pool
	Redis          *redis.Client
	Logger         *zap.Logger
	Queries        *dbgen.Queries
	Security       config.SecurityConfig
	URMClient      URMExchanger
	URMClientID    string
	JWKSValidator  JWKSValidator
	BanChecker     BanChecker
	HTTPClient     *http.Client
	OAuthCreds     *pgadapter.OAuthCredentialStore
	TokenRefresher *tokenrefresh.Refresher
	RouteInspector *pgadapter.RouteInspector
	APIKeyCache    *apikey.Cache
	Gateway        *gateway.Gateway // runtime plane, driven by web runtime chat/image
	GrantChecker   *pgadapter.GroupAccessReader
	WorkspaceSvc   *workspacesvc.Service
	ImageAssets    *imageassets.Service
	FileStore      *filestore.Service
	AsyncTasks     config.AsyncTaskConfig
}

// Console serves the web runtime endpoints + OAuth callback.
type Console struct {
	postgres        *pgxpool.Pool
	redis           *redis.Client
	logger          *zap.Logger
	queries         *dbgen.Queries
	security        config.SecurityConfig
	urmClient       URMExchanger
	urmClientID     string
	jwksValidator   JWKSValidator
	banChecker      BanChecker
	httpClient      *http.Client
	oauthCreds      *pgadapter.OAuthCredentialStore
	tokenRefresher  *tokenrefresh.Refresher
	routeInspector  *pgadapter.RouteInspector
	apiKeyCache     *apikey.Cache
	gateway         runtimeGateway
	grantChecker    *pgadapter.GroupAccessReader
	workspaceSvc    *workspacesvc.Service
	imageAssets     *imageassets.Service
	fileStore       *filestore.Service
	asyncTaskConfig config.AsyncTaskConfig
	asyncTasks      *asynctask.Engine
}

func New(deps Deps) *Console {
	if deps.Logger == nil {
		panic("console: logger is required")
	}
	c := &Console{
		postgres:        deps.Postgres,
		redis:           deps.Redis,
		logger:          deps.Logger,
		queries:         deps.Queries,
		security:        deps.Security,
		urmClient:       deps.URMClient,
		urmClientID:     deps.URMClientID,
		jwksValidator:   deps.JWKSValidator,
		banChecker:      deps.BanChecker,
		httpClient:      deps.HTTPClient,
		oauthCreds:      deps.OAuthCreds,
		tokenRefresher:  deps.TokenRefresher,
		routeInspector:  deps.RouteInspector,
		apiKeyCache:     deps.APIKeyCache,
		gateway:         deps.Gateway,
		grantChecker:    deps.GrantChecker,
		workspaceSvc:    deps.WorkspaceSvc,
		imageAssets:     deps.ImageAssets,
		fileStore:       deps.FileStore,
		asyncTaskConfig: deps.AsyncTasks,
	}
	return c
}

// recoverer turns a panic in a console handler into a standard error envelope.
func (s *Console) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				fields := []zap.Field{
					zap.Any("error", rec),
					zap.Stack("stack"),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				}
				fields = append(fields, consoleLogFields(r.Context())...)
				s.logger.Error("runtime request panic",
					fields...,
				)
				writeErr(w, http.StatusInternalServerError, BizErrInternal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Routes registers the web runtime endpoints on r.
func (s *Console) Routes(r chi.Router) {
	r.With(s.recoverer).Get("/api/auth/callback", s.handleAuthCallback)

	r.Route("/runtime/v1", s.registerRuntimeRoutes)
}

func (s *Console) registerRuntimeRoutes(r chi.Router) {
	// Auth is performed inside each handler (consoleRuntimeSubject) because it
	// speaks the runtime protocol and shares the pipeline with chat/image serving.
	r.Use(s.recoverer)
	r.Post("/chat/sessions/{sessionID}/messages:stream", s.handleConsoleChatStream)
	r.Post("/app-previews/{agentID}", s.handleConsoleAppPreview)
	r.Get("/tasks", s.handlePortalTaskList)
	r.Get("/tasks/{taskID}", s.handlePortalTaskGet)
	r.Post("/tasks/{taskID}/cancel", s.handlePortalTaskCancel)
	r.Delete("/tasks/{taskID}", s.handlePortalTaskDelete)
	r.Get("/images/models", s.handleConsoleImageModels)
	r.Post("/images/tasks", s.handleConsoleImageCreateTask)
	r.Get("/images/tasks/{taskID}/assets/{index}/{variant}", s.handleConsoleImageTaskAsset)
	r.Get("/images/assets/{assetID}", s.handleConsoleImageEphemeralAsset)
	r.Get("/images/tasks/{taskID}", s.handleConsoleImageGetTask)
	r.Post("/images/tasks/{taskID}/cancel", s.handleConsoleImageCancelTask)
	r.Delete("/images/tasks/{taskID}", s.handleConsoleImageDeleteTask)
}

// writeJSON encodes v as JSON with the given status. Local to the console.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func requestIDFromContext(ctx context.Context) string {
	return middleware.GetReqID(ctx)
}
