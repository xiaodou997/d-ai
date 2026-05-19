package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	pgadapter "uni-ai-api/backend/internal/adapters/postgres"
	redisadapter "uni-ai-api/backend/internal/adapters/redis"
	urmadapter "uni-ai-api/backend/internal/adapters/urm"
	"uni-ai-api/backend/internal/apikey"
	"uni-ai-api/backend/internal/config"
	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/observability/metrics"
	"uni-ai-api/backend/internal/routing"
	"uni-ai-api/backend/internal/serving"
	"uni-ai-api/backend/internal/tokenrefresh"
	"uni-ai-api/backend/internal/transport"
	"uni-ai-api/backend/internal/urm"
)

// stickyStoreAdapter adapts redisadapter.RedisSticky to serving.stickyWriter
// without creating an import cycle. RedisSticky already implements routing.StickyStore
// which is the interface used by RouteCandidatesStep. ExecuteStep uses stickyWriter
// (a subset of routing.StickyStore); both are satisfied by *redisadapter.RedisSticky.



type Config struct {
	Server        config.ServerConfig
	Security      config.SecurityConfig
	URM           urmClient
	URMClientID   string
	JWKSValidator jwksValidator
	BanSubscriber banChecker
	Postgres      *pgxpool.Pool
	Redis         *redis.Client
	Logger        *slog.Logger
}

type banChecker interface {
	IsBanned(userID string) bool
}

type urmClient interface {
	Freeze(ctx context.Context, req urm.FreezeRequest) (*urm.FreezeResponse, error)
	Confirm(ctx context.Context, req urm.ConfirmRequest) (*urm.ConfirmResponse, error)
	Cancel(ctx context.Context, transactionID string) error
	ExchangeCode(ctx context.Context, code, redirectURI string) (*urm.TokenPairResponse, error)
}

type jwksValidator interface {
	ValidateToken(ctx context.Context, tokenStr string) (*urm.Claims, error)
}

