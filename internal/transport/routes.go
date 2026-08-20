package transport

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	announcementpkg "xiaodou/dai/internal/announcement"
	"xiaodou/dai/internal/auth"
	billingsvc "xiaodou/dai/internal/billing/service"
	cleanuppkg "xiaodou/dai/internal/cleanup"
	"xiaodou/dai/internal/config"
	invitepkg "xiaodou/dai/internal/invite"
	notificationpkg "xiaodou/dai/internal/notification"
	paymentsvc "xiaodou/dai/internal/payment/service"
	systempkg "xiaodou/dai/internal/system"
	userpkg "xiaodou/dai/internal/user"

	// AI 域
	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/commercial"
	aidb "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	proxypkg "xiaodou/dai/internal/ai/proxy"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/tokenrefresh"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
)

// InfrastructureDeps contains process-wide clients shared by transport
// adapters. It is kept as a named group so the composition root can replace
// these clients with ports when the legacy SQL handlers are migrated.
type InfrastructureDeps struct {
	Version string
	Pool    *pgxpool.Pool
	Redis   *redis.Client
	Logger  *zap.Logger
}

// PortalDeps contains transport policy and public-facing configuration.
type PortalDeps struct {
	// SecureCookies enables the Secure attribute for browser session cookies.
	// Development HTTP deployments may disable it; production wiring always enables it.
	SecureCookies bool
	Legal         config.LegalConfig
}

// IdentityDeps contains account, session, tenant and invitation use cases.
type IdentityDeps struct {
	JWT         *auth.JWTService
	Sessions    *auth.SessionService
	Activations *auth.ActivationService
	MFA         *auth.MFAService
	RecentAuth  *auth.RecentAuthService
	Blacklist   *auth.BlacklistService
	UserService *userpkg.UserService
	Invite      *invitepkg.InviteService
}

// BillingDeps contains payment and balance application services.
type BillingDeps struct {
	Deduction *billingsvc.DeductionService
	Payment   *paymentsvc.PaymentService
}

// OperationsDeps contains platform operations, notification and cleanup
// services. These are kept separate from identity and billing so future role
// binaries can omit them without changing transport signatures.
type OperationsDeps struct {
	Announcements *announcementpkg.Service
	Notifications *notificationpkg.Service
	Modules       *systempkg.Service
	ProxyNodes    *proxypkg.Service
	DataCleanup   *cleanuppkg.Service
}

// AIInfrastructureDeps contains generated query access and runtime-level
// policies needed while the legacy AI transport is being migrated.
type AIInfrastructureDeps struct {
	Queries         *aidb.Queries
	SecretMasterKey string
	AIHTTPClient    *http.Client
	Health          routing.HealthTracker
	Weights         aitransport.ScoreWeightsStore
	BanChecker      aitransport.HumaBanChecker
}

// AIIdentityDeps contains AI-side identity and workspace collaborators.
type AIIdentityDeps struct {
	OAuth          *pgadapter.OAuthCredentialStore
	TokenRefresher *tokenrefresh.Refresher
	APIKeySvc      *identitycontrol.Service
	WorkspaceSvc   *workspacesvc.Service
}

// AIBillingDeps contains AI-side subscription and billing collaborators.
type AIBillingDeps struct {
	Subscriptions *subscription.Service
}

// AICatalogDeps contains AI-side model, pricing and upstream collaborators.
type AICatalogDeps struct {
	ClientCatalog     *clientcatalog.Service
	PriceBookSvc      *billingcontrol.Service
	CommercialSvc     *commercial.Service
	GroupTransferSvc  *commercial.GroupTransferService
	AccountSvc        *upstreamcontrol.Service
	UpstreamAccessSvc *upstreamaccess.Service
}

// AIOperationsDeps contains AI-side dashboards, audit and risk-control collaborators.
type AIOperationsDeps struct {
	DashboardSvc         *observabilitycontrol.DashboardService
	UsageSvc             *observabilitycontrol.UsageService
	AuditSvc             *observabilitycontrol.AuditService
	RiskControlConfigSvc *riskcontrol.ConfigService
	RiskControlLogSvc    *riskcontrol.LogService
	RiskControlEventSvc  *riskcontrol.EventService
	RiskControlChecker   *riskcontrol.Checker
}

// AIDeps contains the AI control-plane and runtime services exposed by the
// AI transport module. It is intentionally separate from Deps so a role that
// only serves platform endpoints cannot accidentally receive AI services.
type AIDeps struct {
	AIInfrastructureDeps
	AIIdentityDeps
	AIBillingDeps
	AICatalogDeps
	AIOperationsDeps
}

// Deps 汇集平台 transport 层注册端点所需的显式领域依赖组。
//
// The embedded groups preserve the existing d.Field handler access pattern,
// but composition code must now name the owning group explicitly. AI
// dependencies are passed separately through AIDeps.
type Deps struct {
	InfrastructureDeps
	PortalDeps
	IdentityDeps
	BillingDeps
	OperationsDeps
}

