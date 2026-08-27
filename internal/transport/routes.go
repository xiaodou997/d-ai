package transport

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
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
	userports "xiaodou/dai/internal/user/ports"

	// AI 域
	proxypkg "xiaodou/dai/internal/ai/proxy"
	"xiaodou/dai/internal/ai/routing"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/ai/workspace"
)

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

// Module is a transport route module. Each module owns one explicit
// dependency bundle and can be registered independently by a future runtime
// role.
type Module interface {
	Register(api huma.API)
}

type metaModule struct {
	version string
	jwt     *auth.JWTService
}

type platformAuthDeps struct {
	JWT       *auth.JWTService
	Blacklist *auth.BlacklistService
}

type aiPlatformDeps struct {
	platformAuthDeps
	TenantReader   tenantports.AdminTenantReader
	IdentityReader userports.IdentityUserReader
}

type authModule struct {
	platformAuthDeps
	Security          authports.AccountSecurityWriter
	SecureCookies     bool
	Sessions          *auth.SessionService
	Activations       *auth.ActivationService
	MFA               *auth.MFAService
	RecentAuth        *auth.RecentAuthService
	AuthRateLimiters  *auth.RateLimiters
	AuthAccountReader authports.AccountReader
	AuthAccountWriter authports.AccountWriter
	AuthLoginReader   authports.LoginReader
	AuthAuditWriter   authports.AuthAuditRecorder
	Logger            *zap.Logger
}

type announcementModule struct {
	auth    platformAuthDeps
	service *announcementpkg.Service
}

type notificationModule struct {
	auth    platformAuthDeps
	service notificationpkg.HTTPService
}

type systemModule struct {
	auth    platformAuthDeps
	service *systempkg.Service
}

type dataCleanupModule struct {
	auth    platformAuthDeps
	service *cleanuppkg.Service
}

type proxyNodesModule struct {
	auth    platformAuthDeps
	service *proxypkg.Service
}

type paymentModule struct {
	auth    platformAuthDeps
	service *paymentsvc.PaymentService
	logger  *zap.Logger
}

type accountModule struct {
	auth    platformAuthDeps
	queries billingports.AccountQueryReader
}

type tenantSelfModule struct {
	auth    platformAuthDeps
	service tenantports.TenantSelfService
}

type tenantBrandingModule struct {
	auth   platformAuthDeps
	reader tenantports.PortalBrandingReader
	writer tenantports.PortalBrandingWriter
}

type publicModule struct {
	invite inviteports.PublicService
	legal  config.LegalConfig
}

type jwtKeysModule struct {
	auth platformAuthDeps
}

type platformOperationsModule struct {
	announcements announcementModule
	notifications notificationModule
	system        systemModule
	dataCleanup   dataCleanupModule
	proxyNodes    proxyNodesModule
}

type platformBillingModule struct {
	payment paymentModule
}

type platformIdentityModule struct {
	auth     authModule
	account  accountModule
	tenant   tenantSelfModule
	branding tenantBrandingModule
	public   publicModule
	jwtKeys  jwtKeysModule
}

type adminRouteAuth struct {
	platformAuthDeps
	Security   authports.AccountSecurityWriter
	RecentAuth *auth.RecentAuthService
}

type adminTenantModule struct {
	adminRouteAuth
	TenantReader       tenantports.AdminTenantReader
	TenantStatusWriter tenantports.AdminTenantStatusWriter
	TenantWriter       tenantports.AdminTenantWriter
	Activations        *auth.ActivationService
}

type adminUsersModule struct {
	adminRouteAuth
	TenantReader       tenantports.AdminTenantReader
	AdminAccounts      userports.AdminAccountReader
	AdminAccountWriter userports.AdminAccountWriter
	Activations        *auth.ActivationService
}

type adminFinanceModule struct {
	adminRouteAuth
	TenantReader   tenantports.AdminTenantReader
	Deduction      *billingsvc.DeductionService
	AccountQueries billingports.AccountQueryReader
	Recharge       *billingsvc.RechargeService
	AuthAuditLogs  authports.AuthAuditLogReader
}

