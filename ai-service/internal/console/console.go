// Package console is the management control plane: the JWT-authenticated admin
// API under /api/v1, the web console chat under /console/v2, and the OAuth
// callback. Every management response uses the {code,message,data} envelope.
package console

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	pgadapter "xiaodou/unihub/ai-service/internal/adapters/postgres"
	"xiaodou/unihub/ai-service/internal/apikey"
	"xiaodou/unihub/ai-service/internal/config"
	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/gateway"
	apikeysvc "xiaodou/unihub/ai-service/internal/service/apikey"
	auditsvc "xiaodou/unihub/ai-service/internal/service/audit"
	dashboardsvc "xiaodou/unihub/ai-service/internal/service/dashboard"
	grantsvc "xiaodou/unihub/ai-service/internal/service/grant"
	limitsvc "xiaodou/unihub/ai-service/internal/service/limit"
	modelsvc "xiaodou/unihub/ai-service/internal/service/model"
	pricebooksvc "xiaodou/unihub/ai-service/internal/service/pricebook"
	providersvc "xiaodou/unihub/ai-service/internal/service/provider"
	usagesvc "xiaodou/unihub/ai-service/internal/service/usage"
	"xiaodou/unihub/ai-service/internal/tokenrefresh"
	"xiaodou/unihub/ai-service/internal/urm"
)

// URMExchanger exchanges an OAuth2 authorization code for a token pair.
type URMExchanger interface {
	ExchangeCode(ctx context.Context, code, redirectURI string) (*urm.TokenPairResponse, error)
}

// JWKSValidator validates a URM-issued JWT and returns its claims.
type JWKSValidator interface {
	ValidateToken(ctx context.Context, tokenStr string) (*urm.Claims, error)
}

// BanChecker reports whether a user has been banned (real-time revocation).
type BanChecker interface {
	IsBanned(userID string) bool
}

// Deps are the management-plane dependencies assembled in cmd/server.
type Deps struct {
	Postgres          *pgxpool.Pool
	Redis             *redis.Client
	Logger            *zap.Logger
	Queries           *dbgen.Queries
	Security          config.SecurityConfig
	URMClient         URMExchanger
	URMClientID       string
	JWKSValidator     JWKSValidator
	BanSubscriber     BanChecker
	HTTPClient        *http.Client
	OAuthCreds        *pgadapter.OAuthCredentialStore
	TokenRefresher    *tokenrefresh.Refresher
	RouteSelector     *pgadapter.RouteSelector
	RouteWeightsStore *pgadapter.RouteWeightsStore
	APIKeyCache       *apikey.Cache    // shared with gateway; console invalidates on key mutation
	Gateway           *gateway.Gateway // runtime plane, driven by console chat

	// Phase B: 管理域 service 层（逐域接通中）
	AuditSvc      *auditsvc.Service
	GrantSvc      *grantsvc.Service
	LimitSvc      *limitsvc.Service
	DashboardSvc  *dashboardsvc.Service
	UsageSvc      *usagesvc.Service
	ModelSvc      *modelsvc.Service
	ModelRouteSvc *modelsvc.RouteService
	PriceBookSvc  *pricebooksvc.Service
	ProviderSvc   *providersvc.Service
	APIKeySvc     *apikeysvc.Service
}

// Console serves the management API.
type Console struct {
	postgres          *pgxpool.Pool
	redis             *redis.Client
	logger            *zap.Logger
	queries           *dbgen.Queries
	security          config.SecurityConfig
	urmClient         URMExchanger
	urmClientID       string
	jwksValidator     JWKSValidator
	banSubscriber     BanChecker
	httpClient        *http.Client
	oauthCreds        *pgadapter.OAuthCredentialStore
	tokenRefresher    *tokenrefresh.Refresher
	routeSelector     *pgadapter.RouteSelector
	routeWeightsStore *pgadapter.RouteWeightsStore
	apiKeyCache       *apikey.Cache
	gateway           *gateway.Gateway

	auditSvc      *auditsvc.Service
	grantSvc      *grantsvc.Service
	limitSvc      *limitsvc.Service
	dashboardSvc  *dashboardsvc.Service
	usageSvc      *usagesvc.Service
	modelSvc      *modelsvc.Service
	modelRouteSvc *modelsvc.RouteService
	priceBookSvc  *pricebooksvc.Service
	providerSvc   *providersvc.Service
	apiKeySvc     *apikeysvc.Service
}

