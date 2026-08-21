package transport

import (
	"context"
	"net/http"
	"testing"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/tokenrefresh"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/internal/auth"
)

func TestBuildAIDepsWiresRuntimeManagementDependencies(t *testing.T) {
	oauth := &pgadapter.OAuthCredentialStore{}
	refresher := &tokenrefresh.Refresher{}
	catalog := &clientcatalog.Service{}
	modelCapabilities := &modelCapabilityResolverStub{}
	httpClient := &httpDoerStub{}
	databaseHealth := &componentHealthProbeStub{}
	redisHealth := &componentHealthProbeStub{}
	health := routing.DefaultInMemoryTracker()
	weights := &pgadapter.RouteWeightsStore{}
	blacklist := &auth.BlacklistService{}
	jwt := &auth.JWTService{}
	banChecker := &humaBanCheckerStub{}
	identity := &aiIdentityProviderStub{}
	providerSecrets := &providerSecretCodecStub{}
	accountReader := &upstreamAccountReaderStub{}
	modelBindings := &upstreamModelBindingStoreStub{}
	modelCatalog := &modelCatalogReaderStub{}
	priceBooks := &priceBookReaderStub{}
	priceBookPorts := billingcontrol.New(nil, nil)
	userUsageLogs := &userUsageLogReaderStub{}
	usageQueries := observabilitycontrol.NewUsageService(nil)
	dashboardQueries := &dashboardQueryReaderStub{}
	auditLogs := &adminAuditLogReaderStub{}
	adminAudit := &adminAuditRecorderStub{}
	identityEnrichmentFailures := &identityEnrichmentFailureObserverStub{}
	accountPorts := upstreamcontrol.New(nil, nil)
	riskConfig := riskcontrol.NewConfigService(nil)
	riskDetector := &riskcontrol.Checker{}
	riskLogs := riskcontrol.NewLogService(nil)
	riskEvents := riskcontrol.NewEventService(nil)
	upstreamAccess := upstreamaccess.New(nil)
	commercialPorts := commercial.NewService(nil)
	apiKeyPorts := identitycontrol.New(nil, nil, nil, nil)
	workspacePorts := workspace.NewService(nil)
	subscriptionPorts := subscription.NewService(nil, nil, nil)
	groupTransfer := commercial.NewGroupTransferService(nil, commercial.GroupTransferOptions{})

	got := buildAIDeps(
		Deps{IdentityDeps: IdentityDeps{Blacklist: blacklist}},
		AIDeps{
			AIIdentityDeps: AIIdentityDeps{
				BanChecker:        banChecker,
				APIKeys:           apiKeyPorts,
				APIKeyWriter:      apiKeyPorts,
				APIKeyLifecycle:   apiKeyPorts,
				APIKeySecrets:     apiKeyPorts,
				WorkspaceOverview: workspacePorts,
				WorkspaceModels:   workspacePorts,
				WorkspaceSessions: workspacePorts,
				WorkspaceManager:  workspacePorts,
				WorkspaceImages:   workspacePorts,
			},
			AICatalogDeps: AICatalogDeps{
				ModelCatalog:       modelCatalog,
				PlatformPriceBooks: priceBookPorts,
				TenantPriceBooks:   priceBookPorts,
				PriceBookSync:      priceBookPorts,
				Groups:             commercialPorts,
				GroupManager:       commercialPorts,
				DispatchRules:      commercialPorts,
				GroupTargets:       commercialPorts,
				UserBindings:       commercialPorts,
				LimitPolicies:      commercialPorts,
				UpstreamAccess:     upstreamAccess,
				GroupTransfer:      groupTransfer,
			},
			AIOperationsDeps: AIOperationsDeps{
				IdentityEnrichmentFailures: identityEnrichmentFailures,
				UserUsageLogs:              userUsageLogs,
				UsageQueries:               usageQueries,
				DashboardQueries:           dashboardQueries,
				AdminAudit:                 adminAudit,
			},
		},
		nil,
	)

	if got.APIKeys != apiKeyPorts || got.APIKeyWriter != apiKeyPorts || got.APIKeyLifecycle != apiKeyPorts || got.APIKeySecrets != apiKeyPorts {
		t.Fatal("API key capability ports were not preserved")
	}
	if got.WorkspaceOverview != workspacePorts || got.WorkspaceModels != workspacePorts || got.WorkspaceSessions != workspacePorts || got.WorkspaceManager != workspacePorts || got.WorkspaceImages != workspacePorts {
		t.Fatal("workspace capability ports were not preserved")
	}
	if got.ModelCatalog != modelCatalog {
		t.Fatal("catalog dependencies were not preserved")
	}
	if got.PlatformPriceBooks != priceBookPorts || got.TenantPriceBooks != priceBookPorts || got.PriceBookSync != priceBookPorts {
		t.Fatal("price book ports were not preserved")
	}
	if got.Groups != commercialPorts || got.GroupManager != commercialPorts || got.DispatchRules != commercialPorts || got.GroupTargets != commercialPorts || got.UserBindings != commercialPorts || got.LimitPolicies != commercialPorts {
		t.Fatal("commercial control-plane ports were not preserved")
	}
	if got.UpstreamAccess != upstreamAccess {
		t.Fatal("upstream access manager was not preserved")
	}
	if got.GroupTransfer != groupTransfer {
		t.Fatal("group transfer manager was not preserved")
	}
	if got.IdentityEnrichmentFailures != identityEnrichmentFailures {
		t.Fatal("observability dependencies were not preserved")
	}
	if got.UserUsageLogs != userUsageLogs {
		t.Fatal("user usage log reader was not preserved")
	}
	if got.UsageQueries != usageQueries {
		t.Fatal("usage query reader was not preserved")
	}
	if got.DashboardQueries != dashboardQueries {
		t.Fatal("dashboard query reader was not preserved")
	}
	if got.AdminAudit != adminAudit {
		t.Fatal("admin audit recorder was not preserved")
	}
	if got.TokenRevocations != blacklist {
		t.Fatal("portal token revocation dependency was not preserved")
	}

	subscriptions := buildSubscriptionHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AISubscriptionHTTPDeps{
			SubscriptionPlans:          subscriptionPorts,
			SubscriptionPlanWriter:     subscriptionPorts,
			SubscriptionPurchases:      subscriptionPorts,
			Subscriptions:              subscriptionPorts,
			SubscriptionOrders:         subscriptionPorts,
			SubscriptionGroupNames:     subscriptionPorts,
			BanChecker:                 banChecker,
			IdentityEnrichmentFailures: identityEnrichmentFailures,
		},
		identity,
	)
	if subscriptions.Auth.TokenVerifier != jwt || subscriptions.Auth.TokenRevocations != blacklist || subscriptions.Auth.BanChecker != banChecker {
		t.Fatal("subscription auth dependencies were not preserved")
	}
	if subscriptions.SubscriptionPlans != subscriptionPorts || subscriptions.SubscriptionPlanWriter != subscriptionPorts || subscriptions.SubscriptionPurchases != subscriptionPorts || subscriptions.Subscriptions != subscriptionPorts || subscriptions.SubscriptionOrders != subscriptionPorts || subscriptions.SubscriptionGroupNames != subscriptionPorts {
		t.Fatal("subscription capability ports were not preserved")
	}
	if subscriptions.IdentityEnrichmentFailures != identityEnrichmentFailures {
		t.Fatal("subscription identity enrichment observer was not preserved")
	}
	if subscriptions.IdentityProvider != identity {
		t.Fatal("subscription identity provider was not preserved")
	}

	riskControl := buildRiskControlHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIRiskControlHTTPDeps{
			ProviderSecrets:     providerSecrets,
			RiskControlConfig:   riskConfig,
			RiskControlDetector: riskDetector,
			RiskControlLogs:     riskLogs,
			RiskEvents:          riskEvents,
			BanChecker:          banChecker,
		},
	)
	if riskControl.Auth.TokenVerifier != jwt || riskControl.Auth.TokenRevocations != blacklist || riskControl.Auth.BanChecker != banChecker {
		t.Fatal("risk-control auth dependencies were not preserved")
	}
	if riskControl.ProviderSecrets != providerSecrets || riskControl.RiskControlConfig != riskConfig || riskControl.RiskControlDetector != riskDetector || riskControl.RiskControlLogs != riskLogs || riskControl.RiskEvents != riskEvents {
		t.Fatal("risk-control dependencies were not preserved")
	}

	auditLog := buildAuditLogHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIAuditLogHTTPDeps{
			AuditLogs:  auditLogs,
			BanChecker: banChecker,
		},
	)
	if auditLog.Auth.TokenVerifier != jwt || auditLog.Auth.TokenRevocations != blacklist || auditLog.Auth.BanChecker != banChecker {
		t.Fatal("audit-log auth dependencies were not preserved")
	}
	if auditLog.AuditLogs != auditLogs {
		t.Fatal("admin audit log reader was not preserved")
	}

	system := buildSystemHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AISystemHTTPDeps{
			DatabaseHealth: databaseHealth,
			RedisHealth:    redisHealth,
			Health:         health,
			Weights:        weights,
			BanChecker:     banChecker,
		},
	)
	if system.Auth.TokenVerifier != jwt || system.Auth.TokenRevocations != blacklist || system.Auth.BanChecker != banChecker {
		t.Fatal("system auth dependencies were not preserved")
	}
	if system.DatabaseHealth != databaseHealth || system.RedisHealth != redisHealth || system.Health != health || system.Weights != weights {
		t.Fatal("system dependencies were not preserved")
	}

	dashboard := buildDashboardHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIDashboardHTTPDeps{
			DashboardQueries:           dashboardQueries,
			BanChecker:                 banChecker,
			IdentityEnrichmentFailures: identityEnrichmentFailures,
		},
		identity,
	)
	if dashboard.Auth.TokenVerifier != jwt || dashboard.Auth.TokenRevocations != blacklist || dashboard.Auth.BanChecker != banChecker {
		t.Fatal("dashboard auth dependencies were not preserved")
	}
	if dashboard.DashboardQueries != dashboardQueries || dashboard.IdentityProvider != identity || dashboard.IdentityEnrichmentFailures != identityEnrichmentFailures {
		t.Fatal("dashboard dependencies were not preserved")
	}

	usage := buildUsageHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIUsageHTTPDeps{
			UsageQueries:               usageQueries,
			BanChecker:                 banChecker,
			IdentityEnrichmentFailures: identityEnrichmentFailures,
		},
		identity,
	)
	if usage.Auth.TokenVerifier != jwt || usage.Auth.TokenRevocations != blacklist || usage.Auth.BanChecker != banChecker {
		t.Fatal("usage auth dependencies were not preserved")
	}
	if usage.UsageQueries != usageQueries || usage.IdentityProvider != identity || usage.IdentityEnrichmentFailures != identityEnrichmentFailures {
		t.Fatal("usage dependencies were not preserved")
	}

	oauthManagement := buildOAuthManagementHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIOAuthManagementHTTPDeps{
			CredentialCreator: oauth,
			CredentialReader:  oauth,
			CredentialWriter:  oauth,
			PoolReader:        oauth,
			PoolWriter:        oauth,
			PoolHealthReader:  oauth,
			TokenRefresher:    refresher,
			ClientCatalog:     catalog,
			ModelBindings:     modelBindings,
			BanChecker:        banChecker,
		},
	)
	if oauthManagement.Auth.TokenVerifier != jwt || oauthManagement.Auth.TokenRevocations != blacklist || oauthManagement.Auth.BanChecker != banChecker {
		t.Fatal("OAuth management auth dependencies were not preserved")
	}
	if oauthManagement.CredentialCreator != oauth || oauthManagement.CredentialReader != oauth || oauthManagement.CredentialWriter != oauth || oauthManagement.PoolReader != oauth || oauthManagement.PoolWriter != oauth || oauthManagement.PoolHealthReader != oauth || oauthManagement.TokenRefresher != refresher || oauthManagement.ClientCatalog != catalog || oauthManagement.ModelBindings != modelBindings {
		t.Fatal("OAuth management dependencies were not preserved")
	}

	modelBindingsHTTP := buildModelBindingHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIModelBindingHTTPDeps{
			AccountReader: accountReader,
			PoolReader:    oauth,
			ModelBindings: modelBindings,
			BanChecker:    banChecker,
		},
	)
	if modelBindingsHTTP.Auth.TokenVerifier != jwt || modelBindingsHTTP.Auth.TokenRevocations != blacklist || modelBindingsHTTP.Auth.BanChecker != banChecker {
		t.Fatal("model-binding auth dependencies were not preserved")
	}
	if modelBindingsHTTP.AccountReader != accountReader || modelBindingsHTTP.PoolReader != oauth || modelBindingsHTTP.ModelBindings != modelBindings {
		t.Fatal("model-binding dependencies were not preserved")
	}

	diagnostics := buildUpstreamDiagnosticsHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIUpstreamDiagnosticsHTTPDeps{
			AccountReader:     accountReader,
			ModelBindings:     modelBindings,
			ProviderSecrets:   providerSecrets,
			HTTPClient:        httpClient,
			AccountHealth:     accountPorts,
			ModelCapabilities: modelCapabilities,
			BanChecker:        banChecker,
		},
	)
	if diagnostics.Auth.TokenVerifier != jwt || diagnostics.Auth.TokenRevocations != blacklist || diagnostics.Auth.BanChecker != banChecker {
		t.Fatal("upstream diagnostics auth dependencies were not preserved")
	}
	if diagnostics.AccountReader != accountReader || diagnostics.ModelBindings != modelBindings || diagnostics.ProviderSecrets != providerSecrets || diagnostics.HTTPClient != httpClient || diagnostics.AccountHealth != accountPorts || diagnostics.ModelCapabilities != modelCapabilities {
		t.Fatal("upstream diagnostics dependencies were not preserved")
	}

	accountManagement := buildUpstreamAccountManagementHTTPDeps(
		Deps{IdentityDeps: IdentityDeps{JWT: jwt, Blacklist: blacklist}},
		AIUpstreamAccountManagementHTTPDeps{
			Accounts:        accountPorts,
			AccountManager:  accountPorts,
			AccountReader:   accountReader,
			ProviderSecrets: providerSecrets,
			ModelBindings:   modelBindings,
			PriceBooks:      priceBooks,
			AdminAudit:      adminAudit,
			BanChecker:      banChecker,
		},
	)
	if accountManagement.Auth.TokenVerifier != jwt || accountManagement.Auth.TokenRevocations != blacklist || accountManagement.Auth.BanChecker != banChecker {
		t.Fatal("upstream account management auth dependencies were not preserved")
	}
	if accountManagement.Accounts != accountPorts || accountManagement.AccountManager != accountPorts || accountManagement.AccountReader != accountReader || accountManagement.ProviderSecrets != providerSecrets || accountManagement.ModelBindings != modelBindings || accountManagement.PriceBooks != priceBooks || accountManagement.AdminAudit != adminAudit {
		t.Fatal("upstream account management dependencies were not preserved")
	}
}

