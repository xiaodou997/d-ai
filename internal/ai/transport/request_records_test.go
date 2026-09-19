package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

func TestRecordScopesCannotBeOverriddenByQuery(t *testing.T) {
	for _, tc := range []struct {
		role           int
		tenant, user   string
		admin, endUser bool
	}{
		{1, "foreign-tenant", "foreign-user", true, false},
		{2, "foreign-tenant", "foreign-user", true, false},
		{3, "own-tenant", "foreign-user", false, false},
		{4, "own-tenant", "own-user", false, true},
	} {
		ctx := context.WithValue(context.Background(), authClaimsContextKey{}, &auth.Claims{UserType: tc.role, TenantID: "own-tenant", UserID: "own-user"})
		scope, err := recordScope(ctx, "foreign-tenant", "foreign-user")
		if err != nil {
			t.Fatal(err)
		}
		if scope.TenantID != tc.tenant || scope.UserID != tc.user || scope.Admin != tc.admin || scope.EndUser != tc.endUser {
			t.Fatalf("role %d scope=%+v", tc.role, scope)
		}
	}
	if _, err := recordScope(context.Background(), "", ""); err == nil {
		t.Fatal("anonymous records scope accepted")
	}
}
func TestLegacyRequestListRoutesAreRemoved(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	RegisterUsage(api, UsageHTTPDeps{})
	for _, path := range []string{"/api/v1/usage-logs", "/api/v1/usage-logs/old", "/api/v1/user-usage-logs", "/api/v1/tenants/me/usage-logs"} {
		if got := performUsageRequest(router, path).Code; got != http.StatusNotFound {
			t.Fatalf("legacy request contract survived %s %d", path, got)
		}
	}
}

func TestRecordQuerySearchKeepsScopeAndTrimsNames(t *testing.T) {
	ctx := context.WithValue(context.Background(), authClaimsContextKey{}, &auth.Claims{UserType: 4, TenantID: "own-tenant", UserID: "own-user"})
	q, err := recordQuery(ctx, &recordListInput{RequestID: " request-123 ", TenantID: "foreign", UserID: "foreign", TenantName: " Acme ", UserName: " Alice ", Group: " premium ", APIKeyName: " production ", Model: " model-id ", From: "2026-09-01T00:00:00+08:00", To: "2026-10-01T00:00:00+08:00"}, "requests")
	if err != nil {
		t.Fatal(err)
	}
	if q.RequestID != "request-123" || q.TenantID != "own-tenant" || q.UserID != "own-user" || !q.EndUser || q.TenantName != "Acme" || q.UserName != "Alice" || q.Group != "premium" || q.APIKeyName != "production" || q.Model != "model-id" {
		t.Fatalf("query=%+v", q)
	}
	if q.From == nil || q.To == nil || !q.From.Before(*q.To) {
		t.Fatalf("invalid time window: %+v", q)
	}
}
type debugRecordsStub struct {
	debugPayloadCalls int
}

func (s *debugRecordsStub) Records(context.Context, domain.RecordQuery) (domain.RecordPage, error) {
	return domain.RecordPage{}, nil
}

func (s *debugRecordsStub) Record(context.Context, domain.RecordScope, string) (domain.RequestRecord, error) {
	return domain.RequestRecord{}, nil
}

func (s *debugRecordsStub) RecordSummary(context.Context, domain.RecordQuery) (domain.RecordSummary, error) {
	return domain.RecordSummary{}, nil
}

func (s *debugRecordsStub) Refund(context.Context, string, string, string) error {
	return nil
}

func (s *debugRecordsStub) StartDebug(context.Context, domain.DebugSessionInput, string) (domain.DebugSession, error) {
	return domain.DebugSession{}, nil
}

func (s *debugRecordsStub) DebugPayload(context.Context, string) (json.RawMessage, error) {
	s.debugPayloadCalls++
	return json.RawMessage(`{"request":"safe"}`), nil
}

func TestDebugPayloadRequiresRecentAuthentication(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	recent := auth.NewRecentAuthService(client)
	records := &debugRecordsStub{}

	_, api := humatest.New(t)
	registerRecordDebug(api, UsageHTTPDeps{
		Records: records,
		Auth: HTTPAuthDeps{
			TokenVerifier: platformAuthTokenVerifierStub{},
			RecentAuth:    recent,
		},
	})
	authorization := "Authorization: Bearer token"

	response := api.Get("/api/v2/requests/request-1/debug", authorization)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("debug payload without recent auth status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if records.debugPayloadCalls != 0 {
		t.Fatal("debug payload handler ran before recent authentication")
	}

	if err := recent.Mark(context.Background(), "admin-1", "session-1", "test"); err != nil {
		t.Fatal(err)
	}
	response = api.Get("/api/v2/requests/request-1/debug", authorization)
	if response.Code != http.StatusOK {
		t.Fatalf("debug payload with recent auth status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if records.debugPayloadCalls != 1 {
		t.Fatalf("debug payload calls = %d, want 1", records.debugPayloadCalls)
	}
}
