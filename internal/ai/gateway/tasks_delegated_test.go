package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/urm"
)

type stubDelegatedValidator struct {
	claims *urm.Claims
	err    error
}

func (s stubDelegatedValidator) ValidateToken(context.Context, string) (*urm.Claims, error) {
	return s.claims, s.err
}

func delegatedClaims() *urm.Claims {
	return &urm.Claims{
		PrincipalType: "delegated",
		TokenUse:      "access",
		ClientID:      "creative-service",
		TenantID:      "tenant-1",
		UserID:        "user-1",
		BillingScope:  "user",
		Scope:         "ai.invoke",
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"ai-service"},
		},
	}
}

func newDelegatedGateway(v delegatedTokenValidator) *Gateway {
	return &Gateway{
		logger:                   zap.NewNop(),
		jwksValidator:            v,
		delegationAllowedClients: []string{"creative-service"},
	}
}

func requestWithBearer() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/internal/v1/tasks", nil)
	r.Header.Set("Authorization", "Bearer some.jwt.token")
	return r
}

func TestDelegatedSubjectUserScope(t *testing.T) {
	g := newDelegatedGateway(stubDelegatedValidator{claims: delegatedClaims()})
	rec := httptest.NewRecorder()
	subject, ok := g.delegatedSubject(rec, requestWithBearer())
	if !ok {
		t.Fatalf("expected success, got %d %s", rec.Code, rec.Body.String())
	}
	if subject.AuthMethod != coreidentity.AuthMethodDelegated {
		t.Fatalf("auth method = %s", subject.AuthMethod)
	}
	if subject.Scope != coreidentity.ScopeUser || subject.TenantID != "tenant-1" || subject.UserID != "user-1" {
		t.Fatalf("unexpected subject: %+v", subject)
	}
	if subject.ActorClientID != "creative-service" {
		t.Fatalf("actor = %q", subject.ActorClientID)
	}
	if subject.RequestSource != coreidentity.RequestSourceDelegated {
		t.Fatalf("request source = %s", subject.RequestSource)
	}
}

func TestDelegatedSubjectTenantScope(t *testing.T) {
	claims := delegatedClaims()
	claims.BillingScope = "tenant"
	claims.UserID = ""
	g := newDelegatedGateway(stubDelegatedValidator{claims: claims})
	rec := httptest.NewRecorder()
	subject, ok := g.delegatedSubject(rec, requestWithBearer())
	if !ok {
		t.Fatalf("expected success, got %d", rec.Code)
	}
	if subject.Scope != coreidentity.ScopeTenant || subject.UserID != "" {
		t.Fatalf("tenant scope must not carry user: %+v", subject)
	}
}

func TestDelegatedSubjectRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*urm.Claims)
		valErr error
		status int
	}{
		{"not delegated", func(c *urm.Claims) { c.PrincipalType = "user" }, nil, http.StatusForbidden},
		{"wrong scope", func(c *urm.Claims) { c.Scope = "other" }, nil, http.StatusForbidden},
		{"wrong audience", func(c *urm.Claims) { c.Audience = jwt.ClaimStrings{"someone-else"} }, nil, http.StatusForbidden},
		{"client not allowed", func(c *urm.Claims) { c.ClientID = "rogue-service" }, nil, http.StatusForbidden},
		{"no tenant", func(c *urm.Claims) { c.TenantID = "" }, nil, http.StatusForbidden},
		{"user scope without user", func(c *urm.Claims) { c.BillingScope = "user"; c.UserID = "" }, nil, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := delegatedClaims()
			tc.mutate(claims)
			g := newDelegatedGateway(stubDelegatedValidator{claims: claims, err: tc.valErr})
			rec := httptest.NewRecorder()
			if _, ok := g.delegatedSubject(rec, requestWithBearer()); ok {
				t.Fatalf("expected rejection")
			}
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}

func TestDelegatedSubjectMissingToken(t *testing.T) {
	g := newDelegatedGateway(stubDelegatedValidator{claims: delegatedClaims()})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/internal/v1/tasks", nil) // no Authorization
	if _, ok := g.delegatedSubject(rec, r); ok {
		t.Fatal("expected rejection for missing token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
