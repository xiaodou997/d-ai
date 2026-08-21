package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/libs/go/server"
)

var (
	_ UpstreamAccountCatalog      = (*upstreamcontrol.Service)(nil)
	_ UpstreamAccountManager      = (*upstreamcontrol.Service)(nil)
	_ UpstreamAccountHealthWriter = (*upstreamcontrol.Service)(nil)
)

type upstreamAccountPortsStub struct {
	accounts []domain.UpstreamAccount
	secret   upstreamcontrol.AccountSecret

	listCalls int
	secretID  string

	createInput   upstreamcontrol.CreateAccountInput
	createResult  domain.UpstreamAccount
	updateInput   upstreamcontrol.UpdateAccountInput
	updateResult  domain.UpstreamAccount
	statusID      string
	status        string
	statusResult  domain.UpstreamAccount
	deleteID      string
	invalidID     string
	invalidReason string
	invalidResult domain.UpstreamAccount
}

func (s *upstreamAccountPortsStub) ListAccounts(context.Context) ([]domain.UpstreamAccount, error) {
	s.listCalls++
	return s.accounts, nil
}

func (s *upstreamAccountPortsStub) GetAccountSecret(_ context.Context, id string) (upstreamcontrol.AccountSecret, error) {
	s.secretID = id
	return s.secret, nil
}

func (s *upstreamAccountPortsStub) CreateAccount(_ context.Context, input upstreamcontrol.CreateAccountInput) (domain.UpstreamAccount, error) {
	s.createInput = input
	return s.createResult, nil
}

func (s *upstreamAccountPortsStub) UpdateAccount(_ context.Context, input upstreamcontrol.UpdateAccountInput) (domain.UpstreamAccount, error) {
	s.updateInput = input
	return s.updateResult, nil
}

func (s *upstreamAccountPortsStub) UpdateAccountStatus(_ context.Context, id, status string) (domain.UpstreamAccount, error) {
	s.statusID = id
	s.status = status
	return s.statusResult, nil
}

func (s *upstreamAccountPortsStub) DeleteAccount(_ context.Context, id string) error {
	s.deleteID = id
	return nil
}

func (s *upstreamAccountPortsStub) MarkAccountInvalid(_ context.Context, id, reason string) (domain.UpstreamAccount, error) {
	s.invalidID = id
	s.invalidReason = reason
	return s.invalidResult, nil
}