type adminUsageBillingModule struct {
	adminRouteAuth
	Deduction *billingsvc.DeductionService
}

type adminDashboardModule struct {
	adminRouteAuth
	Dashboard systemports.AdminDashboardReader
}

type adminEndUsersModule struct {
	adminRouteAuth
	TenantReader       tenantports.AdminTenantReader
	AdminEndUsers      userports.AdminEndUserReader
	AdminEndUserWriter userports.AdminEndUserWriter
	Activations        *auth.ActivationService
}

type platformAdminModule struct {
	tenants      adminTenantModule
	users        adminUsersModule
	finance      adminFinanceModule
	usageBilling adminUsageBillingModule
	dashboard    adminDashboardModule
	endUsers     adminEndUsersModule
}

type aiModule struct {
	platform aiPlatformDeps
	deps     AIHTTPDeps
}

var _ Module = metaModule{}
var _ Module = platformOperationsModule{}
var _ Module = platformBillingModule{}
var _ Module = platformIdentityModule{}
var _ Module = platformAdminModule{}
var _ Module = aiModule{}

func (m metaModule) Register(api huma.API) {
	registerInfo(api, m.version)
	registerJWKS(api, m.jwt)
}

func (m platformOperationsModule) Register(api huma.API) {
	registerAnnouncements(api, m.announcements)
	registerNotifications(api, m.notifications)
	registerModules(api, m.system)
	registerDataCleanup(api, m.dataCleanup)
	registerProxyNodes(api, m.proxyNodes)
}

func (m platformBillingModule) Register(api huma.API) {
	registerPayment(api, m.payment)
	registerTenantCash(api, m.payment)
	registerAdminPayment(api, m.payment)
}

func (m platformIdentityModule) Register(api huma.API) {
	registerAuthPublic(api, m.auth)
	registerAuthProtected(api, m.auth, huma.Middlewares{userAuth(api, m.auth.JWT, m.auth.Blacklist)})
	registerAccount(api, m.account)
	registerTenantSelf(api, m.tenant)
	registerTenantBranding(api, m.branding)
	registerPublic(api, m.public)
	registerJWTKeys(api, m.jwtKeys)
}

func (m platformAdminModule) Register(api huma.API) {
	registerAdminTenants(api, m.tenants)
	registerAdminUsers(api, m.users)
	registerAdminFinance(api, m.finance)
	registerAdminUsageBilling(api, m.usageBilling)
	registerAdminDashboard(api, m.dashboard)
	registerAdminEndUsers(api, m.endUsers)
}

type aiIdentityProvider interface {
	aitransport.IdentityProvider
	aitransport.TenantEndUserVerifier
}

