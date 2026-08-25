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
	authports "xiaodou/dai/internal/auth/ports"
	billingports "xiaodou/dai/internal/billing/ports"
	billingsvc "xiaodou/dai/internal/billing/service"
	cleanuppkg "xiaodou/dai/internal/cleanup"
	"xiaodou/dai/internal/config"
	inviteports "xiaodou/dai/internal/invite/ports"
	notificationpkg "xiaodou/dai/internal/notification"
	paymentsvc "xiaodou/dai/internal/payment/service"
	systempkg "xiaodou/dai/internal/system"
	systemports "xiaodou/dai/internal/system/ports"
	tenantports "xiaodou/dai/internal/tenant/ports"
	userpkg "xiaodou/dai/internal/user"
	userports "xiaodou/dai/internal/user/ports"

	// AI 域
	proxypkg "xiaodou/dai/internal/ai/proxy"
	"xiaodou/dai/internal/ai/routing"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/ai/workspace"
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
	JWT                  *auth.JWTService
	Sessions             *auth.SessionService
	Activations          *auth.ActivationService
	MFA                  *auth.MFAService
	RecentAuth           *auth.RecentAuthService
	Blacklist            *auth.BlacklistService
	UserService          *userpkg.UserService
	AuthAccountReader    authports.AccountReader
	AuthAccountWriter    authports.AccountWriter
	AuthLoginReader      authports.LoginReader
	AuthAuditWriter      authports.AuthAuditRecorder
	AuthAuditLogs        authports.AuthAuditLogReader
	TenantStatusWriter   tenantports.AdminTenantStatusWriter
	TenantWriter         tenantports.AdminTenantWriter
	TenantReader         tenantports.AdminTenantReader
	TenantBrandingReader tenantports.PortalBrandingReader
	TenantBrandingWriter tenantports.PortalBrandingWriter
	TenantSelf           tenantports.TenantSelfService
	AdminAccounts        userports.AdminAccountReader
	AdminAccountWriter   userports.AdminAccountWriter
	AdminEndUsers        userports.AdminEndUserReader
	AdminEndUserWriter   userports.AdminEndUserWriter
	Invite               inviteports.PublicService
}

// BillingDeps contains payment and balance application services.
type BillingDeps struct {
	AccountQueries billingports.AccountQueryReader
	Deduction      *billingsvc.DeductionService
	Recharge       *billingsvc.RechargeService
	Payment        *paymentsvc.PaymentService
}

// OperationsDeps contains platform operations, notification and cleanup
// services. These are kept separate from identity and billing so future role
// binaries can omit them without changing transport signatures.
type OperationsDeps struct {
	Announcements *announcementpkg.Service
	Notifications *notificationpkg.Service
	Modules       *systempkg.Service
	Dashboard     systemports.AdminDashboardReader
	ProxyNodes    *proxypkg.Service
	DataCleanup   *cleanuppkg.Service
}

// AISubscriptionHTTPDeps contains the collaborators owned by the independently
// registered subscription HTTP module.
type AISubscriptionHTTPDeps struct {
	SubscriptionPlans          aitransport.SubscriptionPlanCatalog
	SubscriptionPlanWriter     aitransport.SubscriptionPlanManager
	SubscriptionPurchases      aitransport.SubscriptionPurchaser
	Subscriptions              aitransport.SubscriptionReader
	SubscriptionOrders         aitransport.SubscriptionOrderReader
	SubscriptionGroupNames     aitransport.SubscriptionGroupNameResolver
	BanChecker                 aitransport.HumaBanChecker
	IdentityEnrichmentFailures aitransport.IdentityEnrichmentFailureObserver
}

// AICoreHTTPDeps contains the narrow collaborators owned by the remaining AI
// core platform price-book and limit-policy routes.
type AICoreHTTPDeps struct {
	PlatformPriceBooks         aitransport.PlatformPriceBookManager
	PriceBookSync              aitransport.PriceBookSyncManager
	LimitPolicies              aitransport.CommercialLimitPolicyManager
	BanChecker                 aitransport.HumaBanChecker
	IdentityEnrichmentFailures aitransport.IdentityEnrichmentFailureObserver
}