func TestUpstreamAccountRoutesUseSeparatedPorts(t *testing.T) {
	stub := &upstreamAccountPortsStub{
		accounts: []domain.UpstreamAccount{{
			ID:           "account-list",
			Name:         "Listed",
			ExtraHeaders: []byte(`{"Authorization":"secret","X-Trace":"visible"}`),
			Status:       domain.UpstreamAccountStatusActive,
		}},
		createResult: domain.UpstreamAccount{ID: "account-created", Name: "Created", Status: domain.UpstreamAccountStatusDisabled},
		updateResult: domain.UpstreamAccount{ID: "account-update", Name: "Updated", Status: domain.UpstreamAccountStatusActive},
		statusResult: domain.UpstreamAccount{ID: "account-status", Name: "Updated", Status: domain.UpstreamAccountStatusActive},
	}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerUpstreamAccounts(api, AIDeps{CatalogDeps: CatalogDeps{
		Accounts:       stub,
		AccountManager: stub,
	}})

	listRecorder := performUpstreamAccountRequest(router, http.MethodGet, "/api/v1/upstream-accounts", "")
	requireUpstreamAccountStatus(t, listRecorder, http.StatusOK)
	var listBody struct {
		Items []accountDTO `json:"items"`
		Total int          `json:"total"`
	}
	decodeUpstreamAccountResponse(t, listRecorder, &listBody)
	if stub.listCalls != 1 || listBody.Total != 1 || len(listBody.Items) != 1 {
		t.Fatalf("list calls = %d, response = %#v", stub.listCalls, listBody)
	}
	var headers map[string]string
	if err := json.Unmarshal(listBody.Items[0].ExtraHeaders, &headers); err != nil {
		t.Fatalf("decode redacted headers: %v", err)
	}
	if headers["Authorization"] != "***REDACTED***" || headers["X-Trace"] != "visible" {
		t.Fatalf("redacted headers = %#v", headers)
	}

	createRecorder := performUpstreamAccountRequest(router, http.MethodPost, "/api/v1/upstream-accounts", `{
		"name":"Created","tenant_display_name":"Tenant created","tenant_access_mode":"public",
		"base_url":"https://create.example","api_key":"create-secret","extra_headers":{"X-Trace":"create"},
		"default_provider_family":"anthropic","concurrency_limit":4,"price_book_id":"price-1","tenant_multiplier":1.25
	}`)
	requireUpstreamAccountStatus(t, createRecorder, http.StatusOK)
	if stub.createInput.Name != "Created" || stub.createInput.APIKey != "create-secret" || stub.createInput.ConcurrencyLimit == nil || *stub.createInput.ConcurrencyLimit != 4 {
		t.Fatalf("create input = %#v", stub.createInput)
	}

	updateRecorder := performUpstreamAccountRequest(router, http.MethodPatch, "/api/v1/upstream-accounts/account-update", `{
		"name":"Updated","tenant_display_name":"Tenant updated","tenant_access_mode":"restricted",
		"base_url":"https://update.example","api_key":"update-secret","extra_headers":{"X-Trace":"update"},
		"default_provider_family":"gemini","concurrency_limit":6,"price_book_id":"price-2","tenant_multiplier":1.5
	}`)
	requireUpstreamAccountStatus(t, updateRecorder, http.StatusOK)
	if stub.updateInput.ID != "account-update" || stub.updateInput.Name != "Updated" || stub.updateInput.APIKey != "update-secret" || stub.updateInput.ConcurrencyLimit == nil || *stub.updateInput.ConcurrencyLimit != 6 {
		t.Fatalf("update input = %#v", stub.updateInput)
	}

	statusRecorder := performUpstreamAccountRequest(router, http.MethodPatch, "/api/v1/upstream-accounts/account-status/status", `{"status":"active"}`)
	requireUpstreamAccountStatus(t, statusRecorder, http.StatusOK)
	if stub.statusID != "account-status" || stub.status != domain.UpstreamAccountStatusActive {
		t.Fatalf("status command = id %q status %q", stub.statusID, stub.status)
	}

	deleteRecorder := performUpstreamAccountRequest(router, http.MethodDelete, "/api/v1/upstream-accounts/account-delete", "")
	requireUpstreamAccountStatus(t, deleteRecorder, http.StatusOK)
	if stub.deleteID != "account-delete" {
		t.Fatalf("delete ID = %q", stub.deleteID)
	}
}

func TestUpstreamAccountRoutesRequireSeparatedPorts(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerUpstreamAccounts(api, AIDeps{})

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/upstream-accounts", ""},
		{http.MethodPost, "/api/v1/upstream-accounts", `{"name":"a","base_url":"https://example","api_key":"secret"}`},
		{http.MethodPatch, "/api/v1/upstream-accounts/a", `{"name":"a","base_url":"https://example"}`},
		{http.MethodPatch, "/api/v1/upstream-accounts/a/status", `{"status":"active"}`},
		{http.MethodDelete, "/api/v1/upstream-accounts/a", ""},
	}
	for _, request := range requests {
		recorder := performUpstreamAccountRequest(router, request.method, request.path, request.body)
		requireUpstreamAccountStatus(t, recorder, http.StatusServiceUnavailable)
	}
}