func New(deps Deps) *Console {
	if deps.Logger == nil {
		panic("console: logger is required")
	}
	return &Console{
		postgres:          deps.Postgres,
		redis:             deps.Redis,
		logger:            deps.Logger,
		queries:           deps.Queries,
		security:          deps.Security,
		urmClient:         deps.URMClient,
		urmClientID:       deps.URMClientID,
		jwksValidator:     deps.JWKSValidator,
		banSubscriber:     deps.BanSubscriber,
		httpClient:        deps.HTTPClient,
		oauthCreds:        deps.OAuthCreds,
		tokenRefresher:    deps.TokenRefresher,
		routeSelector:     deps.RouteSelector,
		routeWeightsStore: deps.RouteWeightsStore,
		apiKeyCache:       deps.APIKeyCache,
		gateway:           deps.Gateway,
		auditSvc:          deps.AuditSvc,
		grantSvc:          deps.GrantSvc,
		limitSvc:          deps.LimitSvc,
		dashboardSvc:      deps.DashboardSvc,
		usageSvc:          deps.UsageSvc,
		modelSvc:          deps.ModelSvc,
		modelRouteSvc:     deps.ModelRouteSvc,
		priceBookSvc:      deps.PriceBookSvc,
		providerSvc:       deps.ProviderSvc,
		apiKeySvc:         deps.APIKeySvc,
	}
}

// requireRole gates a route group: the request's role (from the JWT-derived
// apiContext) must be one of roles, else 403. This replaces the old
// apiRequestAllowed path-string switch — access is now declared structurally
// at mount time.
func (s *Console) requireRole(roles ...apiRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := roleFromAPIContext(r.Context())
			for _, allowed := range roles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		})
	}
}