func (m aiModule) Register(api huma.API) {
	identity := newAIIdentityAdapter(m.platform.TenantReader, m.platform.IdentityReader)
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

// Register attaches explicitly constructed modules to a Huma API. The
// composition root owns dependency assembly; this function intentionally does
// not accept a cross-domain service locator.
func Register(api huma.API, modules ...Module) {
	for _, module := range modules {
		if module != nil {
			module.Register(api)
		}
	}
}

func buildAICoreHTTPDeps(platform aiPlatformDeps, d AICoreHTTPDeps, identity aiIdentityProvider) aitransport.CoreHTTPDeps {
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

func buildUpstreamAccountManagementHTTPDeps(platform aiPlatformDeps, d AIUpstreamAccountManagementHTTPDeps) aitransport.UpstreamAccountManagementHTTPDeps {
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

func buildUpstreamAccessManagementHTTPDeps(platform aiPlatformDeps, d AIUpstreamAccessManagementHTTPDeps) aitransport.UpstreamAccessManagementHTTPDeps {
	return aitransport.UpstreamAccessManagementHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		UpstreamAccess: d.UpstreamAccess,
	}
}

func buildTenantCatalogHTTPDeps(platform aiPlatformDeps, d AITenantCatalogHTTPDeps) aitransport.TenantCatalogHTTPDeps {
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

func buildTenantSelfControlHTTPDeps(platform aiPlatformDeps, d AITenantSelfControlHTTPDeps, identity aiIdentityProvider) aitransport.TenantSelfControlHTTPDeps {
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

func buildTenantGroupManagementHTTPDeps(platform aiPlatformDeps, d AITenantGroupManagementHTTPDeps, identity aiIdentityProvider) aitransport.TenantGroupManagementHTTPDeps {
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

func buildAPIKeyManagementHTTPDeps(platform aiPlatformDeps, d AIAPIKeyManagementHTTPDeps) aitransport.APIKeyManagementHTTPDeps {
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

func buildTenantSelfReadHTTPDeps(platform aiPlatformDeps, d AITenantSelfReadHTTPDeps) aitransport.TenantSelfReadHTTPDeps {
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

func buildWorkspaceHTTPDeps(platform aiPlatformDeps, d AIWorkspaceHTTPDeps) aitransport.WorkspaceHTTPDeps {
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

func buildUserSelfControlHTTPDeps(platform aiPlatformDeps, d AIUserSelfControlHTTPDeps) aitransport.UserSelfControlHTTPDeps {
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

func buildUserSelfReadHTTPDeps(platform aiPlatformDeps, d AIUserSelfReadHTTPDeps) aitransport.UserSelfReadHTTPDeps {
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

func buildSubscriptionHTTPDeps(platform aiPlatformDeps, d AISubscriptionHTTPDeps, identity aiIdentityProvider) aitransport.SubscriptionHTTPDeps {
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

func buildRiskControlHTTPDeps(platform aiPlatformDeps, d AIRiskControlHTTPDeps) aitransport.RiskControlHTTPDeps {
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

func buildAuditLogHTTPDeps(platform aiPlatformDeps, d AIAuditLogHTTPDeps) aitransport.AuditLogHTTPDeps {
	return aitransport.AuditLogHTTPDeps{
		Auth: aitransport.HTTPAuthDeps{
			TokenVerifier:    platform.JWT,
			TokenRevocations: platform.Blacklist,
			BanChecker:       d.BanChecker,
		},
		AuditLogs: d.AuditLogs,
	}
}

func buildSystemHTTPDeps(platform aiPlatformDeps, d AISystemHTTPDeps) aitransport.SystemHTTPDeps {
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

func buildDashboardHTTPDeps(platform aiPlatformDeps, d AIDashboardHTTPDeps, identity aiIdentityProvider) aitransport.DashboardHTTPDeps {
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

func buildUsageHTTPDeps(platform aiPlatformDeps, d AIUsageHTTPDeps, identity aiIdentityProvider) aitransport.UsageHTTPDeps {
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

func buildOAuthManagementHTTPDeps(platform aiPlatformDeps, d AIOAuthManagementHTTPDeps) aitransport.OAuthManagementHTTPDeps {
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

func buildModelBindingHTTPDeps(platform aiPlatformDeps, d AIModelBindingHTTPDeps) aitransport.ModelBindingHTTPDeps {
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

func buildUpstreamDiagnosticsHTTPDeps(platform aiPlatformDeps, d AIUpstreamDiagnosticsHTTPDeps) aitransport.UpstreamDiagnosticsHTTPDeps {
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

// RawDeps contains only the collaborators needed by chi-native endpoints.
// These routes intentionally bypass Huma for signature-preserving callbacks
// and binary responses, so they must not receive the full platform container.
type RawDeps struct {
	Payment              *paymentsvc.PaymentService
	TenantBrandingReader tenantports.PortalBrandingReader
	Logger               *zap.Logger
}

// RegisterRaw 注册非 JSON 契约的 chi 原生端点。
func RegisterRaw(mux *chi.Mux, d RawDeps) {
	RegisterPublicRaw(mux, d)
}

func RegisterPublicRaw(mux *chi.Mux, d RawDeps) {
	// 微信支付回调（无认证，验签即鉴权）
	notifyHandler := newPaymentNotifyHandlers(paymentModule{
		service: d.Payment,
		logger:  d.Logger,
	})
	mux.Post("/api/v1/payments/wechat/notify", notifyHandler.wechatNotify)
	registerTenantBrandingRaw(mux, tenantBrandingModule{reader: d.TenantBrandingReader})
}

var _ = http.MethodGet