func TestUpstreamAccountTransferComposesCatalogReaderAndManager(t *testing.T) {
	concurrency := 3
	stub := &upstreamAccountPortsStub{
		accounts: []domain.UpstreamAccount{{
			ID:                "account-export",
			Name:              "Exported",
			TenantDisplayName: "Tenant exported",
			TenantAccessMode:  "public",
			BaseURL:           "https://export.example",
			ExtraHeaders:      []byte(`{"X-Trace":"export"}`),
			DefaultProtocol:   "anthropic",
			ConcurrencyLimit:  &concurrency,
			Status:            domain.UpstreamAccountStatusActive,
		}},
		secret:       upstreamcontrol.AccountSecret{Ciphertext: "ciphertext"},
		createResult: domain.UpstreamAccount{ID: "account-import", DefaultProtocol: "gemini"},
	}
	codec := &providerSecretCodecStub{plaintext: "plaintext-key"}
	d := AIDeps{
		CatalogDeps: CatalogDeps{
			Accounts:       stub,
			AccountReader:  stub,
			AccountManager: stub,
		},
		RuntimeDeps: RuntimeDeps{ProviderSecrets: codec},
	}

	exported, err := exportUpstreamAccounts(t.Context(), d, []string{"account-export"}, false)
	if err != nil {
		t.Fatalf("export accounts: %v", err)
	}
	if stub.secretID != "account-export" || codec.decryptInput != "ciphertext" {
		t.Fatalf("export secret lookup = id %q ciphertext %q", stub.secretID, codec.decryptInput)
	}
	if len(exported.Body.Accounts) != 1 || exported.Body.Accounts[0].APIKey != "plaintext-key" || exported.Body.Accounts[0].Name != "Exported" {
		t.Fatalf("export response = %#v", exported.Body.Accounts)
	}

	stub.accounts = nil
	multiplier := 1.4
	limit := int32(5)
	imported, err := importUpstreamAccounts(t.Context(), d, upstreamAccountImportRequest{
		Accounts: []upstreamAccountTransferAccountDTO{{
			Name:                  " Imported ",
			TenantDisplayName:     " Tenant imported ",
			TenantAccessMode:      "public",
			BaseURL:               " https://import.example ",
			APIKey:                " import-secret ",
			DefaultProviderFamily: "gemini",
			ConcurrencyLimit:      &limit,
			ExtraHeaders:          json.RawMessage(`{"X-Trace":"import"}`),
		}},
		DefaultTenantMultiplier:  &multiplier,
		DuplicateAccountStrategy: "skip",
		DuplicateBindingStrategy: "skip",
	})
	if err != nil {
		t.Fatalf("import accounts: %v", err)
	}
	if len(imported.Body.CreatedAccountIDs) != 1 || imported.Body.CreatedAccountIDs[0] != "account-import" {
		t.Fatalf("import response = %#v", imported.Body)
	}
	if stub.createInput.Name != "Imported" || stub.createInput.APIKey != "import-secret" || stub.createInput.Status != domain.UpstreamAccountStatusDisabled {
		t.Fatalf("import create input = %#v", stub.createInput)
	}
	if stub.createInput.PriceBookID != "" || stub.createInput.TenantMultiplier == nil || *stub.createInput.TenantMultiplier != multiplier {
		t.Fatalf("import billing input = %#v", stub.createInput)
	}
}

func TestReconcileUpstreamAccountStatusUsesHealthWriter(t *testing.T) {
	stub := &upstreamAccountPortsStub{}

	if err := reconcileUpstreamAccountTestStatus(t.Context(), stub, "recovered", domain.UpstreamAccountStatusInvalid, upstreamTestResult{OK: true}); err != nil {
		t.Fatalf("reconcile recovered account: %v", err)
	}
	if stub.statusID != "recovered" || stub.status != domain.UpstreamAccountStatusActive {
		t.Fatalf("recovered command = id %q status %q", stub.statusID, stub.status)
	}

	if err := reconcileUpstreamAccountTestStatus(t.Context(), stub, "rejected", domain.UpstreamAccountStatusActive, upstreamTestResult{HTTPStatus: http.StatusUnauthorized}); err != nil {
		t.Fatalf("reconcile rejected account: %v", err)
	}
	if stub.invalidID != "rejected" || !strings.Contains(stub.invalidReason, "401") {
		t.Fatalf("invalid command = id %q reason %q", stub.invalidID, stub.invalidReason)
	}

	stub.statusID = ""
	stub.invalidID = ""
	if err := reconcileUpstreamAccountTestStatus(t.Context(), stub, "disabled", domain.UpstreamAccountStatusDisabled, upstreamTestResult{HTTPStatus: http.StatusUnauthorized}); err != nil {
		t.Fatalf("reconcile disabled account: %v", err)
	}
	if stub.statusID != "" || stub.invalidID != "" {
		t.Fatalf("disabled account changed: status ID %q invalid ID %q", stub.statusID, stub.invalidID)
	}
}

func performUpstreamAccountRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func requireUpstreamAccountStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, want, recorder.Body.String())
	}
}

func decodeUpstreamAccountResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