// Module is a transport route module. Each module owns one explicit
// dependency bundle and can be registered independently by a future runtime
// role.
type Module interface {
	Register(api huma.API)
}

type aiModule struct {
	platform Deps
	deps     AIDeps
}

type aiIdentityProvider interface {
	aitransport.IdentityProvider
	aitransport.TenantEndUserVerifier
}

func (m aiModule) Register(api huma.API) {
	identity := newAIIdentityAdapter(m.platform.Pool, m.platform.UserService)
	aitransport.RegisterAI(api, buildAIDeps(m.platform, m.deps, identity))
}

// Register 在 Huma API 上注册平台端点和显式 AI 模块。
func Register(api huma.API, d Deps, ai AIDeps) {
	registerMeta(api, d)
	registerPublicPlane(api, d)
	modules := []Module{aiModule{platform: d, deps: ai}}
	for _, module := range modules {
		module.Register(api)
	}
}

func registerMeta(api huma.API, d Deps) {
	registerInfo(api, d.Version)
	registerJWKS(api, d.JWT)
}

func registerPublicPlane(api huma.API, d Deps) {
	registerAuthPublic(api, d)

	// Portal 账号端点（用户 JWT + 黑名单）
	usrAuth := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist)}
	registerAuthProtected(api, d, usrAuth)

	// 管理资源端点（/api/v1，用户 JWT + 类型守卫）
	registerAdminTenants(api, d)
	registerAdminUsers(api, d)
	registerAdminFinance(api, d)
	registerAdminUsageBilling(api, d)
	registerAdminDashboard(api, d)
	registerAdminEndUsers(api, d)
	registerAccount(api, d)
	registerTenantSelf(api, d)
	registerTenantBranding(api, d)
	registerJWTKeys(api, d)
	registerPayment(api, d)
	registerTenantCash(api, d)
	registerAdminPayment(api, d)
	registerAnnouncements(api, d)
	registerProxyNodes(api, d)
	registerNotifications(api, d)
	registerModules(api, d)
	registerDataCleanup(api, d)

	// 公开端点（无认证）
	registerPublic(api, d)

}

func buildAIDeps(platform Deps, d AIDeps, identity aiIdentityProvider) aitransport.AIDeps {
	aiDeps := aitransport.AIDeps{
		InfrastructureDeps: aitransport.InfrastructureDeps{
			Postgres:   platform.Pool,
			Redis:      platform.Redis,
			Queries:    d.Queries,
			Logger:     platform.Logger,
			HTTPClient: d.AIHTTPClient,
		},
		IdentityDeps: aitransport.IdentityDeps{
			OAuth:            d.OAuth,
			TokenRefresher:   d.TokenRefresher,
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
			APIKeySvc:        d.APIKeySvc,
			WorkspaceSvc:     d.WorkspaceSvc,
		},
		BillingDeps: aitransport.BillingDeps{
			Subscriptions: d.Subscriptions,
		},
		CatalogDeps: aitransport.CatalogDeps{
			ClientCatalog:     d.ClientCatalog,
			PriceBookSvc:      d.PriceBookSvc,
			CommercialSvc:     d.CommercialSvc,
			GroupTransferSvc:  d.GroupTransferSvc,
			AccountSvc:        d.AccountSvc,
			UpstreamAccessSvc: d.UpstreamAccessSvc,
		},
		RuntimeDeps: aitransport.RuntimeDeps{
			Health:          d.Health,
			Weights:         d.Weights,
			SecretMasterKey: d.SecretMasterKey,
		},
		OperationsDeps: aitransport.OperationsDeps{
			DashboardSvc:         d.DashboardSvc,
			UsageSvc:             d.UsageSvc,
			AuditSvc:             d.AuditSvc,
			RiskControlConfigSvc: d.RiskControlConfigSvc,
			RiskControlLogSvc:    d.RiskControlLogSvc,
			RiskControlEventSvc:  d.RiskControlEventSvc,
			RiskControlChecker:   d.RiskControlChecker,
		},
	}
	if identity != nil {
		aiDeps.IdentityProvider = identity
		aiDeps.TenantEndUsers = identity
	}
	return aiDeps
}

// RegisterRaw 注册非 JSON 契约的 chi 原生端点。
func RegisterRaw(mux *chi.Mux, d Deps) {
	RegisterPublicRaw(mux, d)
}

func RegisterPublicRaw(mux *chi.Mux, d Deps) {
	// 微信支付回调（无认证，验签即鉴权）
	notifyHandler := newPaymentNotifyHandlers(d)
	mux.Post("/api/v1/payments/wechat/notify", notifyHandler.wechatNotify)
	registerTenantBrandingRaw(mux, d)
}

var _ = http.MethodGet
