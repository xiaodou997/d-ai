package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	pgadapter "xiaodou/uni-ai-api/internal/adapters/postgres"
	redisadapter "xiaodou/uni-ai-api/internal/adapters/redis"
	"xiaodou/uni-ai-api/internal/apikey"
	"xiaodou/uni-ai-api/internal/audit"
	"xiaodou/uni-ai-api/internal/blobstore"
	"xiaodou/uni-ai-api/internal/config"
	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/ledger"
	"xiaodou/uni-ai-api/internal/observability/metrics"
	"xiaodou/uni-ai-api/internal/routing"
	"xiaodou/uni-ai-api/internal/serving"
	"xiaodou/uni-ai-api/internal/tokenrefresh"
	"xiaodou/uni-ai-api/internal/transport"
	"xiaodou/uni-ai-api/internal/urm"
)

// stickyStoreAdapter adapts redisadapter.RedisSticky to serving.stickyWriter
// without creating an import cycle. RedisSticky already implements routing.StickyStore
// which is the interface used by RouteCandidatesStep. ExecuteStep uses stickyWriter
// (a subset of routing.StickyStore); both are satisfied by *redisadapter.RedisSticky.

type Config struct {
	Server        config.ServerConfig
	Security      config.SecurityConfig
	Serving       config.ServingConfig
	URM           urmClient
	URMClientID   string
	JWKSValidator jwksValidator
	BanSubscriber banChecker
	Postgres      *pgxpool.Pool
	Redis         *redis.Client
	Logger        *zap.Logger
}

type banChecker interface {
	IsBanned(userID string) bool
}

type urmClient interface {
	// Consume 单阶段聚合扣款：Phase 3 起 ai-service 唯一调用的 URM 计费 API。
	// 由本地账本 settle worker 在窗口结算时调用。
	Consume(ctx context.Context, req urm.ConsumeRequest) (*urm.ConsumeResponse, error)
	ExchangeCode(ctx context.Context, code, redirectURI string) (*urm.TokenPairResponse, error)
}

type jwksValidator interface {
	ValidateToken(ctx context.Context, tokenStr string) (*urm.Claims, error)
}

