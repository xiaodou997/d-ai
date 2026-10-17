package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/server"
)

type quotaInspectorStub struct {
	calls int
	fail  bool
}

func (s *quotaInspectorStub) RefreshPool(_ context.Context, id string, force bool) (map[string]domain.CredentialQuotaInspection, error) {
	s.calls++
	if s.fail {
		return nil, errors.New("internal upstream secret must not escape")
	}
	return map[string]domain.CredentialQuotaInspection{"cred-1": {State: "failed", ObservedAt: 123}}, nil
}
func TestQuotaRefreshIsExplicitAndDoesNotExposeUpstreamErrors(t *testing.T) {
	inspector := &quotaInspectorStub{}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerOAuthPools(api, OAuthManagementHTTPDeps{QuotaInspector: inspector, CredentialReader: credentialReaderStub{summary: &domain.OAuthCredentialSummary{ID: "cred-1", PoolID: "pool-1", ProviderType: "codex", Status: "active"}}})
	response := performOAuthManagementRequest(router, http.MethodGet, "/api/v1/credential-pools/pool-1/credentials")
	if response.Code != 200 || inspector.calls != 0 {
		t.Fatalf("ordinary list queried upstream: %d", response.Code)
	}
	response = performOAuthManagementRequest(router, http.MethodGet, "/api/v1/credential-pools/pool-1/credentials?refresh_quota=true")
	if response.Code != 200 || inspector.calls != 1 || !strings.Contains(response.Body.String(), `"quota_inspection":{"state":"failed"`) {
		t.Fatalf("partial query result: %s", response.Body.String())
	}
	inspector.fail = true
	response = performOAuthManagementRequest(router, http.MethodGet, "/api/v1/credential-pools/pool-1/credentials?refresh_quota=true")
	if response.Code != 503 || strings.Contains(response.Body.String(), "internal upstream secret") {
		t.Fatalf("unsafe query failure: %s", response.Body.String())
	}
}
