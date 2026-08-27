package main

import (
	"go.uber.org/zap"

	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/transport"
)

// buildPlatformTransportDeps is the composition-root projection from concrete
// platform services to the route registration contract. Keeping this mapping
// here prevents aiModules from owning or forwarding platform HTTP dependencies.
func buildPlatformTransportDeps(version string, cfg *config.Config, platform *platformModules, logger *zap.Logger) transport.Deps {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	deps := transport.Deps{
		InfrastructureDeps: transport.InfrastructureDeps{
			Version: version,
			Logger:  logger,
		},
		PortalDeps: transport.PortalDeps{
			SecureCookies: cfg.App.Env == "production",
			Legal:         cfg.Legal,
		},
	}
	if platform == nil {
		return deps
	}
	deps.IdentityDeps = transport.IdentityDeps{
		JWT:                  platform.JWT,
		Sessions:             platform.Sessions,
		Activations:          platform.Activations,
		MFA:                  platform.MFA,
		RecentAuth:           platform.RecentAuth,
		Blacklist:            platform.Blacklist,
		IdentityReader:       platform.UserService,
		AuthAccountReader:    platform.AuthAccounts,
		AuthAccountWriter:    platform.AuthAccounts,
		AuthLoginReader:      platform.AuthAccounts,
		AuthAuditWriter:      platform.AuthAccounts,
		AuthAuditLogs:        platform.AuthAccounts,
		AuthRateLimiters:     platform.AuthRateLimiters,
		TenantStatusWriter:   platform.TenantRepo,
		TenantWriter:         platform.TenantRepo,
		TenantReader:         platform.TenantRepo,
		TenantBrandingReader: platform.TenantBranding,
		TenantBrandingWriter: platform.TenantBranding,
		TenantSelf:           platform.TenantSelf,
		AdminAccounts:        platform.AdminAccounts,
		AdminAccountWriter:   platform.AdminAccounts,
		AdminEndUsers:        platform.AdminEndUsers,
		AdminEndUserWriter:   platform.AdminEndUsers,
		Invite:               platform.Invite,
	}
	deps.BillingDeps = transport.BillingDeps{
		AccountQueries: platform.AccountQueries,
		Deduction:      platform.Deduction,
		Recharge:       platform.Recharge,
		Payment:        platform.Payment,
	}
	deps.OperationsDeps = transport.OperationsDeps{
		Announcements: platform.Announcements,
		Notifications: platform.Notifications,
		Modules:       platform.Modules,
		Dashboard:     platform.Dashboard,
		ProxyNodes:    platform.ProxyNodes,
		DataCleanup:   platform.DataCleanup,
	}
	return deps
}