type providerSecretCodecStub struct{}

func (*providerSecretCodecStub) Encrypt(string) (string, error) { return "", nil }
func (*providerSecretCodecStub) Decrypt(string) (string, error) { return "", nil }

type httpDoerStub struct{}

func (*httpDoerStub) Do(*http.Request) (*http.Response, error) { return nil, nil }

type componentHealthProbeStub struct{}

func (*componentHealthProbeStub) Check(context.Context) error { return nil }

type identityEnrichmentFailureObserverStub struct{}

func (*identityEnrichmentFailureObserverStub) ObserveFailure(string, error) {}

type humaBanCheckerStub struct{}

func (*humaBanCheckerStub) IsBanned(context.Context, string) (bool, error)       { return false, nil }
func (*humaBanCheckerStub) IsTenantBanned(context.Context, string) (bool, error) { return false, nil }

type aiIdentityProviderStub struct{}

func (*aiIdentityProviderStub) BatchGetUsers(context.Context, []string) (map[string]*aitransport.IdentityUser, error) {
	return nil, nil
}

func (*aiIdentityProviderStub) BatchGetTenants(context.Context, []string) (map[string]*aitransport.IdentityTenant, error) {
	return nil, nil
}

func (*aiIdentityProviderStub) CheckTenantEndUser(context.Context, string, string) error { return nil }

