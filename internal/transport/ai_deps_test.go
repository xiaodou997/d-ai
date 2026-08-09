package transport

import (
	"net/http"
	"testing"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/tokenrefresh"
	"xiaodou/dai/internal/auth"
)

func TestBuildAIDepsWiresRuntimeManagementDependencies(t *testing.T) {
	oauth := &pgadapter.OAuthCredentialStore{}
	refresher := &tokenrefresh.Refresher{}
	catalog := &clientcatalog.Service{}
	httpClient := &http.Client{}
	health := routing.DefaultInMemoryTracker()
	weights := &pgadapter.RouteWeightsStore{}
	blacklist := &auth.BlacklistService{}

	got := buildAIDeps(Deps{
		OAuth:           oauth,
		TokenRefresher:  refresher,
		ClientCatalog:   catalog,
		SecretMasterKey: "secret-master-key",
		AIHTTPClient:    httpClient,
		Health:          health,
		Weights:         weights,
		Blacklist:       blacklist,
	})

	if got.OAuth != oauth || got.TokenRefresher != refresher || got.ClientCatalog != catalog {
		t.Fatal("OAuth management dependencies were not preserved")
	}
	if got.SecretMasterKey != "secret-master-key" || got.HTTPClient != httpClient {
		t.Fatal("upstream management dependencies were not preserved")
	}
	if got.Health != health || got.Weights != weights {
		t.Fatal("routing management dependencies were not preserved")
	}
	if got.TokenRevocations != blacklist {
		t.Fatal("portal token revocation dependency was not preserved")
	}
}
