package transport

import (
	"context"
	"net/http"
	"testing"
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
	q, err := recordQuery(ctx, &recordListInput{TenantID: "foreign", UserID: "foreign", TenantName: " Acme ", UserName: " Alice ", Group: " premium ", APIKeyName: " production ", Model: " model-id ", From: "2026-09-01T00:00:00+08:00", To: "2026-10-01T00:00:00+08:00"}, "requests")
	if err != nil {
		t.Fatal(err)
	}
	if q.TenantID != "own-tenant" || q.UserID != "own-user" || !q.EndUser || q.TenantName != "Acme" || q.UserName != "Alice" || q.Group != "premium" || q.APIKeyName != "production" || q.Model != "model-id" {
		t.Fatalf("query=%+v", q)
	}
	if q.From == nil || q.To == nil || !q.From.Before(*q.To) {
		t.Fatalf("invalid time window: %+v", q)
	}
}
