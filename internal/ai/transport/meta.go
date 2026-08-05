package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/commercial"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/tokenrefresh"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/libs/go/httpx"
	"xiaodou/dai/libs/go/server"
)

// Version is the public ai-service API version exposed in OpenAPI and meta endpoints.
const Version = "1.0.0"

type AIDeps struct {
	Service           string
	Version           string
	Postgres          *pgxpool.Pool
	Redis             *redis.Client
	Queries           *dbgen.Queries
	OAuth             *pgadapter.OAuthCredentialStore
	TokenRefresher    *tokenrefresh.Refresher
	ClientCatalog     *clientcatalog.Service
	ProviderKeyMaster string
	HTTPClient        *http.Client
	Logger            *zap.Logger
	Health            routing.HealthTracker
	Weights           *pgadapter.RouteWeightsStore

	// IdentityProvider 替代原 URMClient——合并后进程内获取用户/租户信息。
	// nil 时 identity enrichment 返回空结果（和原 URMClient == nil 行为一致）。
	IdentityProvider  IdentityProvider

	JWKSValidator    HumaJWKSValidator
	BanChecker       HumaBanChecker
	ExpectedClientID string
	TenantEndUsers   TenantEndUserVerifier
	ServiceAccess    interface {
		Check(context.Context, int, string, string, string, string) error
	}

	PriceBookSvc      *billingcontrol.Service
	AccountSvc        *upstreamcontrol.Service // 上游账号管理
	UpstreamAccessSvc *upstreamaccess.Service
	CommercialSvc     *commercial.Service // 新 commercial control plane（当前已承接 limit 线）
	GroupTransferSvc  *commercial.GroupTransferService
	DashboardSvc      *observabilitycontrol.DashboardService
	UsageSvc          *observabilitycontrol.UsageService
	AuditSvc          *observabilitycontrol.AuditService
	APIKeySvc         *identitycontrol.Service
	WorkspaceSvc      *workspacesvc.Service
	Subscriptions     *subscription.Service // AI 订阅制套餐（nil = urm 未装配时禁用）
	ServiceIdentity   interface{ Ready() (bool, error) }

	// 风控中心（内容安全审核）。四者始终一起装配，nil 只会发生在测试里未注入的场景。
	RiskControlConfigSvc *riskcontrol.ConfigService
	RiskControlLogSvc    *riskcontrol.LogService
	RiskControlEventSvc  *riskcontrol.EventService
	RiskControlChecker   *riskcontrol.Checker // 供 /risk-control/test 复用 Detect()，不落库
}

type infoOutput struct {
	Body struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
}

type readyOutput struct {
	Body struct {
		Status     string                     `json:"status"`
		Components map[string]componentStatus `json:"components"`
	}
}

type componentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func RegisterAI(api huma.API, d AIDeps) {
	if d.Service == "" {
		d.Service = "ai-service"
	}
	if d.Version == "" {
		d.Version = Version
	}
	server.Health(api, d.Service, d.Version)
	registerInfo(api, d)
	registerReady(api, d)

	pricingRead := huma.NewGroup(api)
	pricingRead.UseMiddleware(platformOrTenantUserAuth(api, d))
	registerPricingRead(pricingRead, d)

	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d))
	registerPriceBooks(management, d)
	registerUpstreamAccounts(management, d)
	registerUpstreamDiscovery(management, d)
	registerUpstreamAccountTest(management, d)
	registerUpstreamModelBindings(management, d)
	registerDashboard(management, d)
	registerUsage(management, d)
	registerAudit(management, d)
	registerRiskControl(management, d)
	registerLimits(management, d)
	registerTenantUpstreamAccess(management, d)
	registerAPIKeys(management, d)
	registerRunKeys(management, d)
	registerOAuthPools(management, d)
	registerSystem(management, d)
	registerPricingWrite(management, d)

	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserAuth(api, d))
	registerGroups(tenant, d)
	registerGroupTransfer(tenant, d)
	registerTenantSelfPricing(tenant, d)
	registerTenantPriceBooks(tenant, d)
	registerTenantUpstreamCatalog(tenant, d)
	registerTenantSelfAPIKeys(tenant, d)
	registerTenantSelf(tenant, d)
	registerTenantSelfWorkspace(tenant, d)
	registerTenantSelfAppPrompts(tenant, d)
	registerTenantSelfAppAgents(tenant, d)
	registerTenantSelfRunKeys(tenant, d)
	registerTenantSelfSubscriptions(tenant, d)

	userSelf := huma.NewGroup(api)
	userSelf.UseMiddleware(endUserAuth(api, d))
	registerUserSelf(userSelf, d)
	registerUserSelfWorkspace(userSelf, d)
	registerUserSelfAppPrompts(userSelf, d)
	registerUserSelfAppAgents(userSelf, d)
	registerUserSelfRunKeys(userSelf, d)
	registerUserSelfSubscriptions(userSelf, d)
}