// AIAuditLogHTTPDeps contains the collaborators owned by the independently
// registered management audit-log HTTP module.
type AIAuditLogHTTPDeps struct {
	AuditLogs  aitransport.AdminAuditLogReader
	BanChecker aitransport.HumaBanChecker
}

// AIDashboardHTTPDeps contains the collaborators owned by the independently
// registered management dashboard HTTP module.
type AIDashboardHTTPDeps struct {
	DashboardQueries           aitransport.DashboardQueryReader
	BanChecker                 aitransport.HumaBanChecker
	IdentityEnrichmentFailures aitransport.IdentityEnrichmentFailureObserver
}

// AIUsageHTTPDeps contains the collaborators owned by the independently
// registered management usage HTTP module.
type AIUsageHTTPDeps struct {
	UsageQueries               aitransport.UsageQueryReader
	BanChecker                 aitransport.HumaBanChecker
	IdentityEnrichmentFailures aitransport.IdentityEnrichmentFailureObserver
}

// AIOAuthManagementHTTPDeps contains the collaborators owned by the
// independently registered OAuth pool and credential management module.
type AIOAuthManagementHTTPDeps struct {
	CredentialCreator aitransport.OAuthCredentialCreator
	CredentialReader  aitransport.OAuthCredentialReader
	CredentialWriter  aitransport.OAuthCredentialWriter
	PoolReader        aitransport.OAuthPoolReader
	PoolWriter        aitransport.OAuthPoolWriter
	PoolHealthReader  aitransport.OAuthPoolHealthReader
	TokenRefresher    aitransport.OAuthTokenRefresher
	ClientCatalog     aitransport.ClientCatalogResolver
	ModelBindings     aitransport.UpstreamModelBindingStore
	BanChecker        aitransport.HumaBanChecker
}

// AIModelBindingHTTPDeps contains the collaborators owned by the independently
// registered account and OAuth-pool model-binding HTTP module.
type AIModelBindingHTTPDeps struct {
	AccountReader aitransport.UpstreamAccountReader
	PoolReader    aitransport.OAuthPoolReader
	ModelBindings aitransport.UpstreamModelBindingStore
	BanChecker    aitransport.HumaBanChecker
}

// AIUpstreamDiagnosticsHTTPDeps contains the collaborators owned by the
// independently registered upstream discovery and connectivity HTTP module.
type AIUpstreamDiagnosticsHTTPDeps struct {
	AccountReader     aitransport.UpstreamAccountReader
	ModelBindings     aitransport.UpstreamModelBindingStore
	ProviderSecrets   aitransport.ProviderSecretCodec
	HTTPClient        aitransport.HTTPDoer
	AccountHealth     aitransport.UpstreamAccountHealthWriter
	ModelCapabilities aitransport.ModelCapabilityResolver
	BanChecker        aitransport.HumaBanChecker
}

// AIUpstreamAccountManagementHTTPDeps contains the collaborators owned by
// the independently registered direct upstream-account CRUD and transfer
// module.
type AIUpstreamAccountManagementHTTPDeps struct {
	Accounts        aitransport.UpstreamAccountCatalog
	AccountManager  aitransport.UpstreamAccountManager
	AccountReader   aitransport.UpstreamAccountReader
	ProviderSecrets aitransport.ProviderSecretCodec
	ModelBindings   aitransport.UpstreamModelBindingStore
	PriceBooks      aitransport.PriceBookReader
	AdminAudit      aitransport.AdminAuditRecorder
	BanChecker      aitransport.HumaBanChecker
}

// AIUpstreamAccessManagementHTTPDeps contains the collaborators owned by the
// independently registered platform-admin upstream-access policy module.
type AIUpstreamAccessManagementHTTPDeps struct {
	UpstreamAccess aitransport.UpstreamAccessManager
	BanChecker     aitransport.HumaBanChecker
}