// recoverer turns a panic in any management handler into the standard
// {code,message,data} envelope (HTTP 500). It runs inner to the shared
// httpx.RequestLogger, so management panics keep the console contract instead of
// falling through to httpx's plane-agnostic plain-JSON 500.
func (s *Console) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("console request panic",
					zap.Any("error", rec),
					zap.Stack("stack"),
					zap.String("request_id", requestIDFromContext(r.Context())),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)
				writeErr(w, http.StatusInternalServerError, BizErrInternal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Routes registers the management endpoints on r.
func (s *Console) Routes(r chi.Router) {
	r.With(s.recoverer).Get("/api/auth/callback", s.handleAuthCallback)

	// /console/v2 — web console chat. Auth is performed inside each handler
	// (consoleRuntimeIdentity) because it speaks the runtime protocol, not the
	// management envelope; no apiAuth/adminAudit here.
	r.Route("/console/v2", func(r chi.Router) {
		r.Use(s.recoverer)
		r.Get("/chat/models", s.handleConsoleChatModelsV2)
		r.Get("/chat/sessions", s.handleConsoleChatListSessionsV2)
		r.Post("/chat/sessions", s.handleConsoleChatCreateSessionV2)
		r.Get("/chat/sessions/{sessionID}", s.handleConsoleChatGetSessionV2)
		r.Delete("/chat/sessions/{sessionID}", s.handleConsoleChatDeleteSessionV2)
		r.Post("/chat/sessions/{sessionID}/messages:stream", s.handleConsoleChatStreamV2)
	})

	r.Route("/api/v1", func(r chi.Router) {
		// chi's Timeout middleware swaps the ResponseWriter for a buffered one
		// that cancels the request after the deadline. It MUST NOT wrap the
		// /v1/* runtime routes — long-running SSE streams would be killed
		// mid-flight. Mounting it only on management /api/v1 is safe because
		// those endpoints return JSON within a bounded time.
		r.Use(s.recoverer)
		r.Use(chimw.Timeout(120 * time.Second))
		r.Use(s.apiAuth)
		r.Use(s.adminAudit)

		// ===============================================================
		// 平台级资源（仅 platform 角色）
		// ===============================================================
		r.Group(func(r chi.Router) {
			r.Use(s.requireRole(apiRolePlatform))

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

			// model 写操作（平台）
			r.Post("/models", s.handleAdminCreateModel)
			r.Patch("/models/{modelID}", s.handleAdminUpdateModel)
			r.Patch("/models/{modelID}/status", s.handleAdminUpdateModelStatus)
			r.Post("/models/{modelID}/routes", s.handleAdminCreateModelRoute)
			r.Patch("/models/{modelID}/routes/{routeID}", s.handleAdminUpdateModelRoute)
			r.Patch("/models/{modelID}/routes/{routeID}/status", s.handleAdminUpdateModelRouteStatus)
			r.Delete("/models/{modelID}/routes/{routeID}", s.handleAdminDeleteModelRoute)

			r.Get("/limit-policies", s.handleAdminListRuntimeLimitPolicies)
			r.Post("/limit-policies", s.handleAdminCreateRuntimeLimitPolicy)
			r.Patch("/limit-policies/{policyID}", s.handleAdminUpdateRuntimeLimitPolicy)
			r.Patch("/limit-policies/{policyID}/status", s.handleAdminUpdateRuntimeLimitPolicyStatus)
			r.Get("/audit-logs", s.handleAdminListAuditLogs)

			r.Get("/oauth-pool-health", s.handleAdminGetOAuthPoolHealth)
			r.Get("/system/status", s.handleAdminSystemStatus)
			r.Get("/route-weights/{scope}", s.handleAdminGetRouteWeights)
			r.Put("/route-weights/{scope}", s.handleAdminPutRouteWeights)
			r.Post("/usage-logs/{id}/replay", s.handleReplay)

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

			// 平台管理指定租户
			r.Get("/tenants/{tenantID}/model-grants", s.handleAdminListTenantModelGrants)
			r.Post("/tenants/{tenantID}/model-grants", s.handleAdminGrantModelToTenant)
			r.Patch("/tenants/{tenantID}/model-grants/{modelID}/status", s.handleAdminUpdateTenantModelGrantStatus)
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

			// 平台专属分析
			r.Get("/usage-unit-summary", s.handleAdminListUsageUnitSummary)
			r.Get("/analytics/daily-trend", s.handleAdminListDailyTrend)

			// 价格表（Price Book 统一定价）
			r.Get("/price-books", s.handleAdminListPriceBooks)
			r.Post("/price-books", s.handleAdminCreatePriceBook)
			r.Get("/price-books/{bookID}", s.handleAdminGetPriceBook)
			r.Patch("/price-books/{bookID}", s.handleAdminUpdatePriceBook)
			r.Delete("/price-books/{bookID}", s.handleAdminDeletePriceBook)
			r.Get("/price-books/{bookID}/entries", s.handleAdminListPriceBookEntries)
			r.Put("/price-books/{bookID}/entries", s.handleAdminUpsertPriceBookEntry)    // model_code in body
			r.Delete("/price-books/{bookID}/entries", s.handleAdminDeletePriceBookEntry) // ?model_code=
			r.Get("/price-books/litellm/models", s.handleAdminSearchLiteLLM)
			r.Post("/price-books/{bookID}/sync-common", s.handleAdminSyncCommonModels)
			r.Post("/price-books/{bookID}/import-litellm", s.handleAdminImportLiteLLM)

			// USD→积分 全局汇率
			r.Get("/pricing/credits-per-usd", s.handleAdminGetCreditsPerUSD)
			r.Put("/pricing/credits-per-usd", s.handleAdminSetCreditsPerUSD)

			// 平台→租户售价绑定
			r.Get("/tenant-sell-bindings", s.handleAdminListTenantSellBindings)
			r.Get("/tenants/{tenantID}/sell-binding", s.handleAdminGetTenantSellBinding)
			r.Put("/tenants/{tenantID}/sell-binding", s.handleAdminUpsertTenantSellBinding)
			r.Delete("/tenants/{tenantID}/sell-binding", s.handleAdminDeleteTenantSellBinding)

			// 租户→用户售价绑定（平台代管视图；租户自助在 ai-tenant，P6）
			r.Get("/tenants/{tenantID}/user-sell-binding", s.handleAdminGetUserSellBinding)
			r.Put("/tenants/{tenantID}/user-sell-binding", s.handleAdminUpsertUserSellBinding)
			r.Delete("/tenants/{tenantID}/user-sell-binding", s.handleAdminDeleteUserSellBinding)
		})

		// ===============================================================
		// 三角色共享只读（platform + tenant + user）
		// ===============================================================
		r.Group(func(r chi.Router) {
			r.Use(s.requireRole(apiRolePlatform, apiRoleTenant, apiRoleUser))

			r.Get("/models", s.handleAdminListModels)
			r.Get("/models/{modelID}/routes", s.handleAdminListModelRoutes)
			r.Get("/models/{modelID}/routes/{routeID}", s.handleAdminGetModelRoute)
			r.Get("/dashboard/summary", s.handleDashboardSummaryByRole)
			r.Get("/dashboard/top-models", s.handleAdminDashboardTopModels)
			r.Get("/dashboard/top-tenants", s.handleAdminDashboardTopTenants)
			r.Get("/dashboard/recent-errors", s.handleAdminDashboardRecentErrors)
		})

		// ===============================================================
		// 租户面（platform + tenant）：/tenants/me/* 与租户自动过滤资源
		// ===============================================================
		r.Group(func(r chi.Router) {
			r.Use(s.requireRole(apiRolePlatform, apiRoleTenant))

			r.Post("/tenants/me/api-keys", s.handleTenantsMeAPIKeysCreate)
			r.Patch("/tenants/me/api-keys/{apiKeyID}", s.handleTenantsMeAPIKeysUpdate)
			r.Patch("/tenants/me/api-keys/{apiKeyID}/status", s.handleTenantsMeAPIKeysStatus)
			r.Post("/tenants/me/api-keys/{apiKeyID}/rotate", s.handleTenantsMeAPIKeysRotate)
			r.Delete("/tenants/me/api-keys/{apiKeyID}", s.handleTenantsMeAPIKeysDelete)

			// Price Book 自助定价
			r.Get("/tenants/me/sell-binding", s.handleTenantsMeSellBinding)
			r.Get("/tenants/me/user-sell-binding", s.handleTenantsMeUserSellBinding)
			r.Put("/tenants/me/user-sell-binding", s.handleTenantsMeUserSellBindingUpsert)
			r.Delete("/tenants/me/user-sell-binding", s.handleTenantsMeUserSellBindingDelete)
			r.Get("/tenants/me/effective-prices", s.handleTenantsMeEffectivePrices)

			r.Get("/tenant-api-keys", s.handleTenantAPIKeysSelf)
			r.Get("/tenant-model-grants", s.handleTenantModelGrantsSelf)
			r.Get("/usage-logs", s.handleUsageLogsByRole)
			r.Get("/usage-summary", s.handleAdminListUsageSummary)
		})

		// ===============================================================
		// 用户面（platform + user）：/users/me/* 与用户自动过滤资源
		// ===============================================================
		r.Group(func(r chi.Router) {
			r.Use(s.requireRole(apiRolePlatform, apiRoleUser))

			r.Post("/users/me/api-keys", s.handleUsersMeAPIKeysCreate)
			r.Patch("/users/me/api-keys/{apiKeyID}", s.handleUsersMeAPIKeysUpdate)
			r.Patch("/users/me/api-keys/{apiKeyID}/status", s.handleUsersMeAPIKeysStatus)
			r.Post("/users/me/api-keys/{apiKeyID}/rotate", s.handleUsersMeAPIKeysRotate)
			r.Delete("/users/me/api-keys/{apiKeyID}", s.handleUsersMeAPIKeysDelete)

			r.Get("/users/me/effective-prices", s.handleUsersMeEffectivePrices)
			r.Get("/user-api-keys", s.handleUserAPIKeysSelf)
			r.Get("/user-model-grants", s.handleUserModelGrantsSelf)
			r.Get("/user-usage-logs", s.handleUserUsageLogsSelf)
			r.Get("/user-usage-summary", s.handleUserUsageSummarySelf)
		})
	})
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
