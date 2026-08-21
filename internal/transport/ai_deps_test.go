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
	redisHealth := &componentHealthProbeStub{}
	health := routing.DefaultInMemoryTracker()
	weights := &pgadapter.RouteWeightsStore{}
	blacklist := &auth.BlacklistService{}
	providerSecrets := &providerSecretCodecStub{}
	accountReader := &upstreamAccountReaderStub{}
	identityEnrichmentFailures := &identityEnrichmentFailureObserverStub{}

	got := buildAIDeps(
		Deps{IdentityDeps: IdentityDeps{Blacklist: blacklist}},
		AIDeps{
			AIInfrastructureDeps: AIInfrastructureDeps{
				ProviderSecrets: providerSecrets,
				AIHTTPClient:    httpClient,
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
			},
			AIOperationsDeps: AIOperationsDeps{
				IdentityEnrichmentFailures: identityEnrichmentFailures,
			},
		},
		nil,
	)

	if got.CredentialCreator != oauth || got.CredentialReader != oauth || got.CredentialWriter != oauth || got.PoolReader != oauth || got.PoolWriter != oauth || got.PoolHealthReader != oauth || got.TokenRefresher != refresher {
		t.Fatal("OAuth management dependencies were not preserved")
	}
	if got.ClientCatalog != catalog || got.ModelCapabilities != modelCapabilities || got.AccountReader != accountReader {
		t.Fatal("catalog dependencies were not preserved")
	}
	if got.ProviderSecrets != providerSecrets || got.HTTPClient != httpClient {
		t.Fatal("upstream management dependencies were not preserved")
	}
	if got.RedisHealth != redisHealth || got.Health != health || got.Weights != weights {
		t.Fatal("routing management dependencies were not preserved")
	}
	if got.IdentityEnrichmentFailures != identityEnrichmentFailures {
		t.Fatal("observability dependencies were not preserved")
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