type Server struct {
	httpServer    *http.Server
	postgres      *pgxpool.Pool
	redis         *redis.Client
	logger        *slog.Logger
	queries       *dbgen.Queries
	security      config.SecurityConfig
	urmClient     urmClient
	urmClientID   string
	jwksValidator jwksValidator
	banSubscriber banChecker
	httpClient    *http.Client
	oauthCreds        *pgadapter.OAuthCredentialStore
	tokenRefresher    *tokenrefresh.Refresher
	routeSelector     *pgadapter.RouteSelector
	routeWeightsStore *pgadapter.RouteWeightsStore
	payloadStore      *pgadapter.PayloadStore // optional; nil when masterKey is empty

	// Serving pipeline — shared across requests (steps are stateless)
	pipeline    *serving.Pipeline
	apiKeyCache *apikey.Cache
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	q := dbgen.New(cfg.Postgres)
	grantChecker := pgadapter.NewModelGrantChecker(q)
	oauthCreds := pgadapter.NewOAuthCredentialStore(cfg.Postgres, cfg.Security.ProviderKeyMaster)
	routeSelector := pgadapter.NewRouteSelector(q, cfg.Postgres, cfg.Security.ProviderKeyMaster, grantChecker).
		WithOAuthCredentialStore(oauthCreds)

	// Build the shared HealthTracker. With Redis: multi-node sync via Pub/Sub.
	// Without Redis: single-node in-memory only.
	innerTracker := routing.DefaultInMemoryTracker()
	var healthTracker routing.HealthTracker = innerTracker
	if cfg.Redis != nil {
		rht := routing.NewRedisHealthTracker(innerTracker, cfg.Redis)
		go rht.Start(context.Background())
		healthTracker = rht
	}
	routeSelector.WithHealth(healthTracker)
	priceResolver := pgadapter.NewPriceResolver(q)
	gw := metrics.NewGateway()

	var rateLimiter serving.RateLimiter
	if cfg.Redis != nil {
		rateLimiter = redisadapter.NewRateLimiter(cfg.Redis, q, 4096)
	}

	var urmBiller serving.URMBiller
	if cfg.URM != nil {
		urmBiller = urmadapter.NewBiller(
			cfg.URM,
			priceResolver,
			pgadapter.CalculateBilling,
			4096,
			cfg.Logger,
		)
	}

	// P3: multi-dim scorer. RouteStats backed by Redis when available; otherwise
	// the scorer degrades to priority+weighted random.
	routeWeightsStore := pgadapter.NewRouteWeightsStore(cfg.Postgres)
	var routeStats routing.RouteStatsStore = routing.NoopRouteStats{}
	if cfg.Redis != nil {
		routeStats = redisadapter.NewRedisRouteStats(cfg.Redis)
	}
	scorer := &serving.MultiDimScorer{
		Health:  healthTracker,
		Stats:   routeStats,
		Weights: routeWeightsStore,
	}

	// P4: Sticky routing backed by Redis (disabled gracefully when Redis is nil).
	var stickyStore routing.StickyStore
	if cfg.Redis != nil {
		stickyStore = redisadapter.NewRedisSticky(cfg.Redis)
	}

	// P4: Payload store for failed-request body persistence.
	var payloadStore *pgadapter.PayloadStore
	if cfg.Security.ProviderKeyMaster != "" {
		payloadStore = pgadapter.NewPayloadStore(cfg.Postgres, cfg.Security.ProviderKeyMaster)
	}

	usageLogger := pgadapter.NewUsageLogger(q)
	if payloadStore != nil {
		usageLogger.SetPayloadStore(payloadStore)
	}

	var apiKeyCache *apikey.Cache
	if cfg.Redis != nil {
		apiKeyCache = apikey.NewCache(cfg.Redis)
	}

	s := &Server{
		postgres:          cfg.Postgres,
		redis:             cfg.Redis,
		logger:            cfg.Logger,
		queries:           q,
		security:          cfg.Security,
		urmClient:         cfg.URM,
		urmClientID:       cfg.URMClientID,
		jwksValidator:     cfg.JWKSValidator,
		banSubscriber:     cfg.BanSubscriber,
		oauthCreds:        oauthCreds,
		tokenRefresher:    tokenrefresh.New(oauthCreds, cfg.Logger),
		routeSelector:     routeSelector,
		routeWeightsStore: routeWeightsStore,
		payloadStore:      payloadStore,
		httpClient: &http.Client{
			Timeout: 0,
		},
		pipeline: serving.NewPipeline(
			&serving.AuthNStep{Resolver: pgadapter.NewAPIKeyResolver(q)},
			&serving.AuthZStep{Checker: grantChecker},
			&serving.QuotaCheckStep{},
			&serving.RouteCandidatesStep{Selector: routeSelector, Sticky: stickyStore},
			&serving.RateLimitStep{Limiter: rateLimiter},
			&serving.QuotaReserveStep{Reserver: pgadapter.NewQuotaReserver(q)},
			&serving.URMFreezeStep{Biller: urmBiller},
			&serving.ExecuteStep{
				Transport: transport.NewClient(120 * time.Second),
				Health:    healthTracker,
				OAuthPool: oauthCreds,
				Budget:    serving.DefaultRetryBudget(),
				Scorer:    scorer,
				Stats:     routeStats,
				Sticky:    stickyStore,
			},
			&serving.URMConfirmStep{Biller: urmBiller},
			&serving.UsageLogStep{Logger: usageLogger, Metrics: gw},
		),
		apiKeyCache: apiKeyCache,
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(s.requestLogger)

	router.Get("/health", s.handleHealth)
	router.Get("/ready", s.handleReady)
	router.Get("/metrics", metrics.Handler().ServeHTTP)
	router.Get("/api/auth/callback", s.handleAuthCallback)
	router.Route("/api/v1", func(r chi.Router) {
		// chi's Timeout middleware swaps the ResponseWriter for a buffered one
		// that cancels the request after the deadline. It MUST NOT wrap the
		// /v1/* runtime routes — long-running SSE streams would be killed
		// mid-flight. Mounting it only on management /api/v1 is safe because
		// those endpoints return JSON within a bounded time.
		r.Use(middleware.Timeout(120 * time.Second))
		r.Use(s.apiAuth)
		r.Use(s.adminAudit)
		// =============================================================
		// 平台级资源（仅 platform 角色可访问）
		// =============================================================
		r.Get("/providers", s.handleAdminListProviders)
		r.Post("/providers", s.handleAdminCreateProvider)
		r.Patch("/providers/{providerID}", s.handleAdminUpdateProvider)
		r.Patch("/providers/{providerID}/status", s.handleAdminUpdateProviderStatus)
		r.Get("/providers/{providerID}/endpoints", s.handleAdminListProviderEndpoints)
		r.Post("/providers/{providerID}/endpoints", s.handleAdminCreateProviderEndpoint)
		r.Patch("/providers/{providerID}/endpoints/{endpointID}", s.handleAdminUpdateProviderEndpoint)
		r.Patch("/providers/{providerID}/endpoints/{endpointID}/status", s.handleAdminUpdateProviderEndpointStatus)
		r.Delete("/providers/{providerID}/endpoints/{endpointID}", s.handleAdminDeleteProviderEndpoint)
		r.Get("/providers/{providerID}/endpoints/{endpointID}/upstream-models", s.handleAdminFetchEndpointUpstreamModels)
		r.Post("/providers/{providerID}/endpoints/{endpointID}/import-upstream-models", s.handleAdminImportEndpointUpstreamModels)
		r.Get("/upstream-deployments", s.handleAdminListUpstreamDeployments)
		r.Post("/upstream-deployments", s.handleAdminCreateUpstreamDeployment)
		r.Get("/upstream-deployments/{deploymentID}", s.handleAdminGetUpstreamDeployment)
		r.Patch("/upstream-deployments/{deploymentID}", s.handleAdminUpdateUpstreamDeployment)
		r.Patch("/upstream-deployments/{deploymentID}/status", s.handleAdminUpdateUpstreamDeploymentStatus)
		r.Delete("/upstream-deployments/{deploymentID}", s.handleAdminDeleteUpstreamDeployment)
		r.Post("/upstream-deployments/{deploymentID}/health-check", s.handleAdminCheckUpstreamDeploymentHealth)
		r.Get("/models", s.handleAdminListModels)
		r.Post("/models", s.handleAdminCreateModel)
		r.Patch("/models/{modelID}", s.handleAdminUpdateModel)
		r.Patch("/models/{modelID}/status", s.handleAdminUpdateModelStatus)
		r.Get("/models/{modelID}/price", s.handleAdminGetModelPrice)
		r.Put("/models/{modelID}/price", s.handleAdminUpsertModelPrice)
		r.Get("/models/{modelID}/routes", s.handleAdminListModelRoutes)
		r.Post("/models/{modelID}/routes", s.handleAdminCreateModelRoute)
		r.Get("/models/{modelID}/routes/{routeID}", s.handleAdminGetModelRoute)
		r.Patch("/models/{modelID}/routes/{routeID}", s.handleAdminUpdateModelRoute)
		r.Patch("/models/{modelID}/routes/{routeID}/status", s.handleAdminUpdateModelRouteStatus)
		r.Delete("/models/{modelID}/routes/{routeID}", s.handleAdminDeleteModelRoute)
		r.Get("/limit-policies", s.handleAdminListRuntimeLimitPolicies)
		r.Post("/limit-policies", s.handleAdminCreateRuntimeLimitPolicy)
		r.Patch("/limit-policies/{policyID}", s.handleAdminUpdateRuntimeLimitPolicy)
		r.Patch("/limit-policies/{policyID}/status", s.handleAdminUpdateRuntimeLimitPolicyStatus)
		r.Get("/audit-logs", s.handleAdminListAuditLogs)

		// OAuth credential pool health (aggregate, all pools)
		r.Get("/oauth-pool-health", s.handleAdminGetOAuthPoolHealth)

		// System status: DB/Redis health + circuit breaker snapshot
		r.Get("/system/status", s.handleAdminSystemStatus)

		// P3: route scorer weight config
		r.Get("/route-weights/{scope}", s.handleAdminGetRouteWeights)
		r.Put("/route-weights/{scope}", s.handleAdminPutRouteWeights)

		// P4: request payload replay
		r.Post("/usage-logs/{id}/replay", s.handleReplay)

		// Credential pool management
		r.Get("/credential-pools", s.handleAdminListCredentialPools)
		r.Post("/credential-pools", s.handleAdminCreateCredentialPool)
		r.Patch("/credential-pools/{poolID}", s.handleAdminPatchCredentialPool)
		r.Delete("/credential-pools/{poolID}", s.handleAdminDeleteCredentialPool)
		r.Get("/credential-pools/{poolID}/credentials", s.handleAdminListPoolCredentials)
		r.Post("/credential-pools/{poolID}/credentials", s.handleAdminCreatePoolCredential)
		r.Patch("/credential-pools/{poolID}/credentials/{credID}", s.handleAdminPatchPoolCredential)
		r.Delete("/credential-pools/{poolID}/credentials/{credID}", s.handleAdminDeletePoolCredential)
		r.Post("/credential-pools/{poolID}/credentials/{credID}/refresh", s.handleAdminRefreshPoolCredential)
		r.Get("/credential-pools/{poolID}/available-models", s.handleAdminGetPoolAvailableModels)

		// =============================================================
		// 平台管理指定租户（platform 角色专用）
		// =============================================================
		r.Get("/tenants/{tenantID}/model-grants", s.handleAdminListTenantModelGrants)
		r.Post("/tenants/{tenantID}/model-grants", s.handleAdminGrantModelToTenant)
		r.Patch("/tenants/{tenantID}/model-grants/{modelID}/status", s.handleAdminUpdateTenantModelGrantStatus)
		r.Get("/tenants/{tenantID}/model-price-overrides", s.handleAdminListTenantModelPriceOverrides)
		r.Get("/tenants/{tenantID}/model-price-overrides/{modelID}", s.handleAdminGetTenantModelPriceOverride)
		r.Put("/tenants/{tenantID}/model-price-overrides/{modelID}", s.handleAdminUpsertTenantModelPriceOverride)
		r.Delete("/tenants/{tenantID}/model-price-overrides/{modelID}", s.handleAdminDeleteTenantModelPriceOverride)
		r.Get("/tenants/{tenantID}/user-prices", s.handleAdminListTenantUserPrices)
		r.Get("/tenants/{tenantID}/user-prices/{modelID}", s.handleAdminGetTenantUserPrice)
		r.Put("/tenants/{tenantID}/user-prices/{modelID}", s.handleAdminUpsertTenantUserPrice)
		r.Delete("/tenants/{tenantID}/user-prices/{modelID}", s.handleAdminDeleteTenantUserPrice)
		r.Get("/tenants/{tenantID}/api-keys", s.handleAdminListTenantAPIKeys)
		r.Post("/tenants/{tenantID}/api-keys", s.handleAdminCreateTenantAPIKey)
		r.Patch("/tenants/{tenantID}/api-keys/{apiKeyID}", s.handleAdminUpdateTenantAPIKey)
		r.Patch("/tenants/{tenantID}/api-keys/{apiKeyID}/status", s.handleAdminUpdateTenantAPIKeyStatus)
		r.Post("/tenants/{tenantID}/api-keys/{apiKeyID}/rotate", s.handleAdminRotateTenantAPIKey)
		r.Delete("/tenants/{tenantID}/api-keys/{apiKeyID}", s.handleAdminDeleteTenantAPIKey)
		r.Get("/tenants/{tenantID}/users/{userID}/api-keys", s.handleAdminListUserAPIKeys)
		r.Post("/tenants/{tenantID}/users/{userID}/api-keys", s.handleAdminCreateUserAPIKey)
		r.Patch("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}", s.handleAdminUpdateUserAPIKey)
		r.Patch("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}/status", s.handleAdminUpdateUserAPIKeyStatus)
		r.Post("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}/rotate", s.handleAdminRotateUserAPIKey)
		r.Delete("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}", s.handleAdminDeleteUserAPIKey)

		// =============================================================
		// 租户自管理（/tenants/me/* 路由）
		// =============================================================
		r.Post("/tenants/me/api-keys", s.handleTenantsMeAPIKeysCreate)
		r.Patch("/tenants/me/api-keys/{apiKeyID}", s.handleTenantsMeAPIKeysUpdate)
		r.Patch("/tenants/me/api-keys/{apiKeyID}/status", s.handleTenantsMeAPIKeysStatus)
		r.Post("/tenants/me/api-keys/{apiKeyID}/rotate", s.handleTenantsMeAPIKeysRotate)
		r.Delete("/tenants/me/api-keys/{apiKeyID}", s.handleTenantsMeAPIKeysDelete)
		r.Put("/tenants/me/user-prices/{modelID}", s.handleTenantsMeUserPricesUpsert)
		r.Delete("/tenants/me/user-prices/{modelID}", s.handleTenantsMeUserPricesDelete)

		// =============================================================
		// 用户自管理（/users/me/* 路由）
		// =============================================================
		r.Post("/users/me/api-keys", s.handleUsersMeAPIKeysCreate)
		r.Patch("/users/me/api-keys/{apiKeyID}", s.handleUsersMeAPIKeysUpdate)
		r.Patch("/users/me/api-keys/{apiKeyID}/status", s.handleUsersMeAPIKeysStatus)
		r.Post("/users/me/api-keys/{apiKeyID}/rotate", s.handleUsersMeAPIKeysRotate)
		r.Delete("/users/me/api-keys/{apiKeyID}", s.handleUsersMeAPIKeysDelete)

		// =============================================================
		// 三角色共享（根据 token 自动过滤数据范围）
		// =============================================================
		r.Get("/tenant-api-keys", s.handleTenantAPIKeysSelf)
		r.Get("/tenant-model-grants", s.handleTenantModelGrantsSelf)
		r.Get("/user-api-keys", s.handleUserAPIKeysSelf)
		r.Get("/user-model-grants", s.handleUserModelGrantsSelf)
		r.Get("/dashboard/summary", s.handleDashboardSummaryByRole)
		r.Get("/dashboard/top-models", s.handleAdminDashboardTopModels)
		r.Get("/dashboard/top-tenants", s.handleAdminDashboardTopTenants)
		r.Get("/dashboard/recent-errors", s.handleAdminDashboardRecentErrors)
		r.Get("/usage-logs", s.handleUsageLogsByRole)
		r.Get("/usage-summary", s.handleAdminListUsageSummary)
		r.Get("/usage-unit-summary", s.handleAdminListUsageUnitSummary)
		r.Get("/analytics/daily-trend", s.handleAdminListDailyTrend)

		// =============================================================
		// 租户专用（扁平路径，自动过滤）
		// =============================================================
		r.Get("/user-prices", s.handleUserPricesSelf)

		// =============================================================
		// 用户专用（扁平路径，自动过滤）
		// =============================================================
		r.Get("/user-usage-logs", s.handleUserUsageLogsSelf)
		r.Get("/user-usage-summary", s.handleUserUsageSummarySelf)
	})
	router.Route("/v1", func(r chi.Router) {
		r.Use(s.runtimeAuth)
		r.Get("/models", s.handleListModels)
		r.Post("/chat/completions", s.handleRuntime(domain.CapabilityChat))
		r.Post("/responses", s.handleRuntime(domain.CapabilityChat)) // Responses API uses chat capability
		r.Post("/embeddings", s.handleRuntime(domain.CapabilityEmbedding))
		r.Post("/images/generations", s.handleRuntime(domain.CapabilityImage))
		r.Post("/messages", s.handleRuntime(domain.CapabilityChat)) // Native Anthropic client path
		r.Post("/messages/count_tokens", s.handleCountTokens)       // Anthropic count_tokens API
	})

	s.httpServer = &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}

	return s
}