type modelCapabilityResolverStub struct{}

func (*modelCapabilityResolverStub) Lookup(context.Context, string) (domain.CapabilityType, bool) {
	return "", false
}

type upstreamAccountReaderStub struct{}

func (*upstreamAccountReaderStub) GetAccountSecret(context.Context, string) (upstreamcontrol.AccountSecret, error) {
	return upstreamcontrol.AccountSecret{}, nil
}

type userUsageLogReaderStub struct{}

func (*userUsageLogReaderStub) ListUserLogs(context.Context, string, string, string, int32) ([]domain.UsageLog, error) {
	return nil, nil
}

type dashboardQueryReaderStub struct{}

func (*dashboardQueryReaderStub) Summary(context.Context, domain.DashboardFilter) (domain.DashboardSummary, error) {
	return domain.DashboardSummary{}, nil
}

func (*dashboardQueryReaderStub) TopModels(context.Context, domain.DashboardFilter, int32) ([]domain.DashboardTopModel, error) {
	return nil, nil
}

func (*dashboardQueryReaderStub) TopTenants(context.Context, domain.DashboardFilter, int32) ([]domain.DashboardTopTenant, error) {
	return nil, nil
}

func (*dashboardQueryReaderStub) RecentErrors(context.Context, domain.DashboardFilter, int32) ([]domain.DashboardRecentError, error) {
	return nil, nil
}

