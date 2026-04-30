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

	"uni-ai-api/backend/internal/config"
	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/urm"
)

type Config struct {
	App           config.AppConfig
	Server        config.ServerConfig
	Logging       config.LoggingConfig
	Security      config.SecurityConfig
	URM           urmClient
	URMAppKey     string // 本服务的 app_key，用于校验 JWT claims
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
	urmAppKey     string
	jwksValidator jwksValidator
	banSubscriber banChecker
	httpClient    *http.Client
	logging       config.LoggingConfig
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	s := &Server{
		postgres:      cfg.Postgres,
		redis:         cfg.Redis,
		logger:        cfg.Logger,
		queries:       dbgen.New(cfg.Postgres),
		security:      cfg.Security,
		urmClient:     cfg.URM,
		urmAppKey:     cfg.URMAppKey,
		jwksValidator: cfg.JWKSValidator,
		banSubscriber: cfg.BanSubscriber,
		logging:       cfg.Logging,
		httpClient: &http.Client{
			Timeout: 0,
		},
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Timeout(120 * time.Second))
	router.Use(s.requestLogger)

	router.Get("/health", s.handleHealth)
	router.Get("/ready", s.handleReady)
	router.Get("/api/auth/callback", s.handleAuthCallback)
	router.Route("/api/v1", func(r chi.Router) {
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
		r.Get("/upstream-deployments", s.handleAdminListUpstreamDeployments)
		r.Post("/upstream-deployments", s.handleAdminCreateUpstreamDeployment)
		r.Get("/upstream-deployments/{deploymentID}", s.handleAdminGetUpstreamDeployment)
		r.Patch("/upstream-deployments/{deploymentID}", s.handleAdminUpdateUpstreamDeployment)
		r.Patch("/upstream-deployments/{deploymentID}/status", s.handleAdminUpdateUpstreamDeploymentStatus)
		r.Post("/upstream-deployments/{deploymentID}/health-check", s.handleAdminCheckUpstreamDeploymentHealth)
		r.Get("/upstream-deployments/{deploymentID}/cost-prices", s.handleAdminListUpstreamDeploymentCostPrices)
		r.Post("/upstream-deployments/{deploymentID}/cost-prices", s.handleAdminCreateUpstreamDeploymentCostPrice)
		r.Patch("/upstream-deployments/{deploymentID}/cost-prices/{priceID}", s.handleAdminUpdateUpstreamDeploymentCostPrice)
		r.Patch("/upstream-deployments/{deploymentID}/cost-prices/{priceID}/status", s.handleAdminUpdateUpstreamDeploymentCostPriceStatus)
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
		r.Get("/tenants/{tenantID}/users/{userID}/api-keys", s.handleAdminListUserAPIKeys)
		r.Post("/tenants/{tenantID}/users/{userID}/api-keys", s.handleAdminCreateUserAPIKey)
		r.Patch("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}", s.handleAdminUpdateUserAPIKey)
		r.Patch("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}/status", s.handleAdminUpdateUserAPIKeyStatus)

		// =============================================================
		// 租户自管理（/tenants/me/* 路由）
		// =============================================================
		r.Post("/tenants/me/api-keys", s.handleTenantsMeAPIKeysCreate)
		r.Patch("/tenants/me/api-keys/{apiKeyID}", s.handleTenantsMeAPIKeysUpdate)
		r.Patch("/tenants/me/api-keys/{apiKeyID}/status", s.handleTenantsMeAPIKeysStatus)
		r.Put("/tenants/me/user-prices/{modelID}", s.handleTenantsMeUserPricesUpsert)
		r.Delete("/tenants/me/user-prices/{modelID}", s.handleTenantsMeUserPricesDelete)

		// =============================================================
		// 用户自管理（/users/me/* 路由）
		// =============================================================
		r.Post("/users/me/api-keys", s.handleUsersMeAPIKeysCreate)
		r.Patch("/users/me/api-keys/{apiKeyID}", s.handleUsersMeAPIKeysUpdate)
		r.Patch("/users/me/api-keys/{apiKeyID}/status", s.handleUsersMeAPIKeysStatus)

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
		r.Post("/chat/completions", s.handleChatCompletions)
		r.Post("/responses", s.handleResponses)
		r.Post("/embeddings", s.handleEmbeddings)
		r.Post("/images/generations", s.handleImageGenerations)
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

			if s.logging.SlowRequestMs > 0 && elapsed.Milliseconds() >= s.logging.SlowRequestMs {
				s.logger.Warn("slow http request", attrs...)
			}
			if s.logging.AccessLog {
				s.logger.Info("http request", attrs...)
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