type Server struct {
	httpServer        *http.Server
	postgres          *pgxpool.Pool
	redis             *redis.Client
	logger            *zap.Logger
	queries           *dbgen.Queries
	security          config.SecurityConfig
	urmClient         urmClient
	urmClientID       string
	jwksValidator     jwksValidator
	banSubscriber     banChecker
	httpClient        *http.Client
	oauthCreds        *pgadapter.OAuthCredentialStore
	tokenRefresher    *tokenrefresh.Refresher
	routeSelector     *pgadapter.RouteSelector
	routeWeightsStore *pgadapter.RouteWeightsStore
	auditWorker       *audit.Worker // optional; nil = audit log disabled
	shutdownAudit     context.CancelFunc
	settleWorker      *ledger.Worker // optional; nil when URM not configured
	shutdownSettle    context.CancelFunc

	// Serving pipeline — shared across requests (steps are stateless)
	pipeline    *serving.Pipeline
	apiKeyCache *apikey.Cache
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		panic("logger is required")
	}

	q := dbgen.New(cfg.Postgres)
	grantChecker := pgadapter.NewModelGrantChecker(q)
	oauthCreds := pgadapter.NewOAuthCredentialStore(cfg.Postgres, cfg.Security.ProviderKeyMaster)
	routeSelector := pgadapter.NewRouteSelector(q, cfg.Postgres, cfg.Security.ProviderKeyMaster, grantChecker).
		WithOAuthCredentialStore(oauthCreds).
		WithDefaultTimeouts(domain.RouteTimeouts{
			Connect:     cfg.Serving.Timeouts.Connect,
			FirstByte:   cfg.Serving.Timeouts.FirstByte,
			Idle:        cfg.Serving.Timeouts.Idle,
			MaxDuration: cfg.Serving.Timeouts.MaxDuration,
		})

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
	gw := metrics.NewGateway()

	var rateLimiter serving.RateLimiter
	if cfg.Redis != nil {
		rateLimiter = redisadapter.NewRateLimiter(cfg.Redis, q, 4096)
	}

	// 分账层装配：每请求结束后 LedgerStep finalizer 把 BillingResult 累加到
	// ai_user_credit_ledger.pending_*_micro；settle worker 按 60s tick / 阈值
	// 触发批量调 URM Consume。
	var creditLedger *ledger.Ledger
	var settleWorker *ledger.Worker
	if cfg.URM != nil {
		creditLedger = ledger.New(cfg.Postgres, cfg.URM, cfg.Logger)
		settleWorker = ledger.NewWorker(creditLedger, ledger.WorkerConfig{})
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

	// Audit log: async structured request/response persistence.
	auditStore := pgadapter.NewAuditStore(cfg.Postgres)
	blobStore := blobstore.NewPGStore(cfg.Postgres)
	auditWorker := audit.NewWorker(auditStore, blobStore)

	usageLogger := pgadapter.NewUsageLogger(q)

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
		auditWorker:       auditWorker,
		settleWorker:      settleWorker,
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
			&serving.ExecuteStep{
				Transport: transport.NewClient(120 * time.Second),
				Health:    healthTracker,
				OAuthPool: oauthCreds,
				Budget:    serving.DefaultRetryBudget(),
				Scorer:    scorer,
				Stats:     routeStats,
				Sticky:    stickyStore,
			},
			&serving.UsageLogStep{Logger: usageLogger, Metrics: gw},
		).WithFinalizers(
			&serving.LedgerStep{Ledger: creditLedger, Trigger: workerTrigger(settleWorker)},
			&serving.AuditFinalizer{Worker: auditWorker},
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
	router.Route("/console/v2", func(r chi.Router) {
		r.Get("/chat/models", s.handleConsoleChatModelsV2)
		r.Get("/chat/sessions", s.handleConsoleChatListSessionsV2)
		r.Post("/chat/sessions", s.handleConsoleChatCreateSessionV2)
		r.Get("/chat/sessions/{sessionID}", s.handleConsoleChatGetSessionV2)
		r.Delete("/chat/sessions/{sessionID}", s.handleConsoleChatDeleteSessionV2)
		r.Post("/chat/sessions/{sessionID}/messages:stream", s.handleConsoleChatStreamV2)
	})
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
		r.Post("/images/edits", s.handleRuntime(domain.CapabilityImage))
		r.Post("/messages", s.handleRuntime(domain.CapabilityChat)) // Native Anthropic client path
		r.Post("/messages/count_tokens", s.handleCountTokens)       // Anthropic count_tokens API
	})
	// Native Gemini client endpoints. Chi captures the last URL segment
	// (e.g. "gemini-pro:generateContent") whole; the handler splits on ":" to
	// derive the model name and action. Required so strict 1:1 routing can
	// match gemini_generate / gemini_embeddings deployments without a Chat-API
	// cross-protocol bridge.
	router.Route("/v1beta", func(r chi.Router) {
		r.Use(s.runtimeAuth)
		r.Post("/models/{modelAction}", s.handleGeminiRuntime)
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

// workerTrigger returns w.Trigger() or nil when the worker is unset, so
// LedgerStep.Trigger receives a typed-nil channel that select drops on.
func workerTrigger(w *ledger.Worker) chan<- struct{} {
	if w == nil {
		return nil
	}
	return w.Trigger()
}

func (s *Server) Start(addr string) error {
	auditCtx, auditCancel := context.WithCancel(context.Background())
	s.shutdownAudit = auditCancel
	if s.auditWorker != nil {
		s.auditWorker.Start(auditCtx)
	}

	if s.settleWorker != nil {
		settleCtx, settleCancel := context.WithCancel(context.Background())
		s.shutdownSettle = settleCancel
		go s.settleWorker.Run(settleCtx)
	}

	s.httpServer.Addr = addr
	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpServer.Shutdown(ctx)
	// Cancel audit context: Worker.run drains remaining entries then exits.
	if s.shutdownAudit != nil {
		s.shutdownAudit()
	}
	if s.shutdownSettle != nil {
		s.shutdownSettle()
	}
	return err
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
					writeErr(ww, http.StatusInternalServerError, BizErrInternal, "internal server error")
				}
				s.logger.Error("HTTP request panic",
					zap.Any("error", recovered),
					zap.Stack("stack"),
					zap.String("request_id", requestID),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)
			}

			elapsed := time.Since(start)
			status := responseStatus(ww)
			routePath := routePattern(r)

			fields := []zap.Field{
				zap.String("request_id", requestID),
				zap.String("method", r.Method),
				zap.String("path", routePath),
				zap.Int("status", status),
				zap.Duration("latency", elapsed),
				zap.String("client_ip", r.RemoteAddr),
				zap.Int("bytes", ww.BytesWritten()),
				zap.String("user_agent", r.UserAgent()),
			}
			if logCtx.TenantID != "" {
				fields = append(fields, zap.String("tenant_id", logCtx.TenantID))
			}
			if logCtx.UserID != "" {
				fields = append(fields, zap.String("user_id", logCtx.UserID))
			}
			if logCtx.Role != "" {
				fields = append(fields, zap.String("role", logCtx.Role))
			}
			if logCtx.APIKeyIDHash != "" {
				fields = append(fields, zap.String("api_key_id_hash", logCtx.APIKeyIDHash))
			}

			switch {
			case status >= 500:
				s.logger.Error("HTTP Request", fields...)
			case status >= 400:
				s.logger.Warn("HTTP Request", fields...)
			default:
				s.logger.Info("HTTP Request", fields...)
			}
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
