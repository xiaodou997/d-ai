package main

import (
	"go.uber.org/zap"

	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/transport"
)

// buildPlatformTransportModules projects concrete platform services into
// independently constructible route modules. Keeping this mapping here means
// neither aiModules nor transport needs a cross-domain service locator.
func buildPlatformTransportModules(version string, cfg *config.Config, platform *platformModules, ai transport.AIHTTPDeps, logger *zap.Logger) []transport.Module {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if platform == nil {
		platform = &platformModules{}
	}
	platformAuth := transport.PlatformAuthDeps{JWT: platform.JWT, Blacklist: platform.Blacklist}
	adminAuth := transport.AdminRouteAuthDeps{PlatformAuthDeps: platformAuth, Security: platform.Security, RecentAuth: platform.RecentAuth}
	return []transport.Module{
		transport.NewMetaModule(version, platform.JWT),
		transport.NewPlatformIdentityModule(transport.PlatformIdentityModuleDeps{
			Auth: transport.AuthModuleDeps{
				PlatformAuthDeps:  platformAuth,
				Security:          platform.Security,
				SecureCookies:     cfg.App.Env == "production",
				Sessions:          platform.Sessions,
				Activations:       platform.Activations,
				MFA:               platform.MFA,
				RecentAuth:        platform.RecentAuth,
				AuthRateLimiters:  platform.AuthRateLimiters,
				AuthAccountReader: platform.AuthAccounts,
				AuthAccountWriter: platform.AuthAccounts,
				AuthLoginReader:   platform.AuthAccounts,
				AuthAuditWriter:   platform.AuthAccounts,
				Logger:            logger,
			},
			Account: transport.AccountModuleDeps{
				PlatformAuthDeps: platformAuth,
				Queries:          platform.AccountQueries,
			},
			Tenant: transport.TenantSelfModuleDeps{
				PlatformAuthDeps: platformAuth,
				Service:          platform.TenantSelf,
			},
			Branding: transport.TenantBrandingModuleDeps{
				PlatformAuthDeps: platformAuth,
				Reader:           platform.TenantBranding,
				Writer:           platform.TenantBranding,
			},
			Public:  transport.PublicModuleDeps{Invite: platform.Invite, Legal: cfg.Legal},
			JWTKeys: transport.JWTKeysModuleDeps{PlatformAuthDeps: platformAuth},
		}),
		transport.NewPlatformAdminModule(transport.PlatformAdminModuleDeps{
			Tenants: transport.AdminTenantModuleDeps{
				AdminRouteAuthDeps: adminAuth,
				TenantReader:       platform.TenantRepo,
				TenantStatusWriter: platform.TenantRepo,
				TenantWriter:       platform.TenantRepo,
				Activations:        platform.Activations,
			},
			Users: transport.AdminUsersModuleDeps{
				AdminRouteAuthDeps: adminAuth,
				AdminAccounts:      platform.AdminAccounts,
				AdminAccountWriter: platform.AdminAccounts,
				AccountLifecycle:   platform.AdminAccountLifecycle,
				Activations:        platform.Activations,
			},
			Finance: transport.AdminFinanceModuleDeps{
				AdminRouteAuthDeps: adminAuth,
				Deduction:          platform.Deduction,
				AccountQueries:     platform.AccountQueries,
				Recharge:           platform.Recharge,
				AuthAuditLogs:      platform.AuthAccounts,
			},
			UsageBilling: transport.AdminUsageBillingModuleDeps{
				AdminRouteAuthDeps: adminAuth,
				Deduction:          platform.Deduction,
			},
			Dashboard: transport.AdminDashboardModuleDeps{
				AdminRouteAuthDeps: adminAuth,
				Dashboard:          platform.Dashboard,
			},
			EndUsers: transport.AdminEndUsersModuleDeps{
				AdminRouteAuthDeps: adminAuth,
				AdminEndUsers:      platform.AdminEndUsers,
				AdminEndUserWriter: platform.AdminEndUsers,
				Activations:        platform.Activations,
			},
		}),
		transport.NewPlatformBillingModule(transport.PlatformBillingModuleDeps{
			Payment: transport.PaymentModuleDeps{
				PlatformAuthDeps: platformAuth,
				Service:          platform.Payment,
				Logger:           logger,
			},
		}),
		transport.NewPlatformOperationsModule(transport.PlatformOperationsModuleDeps{
			Announcements: transport.AnnouncementModuleDeps{PlatformAuthDeps: platformAuth, Service: platform.Announcements},
			Notifications: transport.NotificationModuleDeps{PlatformAuthDeps: platformAuth, Service: platform.Notifications},
			System:        transport.SystemModuleDeps{PlatformAuthDeps: platformAuth, Service: platform.Modules},
			DataCleanup:   transport.DataCleanupModuleDeps{PlatformAuthDeps: platformAuth, Service: platform.DataCleanup},
			ProxyNodes:    transport.ProxyNodesModuleDeps{PlatformAuthDeps: platformAuth, Service: platform.ProxyNodes},
		}),
		transport.NewAIModule(transport.AIPlatformModuleDeps{
			PlatformAuthDeps: platformAuth,
			TenantReader:     platform.TenantRepo,
			IdentityReader:   platform.UserService,
		}, ai),
	}
}