// AITenantCatalogHTTPDeps contains the collaborators owned by the
// independently registered tenant self-service catalog module.
type AITenantCatalogHTTPDeps struct {
	ModelCatalog     aitransport.ModelCatalogReader
	Groups           aitransport.CommercialGroupCatalog
	TenantPriceBooks aitransport.TenantPriceBookManager
	PriceBookSync    aitransport.PriceBookSyncManager
	BanChecker       aitransport.HumaBanChecker
}

// AITenantSelfControlHTTPDeps contains the collaborators owned by the
// independently registered tenant API-key and limit-policy module.
type AITenantSelfControlHTTPDeps struct {
	APIKeys         aitransport.APIKeyReader
	APIKeyWriter    aitransport.APIKeyWriter
	APIKeyLifecycle aitransport.APIKeyLifecycleManager
	APIKeySecrets   aitransport.APIKeySecretManager
	Groups          aitransport.CommercialGroupCatalog
	LimitPolicies   aitransport.CommercialLimitPolicyManager
	BanChecker      aitransport.HumaBanChecker
}

// AITenantGroupManagementHTTPDeps contains the collaborators owned by the
// independently registered tenant group and transfer module.
type AITenantGroupManagementHTTPDeps struct {
	Groups           aitransport.CommercialGroupCatalog
	GroupManager     aitransport.CommercialGroupManager
	DispatchRules    aitransport.CommercialDispatchRuleManager
	GroupTargets     aitransport.CommercialGroupTargetManager
	UserBindings     aitransport.CommercialUserBindingManager
	TenantPriceBooks aitransport.TenantPriceBookManager
	GroupTransfer    aitransport.GroupTransferManager
	AdminAudit       aitransport.AdminAuditRecorder
	BanChecker       aitransport.HumaBanChecker
}

// AIAPIKeyManagementHTTPDeps contains the collaborators owned by the
// independently registered platform-admin API-key management module.
type AIAPIKeyManagementHTTPDeps struct {
	APIKeys         aitransport.APIKeyReader
	APIKeyWriter    aitransport.APIKeyWriter
	APIKeyLifecycle aitransport.APIKeyLifecycleManager
	APIKeySecrets   aitransport.APIKeySecretManager
	Groups          aitransport.CommercialGroupCatalog
	LimitPolicies   aitransport.CommercialLimitPolicyManager
	BanChecker      aitransport.HumaBanChecker
}

// AITenantSelfReadHTTPDeps contains the collaborators owned by the
// independently registered tenant dashboard and usage read module.
type AITenantSelfReadHTTPDeps struct {
	DashboardQueries aitransport.DashboardQueryReader
	UsageQueries     aitransport.UsageQueryReader
	BanChecker       aitransport.HumaBanChecker
}

// AIWorkspaceHTTPDeps contains the collaborators owned by the independently
// registered tenant and end-user workspace module.
type AIWorkspaceHTTPDeps struct {
	WorkspaceOverview workspace.OverviewReader
	WorkspaceModels   workspace.ChatModelReader
	WorkspaceSessions workspace.ChatSessionReader
	WorkspaceManager  workspace.ChatSessionManager
	WorkspaceImages   workspace.ImageJobReader
	DashboardQueries  aitransport.DashboardQueryReader
	UsageQueries      aitransport.UsageQueryReader
	BanChecker        aitransport.HumaBanChecker
}

// AIUserSelfControlHTTPDeps contains the collaborators owned by the
// independently registered end-user API-key and limit-policy module.
type AIUserSelfControlHTTPDeps struct {
	APIKeys         aitransport.APIKeyReader
	APIKeyWriter    aitransport.APIKeyWriter
	APIKeyLifecycle aitransport.APIKeyLifecycleManager
	APIKeySecrets   aitransport.APIKeySecretManager
	Groups          aitransport.CommercialGroupCatalog
	LimitPolicies   aitransport.CommercialLimitPolicyManager
	BanChecker      aitransport.HumaBanChecker
}

