package httpserver

import (
	"context"
	"log/slog"
	"net/http"
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
	App      config.AppConfig
	Security config.SecurityConfig
	URM      urmClient
	Postgres *pgxpool.Pool
	Redis    *redis.Client
	Logger   *slog.Logger
}

type urmClient interface {
	UserInfo(ctx context.Context, token string) (*urm.UserInfoResponse, error)
	Freeze(ctx context.Context, req urm.FreezeRequest) (*urm.FreezeResponse, error)
	Confirm(ctx context.Context, req urm.ConfirmRequest) (*urm.ConfirmResponse, error)
	Cancel(ctx context.Context, transactionID string) error
}

type Server struct {
	httpServer *http.Server
	postgres   *pgxpool.Pool
	redis      *redis.Client
	logger     *slog.Logger
	queries    *dbgen.Queries
	security   config.SecurityConfig
	urmClient  urmClient
	httpClient *http.Client
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	s := &Server{
		postgres:  cfg.Postgres,
		redis:     cfg.Redis,
		logger:    cfg.Logger,
		queries:   dbgen.New(cfg.Postgres),
		security:  cfg.Security,
		urmClient: cfg.URM,
		httpClient: &http.Client{
			Timeout: 0,
		},
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(120 * time.Second))
	router.Use(s.requestLogger)

	router.Get("/health", s.handleHealth)
	router.Get("/ready", s.handleReady)
	router.Route("/admin", func(r chi.Router) {
		r.Use(s.adminAuth)
		r.Use(s.adminAudit)
		r.Use(s.adminScope)
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
		r.Get("/tenants/{tenantID}/model-grants", s.handleAdminListTenantModelGrants)
		r.Post("/tenants/{tenantID}/model-grants", s.handleAdminGrantModelToTenant)
		r.Patch("/tenants/{tenantID}/model-grants/{modelID}/status", s.handleAdminUpdateTenantModelGrantStatus)
		r.Get("/tenants/{tenantID}/model-price-overrides", s.handleAdminListTenantModelPriceOverrides)
		r.Get("/tenants/{tenantID}/model-price-overrides/{modelID}", s.handleAdminGetTenantModelPriceOverride)
		r.Put("/tenants/{tenantID}/model-price-overrides/{modelID}", s.handleAdminUpsertTenantModelPriceOverride)
		r.Delete("/tenants/{tenantID}/model-price-overrides/{modelID}", s.handleAdminDeleteTenantModelPriceOverride)
		r.Get("/tenants/{tenantID}/api-keys", s.handleAdminListTenantAPIKeys)
		r.Post("/tenants/{tenantID}/api-keys", s.handleAdminCreateTenantAPIKey)
		r.Patch("/tenants/{tenantID}/api-keys/{apiKeyID}", s.handleAdminUpdateTenantAPIKey)
		r.Patch("/tenants/{tenantID}/api-keys/{apiKeyID}/status", s.handleAdminUpdateTenantAPIKeyStatus)
		r.Get("/tenants/{tenantID}/users/{userID}/api-keys", s.handleAdminListUserAPIKeys)
		r.Post("/tenants/{tenantID}/users/{userID}/api-keys", s.handleAdminCreateUserAPIKey)
		r.Patch("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}", s.handleAdminUpdateUserAPIKey)
		r.Patch("/tenants/{tenantID}/users/{userID}/api-keys/{apiKeyID}/status", s.handleAdminUpdateUserAPIKeyStatus)
		r.Get("/limit-policies", s.handleAdminListRuntimeLimitPolicies)
		r.Post("/limit-policies", s.handleAdminCreateRuntimeLimitPolicy)
		r.Patch("/limit-policies/{policyID}", s.handleAdminUpdateRuntimeLimitPolicy)
		r.Patch("/limit-policies/{policyID}/status", s.handleAdminUpdateRuntimeLimitPolicyStatus)
		r.Get("/dashboard/summary", s.handleAdminDashboardSummary)
		r.Get("/dashboard/top-models", s.handleAdminDashboardTopModels)
		r.Get("/dashboard/top-tenants", s.handleAdminDashboardTopTenants)
		r.Get("/dashboard/recent-errors", s.handleAdminDashboardRecentErrors)
		r.Get("/usage-logs", s.handleAdminListUsageLogs)
		r.Get("/usage-summary", s.handleAdminListUsageSummary)
		r.Get("/usage-unit-summary", s.handleAdminListUsageUnitSummary)
		r.Get("/audit-logs", s.handleAdminListAuditLogs)
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
		next.ServeHTTP(ww, r)
		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
