package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/server"
)

type accountCatalogStub struct {
	fresh, cached, reads int
	result               clientcatalog.Result
}

func (s *accountCatalogStub) Resolve(context.Context, domain.CredentialPool) clientcatalog.Result {
	s.cached++
	return s.result
}
func (s *accountCatalogStub) ResolveFresh(context.Context, domain.CredentialPool) clientcatalog.Result {
	s.fresh++
	return s.result
}
func (s *accountCatalogStub) ReadCredentialModels(context.Context, string) (map[string]domain.CredentialModelCatalog, error) {
	s.reads++
	out := map[string]domain.CredentialModelCatalog{}
	for _, account := range s.result.Accounts {
		out[account.CredentialID] = account
	}
	return out, nil
}

func TestPoolModelsExposeAccountCatalogsAndExplicitRefresh(t *testing.T) {
	catalog := &accountCatalogStub{result: clientcatalog.Result{
		Source: "live", ObservedAt: time.Now(), ProfileRevision: "profile",
		Models:   []clientruntime.ModelCard{{ID: "account-model"}},
		Accounts: []domain.CredentialModelCatalog{{CredentialID: "cred-1", Models: []string{"account-model"}, Source: "live", ValidUntil: time.Now().Add(time.Minute).UnixMilli()}},
	}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerOAuthPools(api, OAuthManagementHTTPDeps{
		ClientCatalog:    catalog,
		PoolReader:       &poolReaderStub{pool: &domain.CredentialPool{ID: "pool-1", FixedProviderType: domain.FixedProviderCodex}},
		CredentialReader: credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-1", ProviderType: "codex", Status: "active"}},
	})
	response := performOAuthManagementRequest(router, http.MethodGet, "/api/v1/credential-pools/pool-1/available-models?refresh=true")
	if response.Code != http.StatusOK || catalog.fresh != 1 || catalog.cached != 0 {
		t.Fatalf("explicit discovery: status=%d fresh=%d cached=%d body=%s", response.Code, catalog.fresh, catalog.cached, response.Body.String())
	}
	var available poolAvailableModelsDTO
	if err := json.Unmarshal(response.Body.Bytes(), &available); err != nil || len(available.Accounts) != 1 || available.Accounts[0].Models[0] != "account-model" {
		t.Fatalf("account observations missing: %+v error=%v", available, err)
	}
	response = performOAuthManagementRequest(router, http.MethodGet, "/api/v1/credential-pools/pool-1/credentials")
	var list struct {
		Items []poolCredentialDTO `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil || response.Code != http.StatusOK || len(list.Items) != 1 || list.Items[0].ModelCatalog == nil || catalog.reads != 1 || catalog.fresh != 1 {
		t.Fatalf("ordinary list must use snapshots only: %s error=%v", response.Body.String(), err)
	}
}