type adminAuditLogReaderStub struct{}

func (*adminAuditLogReaderStub) List(context.Context, int32) ([]domain.AuditLog, error) {
	return nil, nil
}

type adminAuditRecorderStub struct{}

func (*adminAuditRecorderStub) Record(context.Context, domain.AdminAuditEvent) error { return nil }

type upstreamModelBindingStoreStub struct{}

type modelCatalogReaderStub struct{}

type priceBookReaderStub struct{}

func (*priceBookReaderStub) GetPriceBook(context.Context, string) (domain.PriceBook, error) {
	return domain.PriceBook{}, nil
}

func (*modelCatalogReaderStub) ListAvailableModelPrices(context.Context, domain.ModelCatalogScope) ([]domain.RoutedModelPrice, error) {
	return nil, nil
}
func (*modelCatalogReaderStub) ListRoutedGroupPrices(context.Context, string) ([]domain.RoutedModelPrice, error) {
	return nil, nil
}
func (*modelCatalogReaderStub) ListTenantUpstreamResources(context.Context, string) ([]domain.TenantUpstreamResource, error) {
	return nil, nil
}

func (*upstreamModelBindingStoreStub) List(context.Context, domain.UpstreamModelBindingScope) ([]domain.UpstreamModelBinding, error) {
	return nil, nil
}
func (*upstreamModelBindingStoreStub) ListModelCodes(context.Context, domain.UpstreamModelBindingScope) ([]string, error) {
	return nil, nil
}
func (*upstreamModelBindingStoreStub) FindByModel(context.Context, domain.UpstreamModelBindingScope, string) (domain.UpstreamModelBinding, error) {
	return domain.UpstreamModelBinding{}, nil
}
func (*upstreamModelBindingStoreStub) Get(context.Context, domain.UpstreamModelBindingScope, string) (domain.UpstreamModelBinding, error) {
	return domain.UpstreamModelBinding{}, nil
}
func (*upstreamModelBindingStoreStub) Create(context.Context, domain.UpstreamModelBindingScope, domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error) {
	return domain.UpstreamModelBinding{}, nil
}
func (*upstreamModelBindingStoreStub) Update(context.Context, domain.UpstreamModelBindingScope, string, domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error) {
	return domain.UpstreamModelBinding{}, nil
}
func (*upstreamModelBindingStoreStub) Delete(context.Context, domain.UpstreamModelBindingScope, string) error {
	return nil
}
func (*upstreamModelBindingStoreStub) BatchDelete(context.Context, domain.UpstreamModelBindingScope, []string) (int64, error) {
	return 0, nil
}
func (*upstreamModelBindingStoreStub) Import(context.Context, domain.UpstreamModelBindingScope, []domain.UpstreamModelBindingWrite) (domain.UpstreamModelBindingImportResult, error) {
	return domain.UpstreamModelBindingImportResult{}, nil
}