// AIUserSelfReadHTTPDeps contains the collaborators owned by the
// independently registered end-user group, model-grant and usage read module.
type AIUserSelfReadHTTPDeps struct {
	Groups        aitransport.CommercialGroupCatalog
	ModelCatalog  aitransport.ModelCatalogReader
	UserUsageLogs aitransport.UserUsageLogReader
	UsageQueries  aitransport.UsageQueryReader
	BanChecker    aitransport.HumaBanChecker
}

// AISystemHTTPDeps contains the collaborators owned by the independently
// registered system status and route-weight HTTP module.
type AISystemHTTPDeps struct {
	DatabaseHealth aitransport.ComponentHealthProbe
	RedisHealth    aitransport.ComponentHealthProbe
	Health         routing.HealthTracker
	Weights        aitransport.ScoreWeightsStore
	BanChecker     aitransport.HumaBanChecker
}

// AIRiskControlHTTPDeps contains the collaborators owned by the independently
// registered risk-control HTTP module.
type AIRiskControlHTTPDeps struct {
	ProviderSecrets     aitransport.ProviderSecretCodec
	RiskControlConfig   aitransport.RiskControlConfigStore
	RiskControlDetector aitransport.RiskControlDetector
	RiskControlLogs     aitransport.RiskControlLogReader
	RiskEvents          aitransport.RiskEventManager
	BanChecker          aitransport.HumaBanChecker
}

// AIHTTPDeps is a composition-only collection of independently registered AI
// route modules. Handlers receive the narrower module dependency type.
type AIHTTPDeps struct {
	Core                AICoreHTTPDeps
	Subscriptions       AISubscriptionHTTPDeps
	RiskControl         AIRiskControlHTTPDeps
	AuditLog            AIAuditLogHTTPDeps
	System              AISystemHTTPDeps
	Dashboard           AIDashboardHTTPDeps
	Usage               AIUsageHTTPDeps
	OAuthManagement     AIOAuthManagementHTTPDeps
	ModelBindings       AIModelBindingHTTPDeps
	UpstreamDiagnostics AIUpstreamDiagnosticsHTTPDeps
	UpstreamAccounts    AIUpstreamAccountManagementHTTPDeps
	UpstreamAccess      AIUpstreamAccessManagementHTTPDeps
	TenantCatalog       AITenantCatalogHTTPDeps
	TenantSelfControl   AITenantSelfControlHTTPDeps
	TenantGroups        AITenantGroupManagementHTTPDeps
	APIKeyManagement    AIAPIKeyManagementHTTPDeps
	TenantSelfRead      AITenantSelfReadHTTPDeps
	Workspace           AIWorkspaceHTTPDeps
	UserSelfControl     AIUserSelfControlHTTPDeps
	UserSelfRead        AIUserSelfReadHTTPDeps
}

// Deps 汇集平台 transport 层注册端点所需的显式领域依赖组。
//
// The embedded groups preserve the existing d.Field handler access pattern,
// but composition code must now name the owning group explicitly. AI
// dependencies are passed separately through AIHTTPDeps modules.
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
	deps     AIHTTPDeps
}

type aiIdentityProvider interface {
	aitransport.IdentityProvider
	aitransport.TenantEndUserVerifier
}