func (s *Server) Start(addr string) error {
	if s.payloadStore != nil {
		s.payloadStore.StartCleanupJob(context.Background())
	}
	s.httpServer.Addr = addr
	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		ctx, logCtx := withRequestLogContext(r.Context())
		r = r.WithContext(ctx)
		requestID := middleware.GetReqID(ctx)
		if requestID != "" {
			w.Header().Set("X-Request-ID", requestID)
		}

		defer func() {
			if recovered := recover(); recovered != nil {
				if ww.Status() == 0 {
					http.Error(ww, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
				s.logger.Error("http request panic",
					"error", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
				)
			}

			elapsed := time.Since(start)
			status := responseStatus(ww)
			routePath := routePattern(r)

			attrs := []any{
				"method", r.Method,
				"path", routePath,
				"raw_path", r.URL.Path,
				"status", status,
				"bytes", ww.BytesWritten(),
				"duration_ms", elapsed.Milliseconds(),
				"request_id", requestID,
				"remote_ip", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			}
			if logCtx.TenantID != "" {
				attrs = append(attrs, "tenant_id", logCtx.TenantID)
			}
			if logCtx.UserID != "" {
				attrs = append(attrs, "user_id", logCtx.UserID)
			}
			if logCtx.Role != "" {
				attrs = append(attrs, "role", logCtx.Role)
			}
			if logCtx.APIKeyIDHash != "" {
				attrs = append(attrs, "api_key_id_hash", logCtx.APIKeyIDHash)
			}

			s.logger.Info("http request", attrs...)
		}()

		next.ServeHTTP(ww, r)
	})
}

func responseStatus(w middleware.WrapResponseWriter) int {
	if w.Status() == 0 {
		return http.StatusOK
	}
	return w.Status()
}

func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return r.URL.Path
}
