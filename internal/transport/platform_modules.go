package transport

import (
	"context"
	"go.uber.org/zap"

	proxypkg "xiaodou/dai/internal/ai/proxy"
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
)

// PlatformAuthDeps is the common authentication projection used by platform
// route modules. It contains no database, cache, or business service.
type PlatformAuthDeps struct {
	JWT       *auth.JWTService
	Blacklist *auth.BlacklistService
}

// AuthModuleDeps contains only the collaborators used by authentication
// routes.
type AuthModuleDeps struct {
	PlatformAuthDeps
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

// AccountModuleDeps contains the account balance/read capability used by the
// platform account routes.
type AccountModuleDeps struct {
	PlatformAuthDeps
	Queries billingports.AccountQueryReader
}

// TenantSelfModuleDeps contains the tenant self-service capability.
type TenantSelfModuleDeps struct {
	PlatformAuthDeps
	Service tenantports.TenantSelfService
}

// TenantBrandingModuleDeps contains the portal branding read/write ports.
type TenantBrandingModuleDeps struct {
	PlatformAuthDeps
	Reader tenantports.PortalBrandingReader
	Writer tenantports.PortalBrandingWriter
}

// PublicModuleDeps contains public invitation and legal-policy inputs.
type PublicModuleDeps struct {
	Invite inviteports.PublicService
	Legal  config.LegalConfig
}

// JWTKeysModuleDeps contains the JWT key-management authentication inputs.
type JWTKeysModuleDeps struct {
	PlatformAuthDeps
}

// PlatformIdentityModuleDeps groups independently registered platform
// identity modules without exposing a service locator.
type PlatformIdentityModuleDeps struct {
	Auth     AuthModuleDeps
	Account  AccountModuleDeps
	Tenant   TenantSelfModuleDeps
	Branding TenantBrandingModuleDeps
	Public   PublicModuleDeps
	JWTKeys  JWTKeysModuleDeps
}

// AdminRouteAuthDeps contains the auth policy shared by one administrator
// route module.
type AdminRouteAuthDeps struct {
	PlatformAuthDeps
	Security   authports.AccountSecurityWriter
	RecentAuth *auth.RecentAuthService
}

// AdminTenantModuleDeps contains administrator tenant-management routes.
type AdminTenantModuleDeps struct {
	AdminRouteAuthDeps
	TenantReader    tenantports.AdminTenantReader
	TenantLifecycle tenantports.AdminTenantLifecycle
	TenantWriter    tenantports.AdminTenantWriter
	Activations     *auth.ActivationService
}

// AdminUsersModuleDeps contains system-admin and tenant-user management
// routes.
type AdminUsersModuleDeps struct {
	AdminRouteAuthDeps
	AdminAccounts      userports.AdminAccountReader
	AdminAccountWriter userports.AdminAccountWriter
	AccountLifecycle   userports.AdminAccountLifecycle
	Activations        *auth.ActivationService
}

// AdminFinanceModuleDeps contains administrator finance and auth-audit routes.
type AdminFinanceModuleDeps struct {
	AdminRouteAuthDeps
	Deduction      *billingsvc.DeductionService
	AccountQueries billingports.AccountQueryReader
	Recharge       *billingsvc.RechargeService
	AuthAuditLogs  authports.AuthAuditLogReader
}

// AdminUsageBillingModuleDeps contains administrator batch usage refund
// routes.
type AdminUsageBillingModuleDeps struct {
	AdminRouteAuthDeps
	Deduction *billingsvc.DeductionService
}

// AdminDashboardModuleDeps contains administrator dashboard routes.
type AdminDashboardModuleDeps struct {
	AdminRouteAuthDeps
	Dashboard systemports.AdminDashboardReader
}

// AdminEndUsersModuleDeps contains administrator end-user management routes.
type AdminEndUsersModuleDeps struct {
	AdminRouteAuthDeps
	AdminEndUsers      userports.AdminEndUserReader
	AdminEndUserWriter userports.AdminEndUserWriter
	EndUserLifecycle   userports.AdminEndUserLifecycle
	Activations        *auth.ActivationService
}

// PlatformAdminModuleDeps groups administrator routes by use case.
type PlatformAdminModuleDeps struct {
	Tenants      AdminTenantModuleDeps
	Users        AdminUsersModuleDeps
	Finance      AdminFinanceModuleDeps
	UsageBilling AdminUsageBillingModuleDeps
	Dashboard    AdminDashboardModuleDeps
	EndUsers     AdminEndUsersModuleDeps
}

// PaymentModuleDeps contains platform payment routes and their callback
// logging dependency.
type PaymentModuleDeps struct {
	PlatformAuthDeps
	Service *paymentsvc.PaymentService
	Logger  *zap.Logger
}

// PlatformBillingModuleDeps groups the platform billing route module.
type PlatformBillingModuleDeps struct {
	Payment PaymentModuleDeps
}

// AnnouncementModuleDeps contains announcement routes.
type AnnouncementModuleDeps struct {
	PlatformAuthDeps
	Service AnnouncementHTTPService
}

// AnnouncementHTTPService is the narrow application surface consumed by
// announcement HTTP handlers. The concrete service remains owned by the
// composition root and is not exposed through transport module wiring.
type AnnouncementHTTPService interface {
	GetManaged(context.Context, announcementpkg.Actor, string) (announcementpkg.Announcement, error)
	DeleteDraft(context.Context, announcementpkg.Actor, string) error
	ListManaged(context.Context, announcementpkg.Actor, announcementpkg.ManageQuery) (announcementpkg.ManagedPage, error)
	ListRecipients(context.Context, announcementpkg.Actor, string, announcementpkg.RecipientQuery) (announcementpkg.RecipientPage, error)
	Archive(context.Context, announcementpkg.Actor, string) (announcementpkg.Announcement, error)
	GetStats(context.Context, announcementpkg.Actor, string) (announcementpkg.Stats, error)
	ListInbox(context.Context, announcementpkg.Principal, announcementpkg.InboxQuery) (announcementpkg.InboxPage, error)
	MarkRead(context.Context, announcementpkg.Principal, string) error
	GetVisible(context.Context, announcementpkg.Principal, string) (announcementpkg.InboxItem, error)
	CreateDraft(context.Context, announcementpkg.Actor, announcementpkg.DraftInput) (announcementpkg.Announcement, error)
	Publish(context.Context, announcementpkg.Actor, string) (announcementpkg.Announcement, error)
	UpdateDraft(context.Context, announcementpkg.Actor, string, announcementpkg.DraftInput) (announcementpkg.Announcement, error)
}

// NotificationModuleDeps contains notification routes.
type NotificationModuleDeps struct {
	PlatformAuthDeps
	Service notificationpkg.HTTPService
}

// SystemModuleDeps contains system-module routes.
type SystemModuleDeps struct {
	PlatformAuthDeps
	Service SystemHTTPService
}

type SystemHTTPService interface {
	List(context.Context) ([]systempkg.Status, error)
	Get(context.Context, string) (systempkg.Status, error)
	SetEnabled(context.Context, string, bool, string) (systempkg.Status, error)
	GetPIIConfig(context.Context) (systempkg.PIIConfig, error)
	UpdatePIIConfig(context.Context, systempkg.PIIConfig, string) (systempkg.PIIConfig, error)
}

// DataCleanupModuleDeps contains data-cleanup routes.
type DataCleanupModuleDeps struct {
	PlatformAuthDeps
	Service DataCleanupHTTPService
}

type DataCleanupHTTPService interface {
	GetPolicy(context.Context) (cleanuppkg.Policy, error)
	UpdatePolicy(context.Context, cleanuppkg.Policy, string) (cleanuppkg.Policy, error)
	Preview(context.Context) (cleanuppkg.Preview, error)
	ListRuns(context.Context) ([]cleanuppkg.Run, error)
	StartManual([]string, string) (cleanuppkg.Run, error)
}

// ProxyNodesModuleDeps contains proxy-node routes.
type ProxyNodesModuleDeps struct {
	PlatformAuthDeps
	Service ProxyNodesHTTPService
}

type ProxyNodesHTTPService interface {
	List(context.Context) ([]proxypkg.Node, error)
	Upsert(context.Context, string, proxypkg.UpsertInput, string) (proxypkg.Node, error)
	Delete(context.Context, string) error
}

// PlatformOperationsModuleDeps groups platform operations route modules.
type PlatformOperationsModuleDeps struct {
	Announcements AnnouncementModuleDeps
	Notifications NotificationModuleDeps
	System        SystemModuleDeps
	DataCleanup   DataCleanupModuleDeps
	ProxyNodes    ProxyNodesModuleDeps
}

// AIPlatformModuleDeps contains only platform capabilities shared by AI HTTP
// modules.
type AIPlatformModuleDeps struct {
	PlatformAuthDeps
	TenantReader   tenantports.AdminTenantReader
	IdentityReader userports.IdentityUserReader
}

// NewMetaModule creates the service metadata/JWKS module.
func NewMetaModule(version string, jwt *auth.JWTService) Module {
	return metaModule{version: version, jwt: jwt}
}

// NewPlatformIdentityModule creates the platform identity route module.
func NewPlatformIdentityModule(d PlatformIdentityModuleDeps) Module {
	return platformIdentityModule{
		auth: authModule{
			platformAuthDeps:  platformAuthDeps{JWT: d.Auth.JWT, Blacklist: d.Auth.Blacklist},
			Security:          d.Auth.Security,
			SecureCookies:     d.Auth.SecureCookies,
			Sessions:          d.Auth.Sessions,
			Activations:       d.Auth.Activations,
			MFA:               d.Auth.MFA,
			RecentAuth:        d.Auth.RecentAuth,
			AuthRateLimiters:  d.Auth.AuthRateLimiters,
			AuthAccountReader: d.Auth.AuthAccountReader,
			AuthAccountWriter: d.Auth.AuthAccountWriter,
			AuthLoginReader:   d.Auth.AuthLoginReader,
			AuthAuditWriter:   d.Auth.AuthAuditWriter,
			Logger:            d.Auth.Logger,
		},
		account: accountModule{
			auth:    platformAuthDeps{JWT: d.Account.JWT, Blacklist: d.Account.Blacklist},
			queries: d.Account.Queries,
		},
		tenant: tenantSelfModule{
			auth:    platformAuthDeps{JWT: d.Tenant.JWT, Blacklist: d.Tenant.Blacklist},
			service: d.Tenant.Service,
		},
		branding: tenantBrandingModule{
			auth:   platformAuthDeps{JWT: d.Branding.JWT, Blacklist: d.Branding.Blacklist},
			reader: d.Branding.Reader,
			writer: d.Branding.Writer,
		},
		public: publicModule{
			invite: d.Public.Invite,
			legal:  d.Public.Legal,
		},
		jwtKeys: jwtKeysModule{auth: platformAuthDeps{JWT: d.JWTKeys.JWT, Blacklist: d.JWTKeys.Blacklist}},
	}
}

func toAdminRouteAuth(d AdminRouteAuthDeps) adminRouteAuth {
	return adminRouteAuth{
		platformAuthDeps: platformAuthDeps{JWT: d.JWT, Blacklist: d.Blacklist},
		Security:         d.Security,
		RecentAuth:       d.RecentAuth,
	}
}

// NewPlatformAdminModule creates the six administrator route submodules.
func NewPlatformAdminModule(d PlatformAdminModuleDeps) Module {
	return platformAdminModule{
		tenants: adminTenantModule{
			adminRouteAuth:  toAdminRouteAuth(d.Tenants.AdminRouteAuthDeps),
			TenantReader:    d.Tenants.TenantReader,
			TenantLifecycle: d.Tenants.TenantLifecycle,
			TenantWriter:    d.Tenants.TenantWriter,
			Activations:     d.Tenants.Activations,
		},
		users: adminUsersModule{
			adminRouteAuth:     toAdminRouteAuth(d.Users.AdminRouteAuthDeps),
			AdminAccounts:      d.Users.AdminAccounts,
			AdminAccountWriter: d.Users.AdminAccountWriter,
			AccountLifecycle:   d.Users.AccountLifecycle,
			Activations:        d.Users.Activations,
		},
		finance: adminFinanceModule{
			adminRouteAuth: toAdminRouteAuth(d.Finance.AdminRouteAuthDeps),
			Deduction:      d.Finance.Deduction,
			AccountQueries: d.Finance.AccountQueries,
			Recharge:       d.Finance.Recharge,
			AuthAuditLogs:  d.Finance.AuthAuditLogs,
		},
		usageBilling: adminUsageBillingModule{
			adminRouteAuth: toAdminRouteAuth(d.UsageBilling.AdminRouteAuthDeps),
			Deduction:      d.UsageBilling.Deduction,
		},
		dashboard: adminDashboardModule{
			adminRouteAuth: toAdminRouteAuth(d.Dashboard.AdminRouteAuthDeps),
			Dashboard:      d.Dashboard.Dashboard,
		},
		endUsers: adminEndUsersModule{
			adminRouteAuth:     toAdminRouteAuth(d.EndUsers.AdminRouteAuthDeps),
			AdminEndUsers:      d.EndUsers.AdminEndUsers,
			AdminEndUserWriter: d.EndUsers.AdminEndUserWriter,
			EndUserLifecycle:   d.EndUsers.EndUserLifecycle,
			Activations:        d.EndUsers.Activations,
		},
	}
}

// NewPlatformBillingModule creates the payment route module.
func NewPlatformBillingModule(d PlatformBillingModuleDeps) Module {
	return platformBillingModule{payment: paymentModule{
		auth:    platformAuthDeps{JWT: d.Payment.JWT, Blacklist: d.Payment.Blacklist},
		service: d.Payment.Service,
		logger:  d.Payment.Logger,
	}}
}

// NewPlatformOperationsModule creates the operations route modules.
func NewPlatformOperationsModule(d PlatformOperationsModuleDeps) Module {
	return platformOperationsModule{
		announcements: announcementModule{
			auth:    platformAuthDeps{JWT: d.Announcements.JWT, Blacklist: d.Announcements.Blacklist},
			service: d.Announcements.Service,
		},
		notifications: notificationModule{
			auth:    platformAuthDeps{JWT: d.Notifications.JWT, Blacklist: d.Notifications.Blacklist},
			service: d.Notifications.Service,
		},
		system: systemModule{
			auth:    platformAuthDeps{JWT: d.System.JWT, Blacklist: d.System.Blacklist},
			service: d.System.Service,
		},
		dataCleanup: dataCleanupModule{
			auth:    platformAuthDeps{JWT: d.DataCleanup.JWT, Blacklist: d.DataCleanup.Blacklist},
			service: d.DataCleanup.Service,
		},
		proxyNodes: proxyNodesModule{
			auth:    platformAuthDeps{JWT: d.ProxyNodes.JWT, Blacklist: d.ProxyNodes.Blacklist},
			service: d.ProxyNodes.Service,
		},
	}
}

// NewAIModule creates the AI HTTP route module with explicit platform ports.
func NewAIModule(platform AIPlatformModuleDeps, deps AIHTTPDeps) Module {
	return aiModule{
		platform: aiPlatformDeps{
			platformAuthDeps: platformAuthDeps{JWT: platform.JWT, Blacklist: platform.Blacklist},
			TenantReader:     platform.TenantReader,
			IdentityReader:   platform.IdentityReader,
		},
		deps: deps,
	}
}