func (m aiModule) Register(api huma.API) {
	identity := newAIIdentityAdapter(m.platform.TenantReader, m.platform.UserService)
	aitransport.RegisterAICore(api, buildAICoreHTTPDeps(m.platform, m.deps.Core, identity))
	aitransport.RegisterSubscriptions(api, buildSubscriptionHTTPDeps(m.platform, m.deps.Subscriptions, identity))
	aitransport.RegisterRiskControl(api, buildRiskControlHTTPDeps(m.platform, m.deps.RiskControl))
	aitransport.RegisterAuditLog(api, buildAuditLogHTTPDeps(m.platform, m.deps.AuditLog))
	aitransport.RegisterSystem(api, buildSystemHTTPDeps(m.platform, m.deps.System))
	aitransport.RegisterDashboard(api, buildDashboardHTTPDeps(m.platform, m.deps.Dashboard, identity))
	aitransport.RegisterUsage(api, buildUsageHTTPDeps(m.platform, m.deps.Usage, identity))
	aitransport.RegisterOAuthManagement(api, buildOAuthManagementHTTPDeps(m.platform, m.deps.OAuthManagement))
	aitransport.RegisterModelBindings(api, buildModelBindingHTTPDeps(m.platform, m.deps.ModelBindings))
	aitransport.RegisterUpstreamDiagnostics(api, buildUpstreamDiagnosticsHTTPDeps(m.platform, m.deps.UpstreamDiagnostics))
	aitransport.RegisterUpstreamAccountManagement(api, buildUpstreamAccountManagementHTTPDeps(m.platform, m.deps.UpstreamAccounts))
	aitransport.RegisterUpstreamAccessManagement(api, buildUpstreamAccessManagementHTTPDeps(m.platform, m.deps.UpstreamAccess))
	aitransport.RegisterTenantCatalog(api, buildTenantCatalogHTTPDeps(m.platform, m.deps.TenantCatalog))
	aitransport.RegisterTenantSelfControl(api, buildTenantSelfControlHTTPDeps(m.platform, m.deps.TenantSelfControl, identity))
	aitransport.RegisterTenantGroupManagement(api, buildTenantGroupManagementHTTPDeps(m.platform, m.deps.TenantGroups, identity))
	aitransport.RegisterAPIKeyManagement(api, buildAPIKeyManagementHTTPDeps(m.platform, m.deps.APIKeyManagement))
	aitransport.RegisterTenantSelfRead(api, buildTenantSelfReadHTTPDeps(m.platform, m.deps.TenantSelfRead))
	aitransport.RegisterWorkspace(api, buildWorkspaceHTTPDeps(m.platform, m.deps.Workspace))
	aitransport.RegisterUserSelfControl(api, buildUserSelfControlHTTPDeps(m.platform, m.deps.UserSelfControl))
	aitransport.RegisterUserSelfRead(api, buildUserSelfReadHTTPDeps(m.platform, m.deps.UserSelfRead))
}

// Register 在 Huma API 上注册平台端点和显式 AI 模块。
func Register(api huma.API, d Deps, ai AIHTTPDeps) {
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

func buildAICoreHTTPDeps(platform Deps, d AICoreHTTPDeps, identity aiIdentityProvider) aitransport.CoreHTTPDeps {
	aiDeps := aitransport.CoreHTTPDeps{
		TokenVerifier:              platform.JWT,
		TokenRevocations:           platform.Blacklist,
		BanChecker:                 d.BanChecker,
		PlatformPriceBooks:         d.PlatformPriceBooks,
		PriceBookSync:              d.PriceBookSync,
		LimitPolicies:              d.LimitPolicies,
		IdentityEnrichmentFailures: d.IdentityEnrichmentFailures,
	}
	if identity != nil {
		aiDeps.IdentityProvider = identity
	}
	return aiDeps
}

func buildUpstreamAccountManagementHTTPDeps(platform Deps, d AIUpstreamAccountManagementHTTPDeps) aitransport.UpstreamAccountManagementHTTPDeps {
	return aitransport.UpstreamAccountManagementHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		Accounts:        d.Accounts,
		AccountManager:  d.AccountManager,
		AccountReader:   d.AccountReader,
		ProviderSecrets: d.ProviderSecrets,
		ModelBindings:   d.ModelBindings,
		PriceBooks:      d.PriceBooks,
		AdminAudit:      d.AdminAudit,
	}
}

func buildUpstreamAccessManagementHTTPDeps(platform Deps, d AIUpstreamAccessManagementHTTPDeps) aitransport.UpstreamAccessManagementHTTPDeps {
	return aitransport.UpstreamAccessManagementHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		UpstreamAccess: d.UpstreamAccess,
	}
}

func buildTenantCatalogHTTPDeps(platform Deps, d AITenantCatalogHTTPDeps) aitransport.TenantCatalogHTTPDeps {
	return aitransport.TenantCatalogHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		ModelCatalog:     d.ModelCatalog,
		Groups:           d.Groups,
		TenantPriceBooks: d.TenantPriceBooks,
		PriceBookSync:    d.PriceBookSync,
	}
}

