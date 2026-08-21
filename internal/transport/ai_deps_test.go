package transport

import (
	"context"
	"net/http"
	"testing"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/tokenrefresh"
	"xiaodou/dai/internal/ai/upstreamcontrol"
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
	providerSecrets := &providerSecretCodecStub{}
	accountReader := &upstreamAccountReaderStub{}
	modelBindings := &upstreamModelBindingStoreStub{}
	modelCatalog := &modelCatalogReaderStub{}
	priceBooks := &priceBookReaderStub{}
	userUsageLogs := &userUsageLogReaderStub{}
	adminAudit := &adminAuditRecorderStub{}
	identityEnrichmentFailures := &identityEnrichmentFailureObserverStub{}

	got := buildAIDeps(
		Deps{IdentityDeps: IdentityDeps{Blacklist: blacklist}},
		AIDeps{
			AIInfrastructureDeps: AIInfrastructureDeps{
				ProviderSecrets: providerSecrets,
				AIHTTPClient:    httpClient,
				DatabaseHealth:  databaseHealth,
				RedisHealth:     redisHealth,
				Health:          health,
				Weights:         weights,
			},
			AIIdentityDeps: AIIdentityDeps{
				CredentialCreator: oauth,
				CredentialReader:  oauth,
				CredentialWriter:  oauth,
				PoolReader:        oauth,
				PoolWriter:        oauth,
				PoolHealthReader:  oauth,
				TokenRefresher:    refresher,
			},
			AICatalogDeps: AICatalogDeps{
				ClientCatalog:     catalog,
				ModelCapabilities: modelCapabilities,
				AccountReader:     accountReader,
				ModelBindings:     modelBindings,
				ModelCatalog:      modelCatalog,
				PriceBooks:        priceBooks,
			},
			AIOperationsDeps: AIOperationsDeps{
				IdentityEnrichmentFailures: identityEnrichmentFailures,
				UserUsageLogs:              userUsageLogs,
				AdminAudit:                 adminAudit,
			},
		},
		nil,
	)

	if got.CredentialCreator != oauth || got.CredentialReader != oauth || got.CredentialWriter != oauth || got.PoolReader != oauth || got.PoolWriter != oauth || got.PoolHealthReader != oauth || got.TokenRefresher != refresher {
		t.Fatal("OAuth management dependencies were not preserved")
	}
	if got.ClientCatalog != catalog || got.ModelCapabilities != modelCapabilities || got.AccountReader != accountReader || got.ModelBindings != modelBindings || got.ModelCatalog != modelCatalog || got.PriceBooks != priceBooks {
		t.Fatal("catalog dependencies were not preserved")
	}
	if got.ProviderSecrets != providerSecrets || got.HTTPClient != httpClient {
		t.Fatal("upstream management dependencies were not preserved")
	}
	if got.DatabaseHealth != databaseHealth || got.RedisHealth != redisHealth || got.Health != health || got.Weights != weights {
		t.Fatal("routing management dependencies were not preserved")
	}
	if got.IdentityEnrichmentFailures != identityEnrichmentFailures {
		t.Fatal("observability dependencies were not preserved")
	}
	if got.UserUsageLogs != userUsageLogs {
		t.Fatal("user usage log reader was not preserved")
	}
	if got.AdminAudit != adminAudit {
		t.Fatal("admin audit recorder was not preserved")
	}
	if got.TokenRevocations != blacklist {
		t.Fatal("portal token revocation dependency was not preserved")
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
