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
	"xiaodou/dai/internal/ai/identitycontrol"
	proxypkg "xiaodou/dai/internal/ai/proxy"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/subscription"
	aitransport "xiaodou/dai/internal/ai/transport"
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

// AIInfrastructureDeps contains runtime-level infrastructure policies used by
// AI transport.
type AIInfrastructureDeps struct {
	ProviderSecrets aitransport.ProviderSecretCodec
	AIHTTPClient    aitransport.HTTPDoer
	DatabaseHealth  aitransport.ComponentHealthProbe
	RedisHealth     aitransport.ComponentHealthProbe
	Health          routing.HealthTracker
	Weights         aitransport.ScoreWeightsStore
	BanChecker      aitransport.HumaBanChecker
}

// AIIdentityDeps contains AI-side identity and workspace collaborators.
type AIIdentityDeps struct {
	CredentialCreator aitransport.OAuthCredentialCreator
	CredentialReader  aitransport.OAuthCredentialReader
	CredentialWriter  aitransport.OAuthCredentialWriter
	PoolReader        aitransport.OAuthPoolReader
	PoolWriter        aitransport.OAuthPoolWriter
	PoolHealthReader  aitransport.OAuthPoolHealthReader
	TokenRefresher    aitransport.OAuthTokenRefresher
	APIKeySvc         *identitycontrol.Service
	WorkspaceSvc      *workspacesvc.Service
}

// AIBillingDeps contains AI-side subscription and billing collaborators.
type AIBillingDeps struct {
	Subscriptions *subscription.Service
}

// AICatalogDeps contains AI-side model, pricing and upstream collaborators.
type AICatalogDeps struct {
	ClientCatalog      aitransport.ClientCatalogResolver
	ModelCapabilities  aitransport.ModelCapabilityResolver
	AccountReader      aitransport.UpstreamAccountReader
	ModelBindings      aitransport.UpstreamModelBindingStore
	ModelCatalog       aitransport.ModelCatalogReader
	PriceBooks         aitransport.PriceBookReader
	PlatformPriceBooks aitransport.PlatformPriceBookManager
	TenantPriceBooks   aitransport.TenantPriceBookManager
	PriceBookSync      aitransport.PriceBookSyncManager
	Groups             aitransport.CommercialGroupCatalog
	GroupManager       aitransport.CommercialGroupManager
	DispatchRules      aitransport.CommercialDispatchRuleManager
	GroupTargets       aitransport.CommercialGroupTargetManager
	UserBindings       aitransport.CommercialUserBindingManager
	LimitPolicies      aitransport.CommercialLimitPolicyManager
	GroupTransfer      aitransport.GroupTransferManager
	Accounts           aitransport.UpstreamAccountCatalog
	AccountManager     aitransport.UpstreamAccountManager
	AccountHealth      aitransport.UpstreamAccountHealthWriter
	UpstreamAccess     aitransport.UpstreamAccessManager
}

// AIOperationsDeps contains AI-side dashboards, audit and risk-control collaborators.
type AIOperationsDeps struct {
	DashboardQueries           aitransport.DashboardQueryReader
	UsageQueries               aitransport.UsageQueryReader
	UserUsageLogs              aitransport.UserUsageLogReader
	AuditLogs                  aitransport.AdminAuditLogReader
	AdminAudit                 aitransport.AdminAuditRecorder
	IdentityEnrichmentFailures aitransport.IdentityEnrichmentFailureObserver
	RiskControlConfig          aitransport.RiskControlConfigStore
	RiskControlDetector        aitransport.RiskControlDetector
	RiskControlLogs            aitransport.RiskControlLogReader
	RiskEvents                 aitransport.RiskEventManager
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
			DatabaseHealth: d.DatabaseHealth,
			RedisHealth:    d.RedisHealth,
			HTTPClient:     d.AIHTTPClient,
		},
		IdentityDeps: aitransport.IdentityDeps{
			CredentialCreator: d.CredentialCreator,
			CredentialReader:  d.CredentialReader,
			CredentialWriter:  d.CredentialWriter,
			PoolReader:        d.PoolReader,
			PoolWriter:        d.PoolWriter,
			PoolHealthReader:  d.PoolHealthReader,
			TokenRefresher:    d.TokenRefresher,
			TokenVerifier:     platform.JWT,
			TokenRevocations:  platform.Blacklist,
			BanChecker:        d.BanChecker,
			APIKeySvc:         d.APIKeySvc,
			WorkspaceSvc:      d.WorkspaceSvc,
		},
		BillingDeps: aitransport.BillingDeps{
			Subscriptions: d.Subscriptions,
		},
		CatalogDeps: aitransport.CatalogDeps{
			ClientCatalog:      d.ClientCatalog,
			ModelCapabilities:  d.ModelCapabilities,
			AccountReader:      d.AccountReader,
			ModelBindings:      d.ModelBindings,
			ModelCatalog:       d.ModelCatalog,
			PriceBooks:         d.PriceBooks,
			PlatformPriceBooks: d.PlatformPriceBooks,
			TenantPriceBooks:   d.TenantPriceBooks,
			PriceBookSync:      d.PriceBookSync,
			Groups:             d.Groups,
			GroupManager:       d.GroupManager,
			DispatchRules:      d.DispatchRules,
			GroupTargets:       d.GroupTargets,
			UserBindings:       d.UserBindings,
			LimitPolicies:      d.LimitPolicies,
			GroupTransfer:      d.GroupTransfer,
			Accounts:           d.Accounts,
			AccountManager:     d.AccountManager,
			AccountHealth:      d.AccountHealth,
			UpstreamAccess:     d.UpstreamAccess,
		},
		RuntimeDeps: aitransport.RuntimeDeps{
			Health:          d.Health,
			Weights:         d.Weights,
			ProviderSecrets: d.ProviderSecrets,
		},
		OperationsDeps: aitransport.OperationsDeps{
			DashboardQueries:           d.DashboardQueries,
			UsageQueries:               d.UsageQueries,
			UserUsageLogs:              d.UserUsageLogs,
			AuditLogs:                  d.AuditLogs,
			AdminAudit:                 d.AdminAudit,
			IdentityEnrichmentFailures: d.IdentityEnrichmentFailures,
			RiskControlConfig:          d.RiskControlConfig,
			RiskControlDetector:        d.RiskControlDetector,
			RiskControlLogs:            d.RiskControlLogs,
			RiskEvents:                 d.RiskEvents,
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