func buildTenantSelfControlHTTPDeps(platform Deps, d AITenantSelfControlHTTPDeps, identity aiIdentityProvider) aitransport.TenantSelfControlHTTPDeps {
	return aitransport.TenantSelfControlHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		APIKeys:         d.APIKeys,
		APIKeyWriter:    d.APIKeyWriter,
		APIKeyLifecycle: d.APIKeyLifecycle,
		APIKeySecrets:   d.APIKeySecrets,
		Groups:          d.Groups,
		LimitPolicies:   d.LimitPolicies,
		TenantEndUsers:  identity,
	}
}

func buildTenantGroupManagementHTTPDeps(platform Deps, d AITenantGroupManagementHTTPDeps, identity aiIdentityProvider) aitransport.TenantGroupManagementHTTPDeps {
	return aitransport.TenantGroupManagementHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		Groups:           d.Groups,
		GroupManager:     d.GroupManager,
		DispatchRules:    d.DispatchRules,
		GroupTargets:     d.GroupTargets,
		UserBindings:     d.UserBindings,
		TenantEndUsers:   identity,
		TenantPriceBooks: d.TenantPriceBooks,
		GroupTransfer:    d.GroupTransfer,
		AdminAudit:       d.AdminAudit,
	}
}

func buildAPIKeyManagementHTTPDeps(platform Deps, d AIAPIKeyManagementHTTPDeps) aitransport.APIKeyManagementHTTPDeps {
	return aitransport.APIKeyManagementHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		APIKeys:         d.APIKeys,
		APIKeyWriter:    d.APIKeyWriter,
		APIKeyLifecycle: d.APIKeyLifecycle,
		APIKeySecrets:   d.APIKeySecrets,
		Groups:          d.Groups,
		LimitPolicies:   d.LimitPolicies,
	}
}

func buildTenantSelfReadHTTPDeps(platform Deps, d AITenantSelfReadHTTPDeps) aitransport.TenantSelfReadHTTPDeps {
	return aitransport.TenantSelfReadHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		DashboardQueries: d.DashboardQueries,
		UsageQueries:     d.UsageQueries,
	}
}

func buildWorkspaceHTTPDeps(platform Deps, d AIWorkspaceHTTPDeps) aitransport.WorkspaceHTTPDeps {
	auth := aitransport.HTTPAuthDeps{
		TokenVerifier:    platform.JWT,
		TokenRevocations: platform.Blacklist,
		BanChecker:       d.BanChecker,
	}
	return aitransport.WorkspaceHTTPDeps{
		TenantAuth:        auth,
		UserAuth:          auth,
		WorkspaceOverview: d.WorkspaceOverview,
		WorkspaceModels:   d.WorkspaceModels,
		WorkspaceSessions: d.WorkspaceSessions,
		WorkspaceManager:  d.WorkspaceManager,
		WorkspaceImages:   d.WorkspaceImages,
		DashboardQueries:  d.DashboardQueries,
		UsageQueries:      d.UsageQueries,
	}
}

func buildUserSelfControlHTTPDeps(platform Deps, d AIUserSelfControlHTTPDeps) aitransport.UserSelfControlHTTPDeps {
	return aitransport.UserSelfControlHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		APIKeys:         d.APIKeys,
		APIKeyWriter:    d.APIKeyWriter,
		APIKeyLifecycle: d.APIKeyLifecycle,
		APIKeySecrets:   d.APIKeySecrets,
		Groups:          d.Groups,
		LimitPolicies:   d.LimitPolicies,
	}
}

func buildUserSelfReadHTTPDeps(platform Deps, d AIUserSelfReadHTTPDeps) aitransport.UserSelfReadHTTPDeps {
	return aitransport.UserSelfReadHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		Groups:        d.Groups,
		ModelCatalog:  d.ModelCatalog,
		UserUsageLogs: d.UserUsageLogs,
		UsageQueries:  d.UsageQueries,
	}
}