func mapServiceError(err error) error {
	if err == nil {
		return nil
	}

	var verr *domain.ValidationError
	var commercialErr *commercial.ValidationError
	var priceConflict *domain.DispatchRulePriceConflictError
	var groupInUse *domain.GroupInUseError
	switch {
	case errors.As(err, &priceConflict):
		return httpx.New("dispatch_rule_price_conflict", http.StatusConflict, "Conflict").
			WithDetail("调度规则目标模型在分组零售价格表中缺少所需能力价格").
			WithMeta(map[string]any{"conflicts": priceConflict.Conflicts}).
			WithCause(err)
	case errors.As(err, &groupInUse):
		return httpx.New("group_in_use", http.StatusConflict, "Conflict").
			WithDetail("分组仍被业务配置引用，请先解除引用").
			WithMeta(map[string]any{
				"group_id":     groupInUse.GroupID,
				"group_name":   groupInUse.GroupName,
				"dependencies": groupInUse.Dependencies,
			}).
			WithCause(err)
	case errors.As(err, &commercialErr):
		detail := commercialErr.Message
		if commercialErr.Field != "" {
			detail = commercialErr.Field + ": " + commercialErr.Message
		}
		return httpx.ErrBadRequest.WithDetail(detail).WithCause(err)
	case errors.As(err, &verr):
		detail := verr.Message
		if verr.Field != "" {
			detail = verr.Field + ": " + verr.Message
		}
		return httpx.ErrBadRequest.WithDetail(detail).WithCause(err)
	case errors.Is(err, domain.ErrValidation):
		return httpx.ErrBadRequest.WithDetail(err.Error()).WithCause(err)
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		return httpx.ErrNotFound.WithDetail("resource not found").WithCause(err)
	case errors.Is(err, domain.ErrConflict):
		detail := "resource already exists"
		if err.Error() != domain.ErrConflict.Error() {
			detail = err.Error()
		}
		return httpx.ErrConflict.WithDetail(detail).WithCause(err)
	case errors.Is(err, domain.ErrForbidden):
		return httpx.ErrForbidden.WithDetail("forbidden").WithCause(err)
	case errors.Is(err, commercial.ErrNoAccessibleGroup), errors.Is(err, coreruntime.ErrNoAllowedGroup):
		return httpx.ErrForbidden.WithDetail("no group is accessible to this caller").WithCause(err)
	case errors.Is(err, commercial.ErrClientSurfaceNotAllowed):
		return httpx.ErrForbidden.WithDetail("this API endpoint is not enabled for the group").WithCause(err)
	case errors.Is(err, coreruntime.ErrNoDispatchOption), errors.Is(err, coreruntime.ErrNoRouteCandidates):
		return httpx.New("no_available_route", http.StatusServiceUnavailable, "Service Unavailable").
			WithDetail("no available upstream route for this request").
			WithCause(err)
	case isInvalidUUIDError(err):
		return httpx.ErrBadRequest.WithDetail("invalid UUID").WithCause(err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return httpx.ErrConflict.WithDetail("resource already exists").WithCause(err)
		case "23503":
			return httpx.ErrBadRequest.WithDetail("referenced resource not found").WithCause(err)
		case "23514":
			return httpx.ErrBadRequest.WithDetail("invalid field value").WithCause(err)
		case "22P02":
			return httpx.ErrBadRequest.WithDetail("invalid input format").WithCause(err)
		}
	}

	return httpx.ErrInternal.WithCause(err)
}

func isInvalidUUIDError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "cannot parse UUID")
}

func registerInfo(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-info",
		Method:      http.MethodGet,
		Path:        "/api/v1/info",
		Summary:     "服务信息",
		Tags:        []string{"meta"},
	}, func(_ context.Context, _ *struct{}) (*infoOutput, error) {
		out := &infoOutput{}
		out.Body.Name = d.Service
		out.Body.Version = d.Version
		return out, nil
	})
}

func registerReady(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-ready-check",
		Method:      http.MethodGet,
		Path:        "/readyz",
		Summary:     "就绪检查",
		Description: "检查 ai-service 依赖的 PostgreSQL 与 Redis 是否可用。",
		Tags:        []string{"meta"},
	}, func(ctx context.Context, _ *struct{}) (*readyOutput, error) {
		components := map[string]componentStatus{}
		ready := true

		if d.Postgres == nil {
			ready = false
			components["postgres"] = componentStatus{Status: "error", Error: "postgres dependency is not configured"}
		} else if err := d.Postgres.Ping(ctx); err != nil {
			ready = false
			components["postgres"] = componentStatus{Status: "error", Error: err.Error()}
		} else {
			components["postgres"] = componentStatus{Status: "ok"}
		}

		if d.Redis == nil {
			components["redis"] = componentStatus{Status: "disabled"}
		} else if err := d.Redis.Ping(ctx).Err(); err != nil {
			ready = false
			components["redis"] = componentStatus{Status: "error", Error: err.Error()}
		} else {
			components["redis"] = componentStatus{Status: "ok"}
		}

		if d.ServiceIdentity == nil {
			ready = false
			components["service_identity"] = componentStatus{Status: "error", Error: "service identity is not configured"}
		} else if ok, err := d.ServiceIdentity.Ready(); !ok {
			ready = false
			message := "service session has not been established"
			if err != nil {
				message = err.Error()
			}
			components["service_identity"] = componentStatus{Status: "error", Error: message}
		} else {
			components["service_identity"] = componentStatus{Status: "ok"}
		}

		out := &readyOutput{}
		out.Body.Components = components
		out.Body.Status = "ok"
		if !ready {
			out.Body.Status = "error"
			return out, httpx.ErrUnavailable.WithDetail("ai-service dependencies are not ready")
		}
		return out, nil
	})
}