func buildSubscriptionHTTPDeps(platform Deps, d AISubscriptionHTTPDeps, identity aiIdentityProvider) aitransport.SubscriptionHTTPDeps {
	deps := aitransport.SubscriptionHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		SubscriptionPlans:          d.SubscriptionPlans,
		SubscriptionPlanWriter:     d.SubscriptionPlanWriter,
		SubscriptionPurchases:      d.SubscriptionPurchases,
		Subscriptions:              d.Subscriptions,
		SubscriptionOrders:         d.SubscriptionOrders,
		SubscriptionGroupNames:     d.SubscriptionGroupNames,
		IdentityEnrichmentFailures: d.IdentityEnrichmentFailures,
	}
	if identity != nil {
		deps.IdentityProvider = identity
	}
	return deps
}

func buildRiskControlHTTPDeps(platform Deps, d AIRiskControlHTTPDeps) aitransport.RiskControlHTTPDeps {
	return aitransport.RiskControlHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		ProviderSecrets:     d.ProviderSecrets,
		RiskControlConfig:   d.RiskControlConfig,
		RiskControlDetector: d.RiskControlDetector,
		RiskControlLogs:     d.RiskControlLogs,
		RiskEvents:          d.RiskEvents,
	}
}

func buildAuditLogHTTPDeps(platform Deps, d AIAuditLogHTTPDeps) aitransport.AuditLogHTTPDeps {
	return aitransport.AuditLogHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		AuditLogs: d.AuditLogs,
	}
}

func buildSystemHTTPDeps(platform Deps, d AISystemHTTPDeps) aitransport.SystemHTTPDeps {
	return aitransport.SystemHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		DatabaseHealth: d.DatabaseHealth,
		RedisHealth:    d.RedisHealth,
		Health:         d.Health,
		Weights:        d.Weights,
	}
}

func buildDashboardHTTPDeps(platform Deps, d AIDashboardHTTPDeps, identity aiIdentityProvider) aitransport.DashboardHTTPDeps {
	deps := aitransport.DashboardHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		DashboardQueries:           d.DashboardQueries,
		IdentityEnrichmentFailures: d.IdentityEnrichmentFailures,
	}
	if identity != nil {
		deps.IdentityProvider = identity
	}
	return deps
}

func buildUsageHTTPDeps(platform Deps, d AIUsageHTTPDeps, identity aiIdentityProvider) aitransport.UsageHTTPDeps {
	deps := aitransport.UsageHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		UsageQueries:               d.UsageQueries,
		IdentityEnrichmentFailures: d.IdentityEnrichmentFailures,
	}
	if identity != nil {
		deps.IdentityProvider = identity
	}
	return deps
}

func buildOAuthManagementHTTPDeps(platform Deps, d AIOAuthManagementHTTPDeps) aitransport.OAuthManagementHTTPDeps {
	return aitransport.OAuthManagementHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		CredentialCreator: d.CredentialCreator,
		CredentialReader:  d.CredentialReader,
		CredentialWriter:  d.CredentialWriter,
		PoolReader:        d.PoolReader,
		PoolWriter:        d.PoolWriter,
		PoolHealthReader:  d.PoolHealthReader,
		TokenRefresher:    d.TokenRefresher,
		ClientCatalog:     d.ClientCatalog,
		ModelBindings:     d.ModelBindings,
	}
}

func buildModelBindingHTTPDeps(platform Deps, d AIModelBindingHTTPDeps) aitransport.ModelBindingHTTPDeps {
	return aitransport.ModelBindingHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		AccountReader: d.AccountReader,
		PoolReader:    d.PoolReader,
		ModelBindings: d.ModelBindings,
	}
}

func buildUpstreamDiagnosticsHTTPDeps(platform Deps, d AIUpstreamDiagnosticsHTTPDeps) aitransport.UpstreamDiagnosticsHTTPDeps {
	return aitransport.UpstreamDiagnosticsHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		AccountReader:     d.AccountReader,
		ModelBindings:     d.ModelBindings,
		ProviderSecrets:   d.ProviderSecrets,
		HTTPClient:        d.HTTPClient,
		AccountHealth:     d.AccountHealth,
		ModelCapabilities: d.ModelCapabilities,
	}
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
