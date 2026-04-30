package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"uni-ai-api/backend/internal/urm"
)

// fakeURMClient satisfies urmClient interface for tests that don't need URM calls
type fakeURMClient struct{}

func (c *fakeURMClient) Freeze(context.Context, urm.FreezeRequest) (*urm.FreezeResponse, error) {
	return &urm.FreezeResponse{}, nil
}
func (c *fakeURMClient) Confirm(context.Context, urm.ConfirmRequest) (*urm.ConfirmResponse, error) {
	return &urm.ConfirmResponse{}, nil
}
func (c *fakeURMClient) Cancel(context.Context, string) error               { return nil }
func (c *fakeURMClient) ExchangeCode(context.Context, string, string) (*urm.TokenPairResponse, error) {
	return nil, nil
}

// fakeJWKSValidator satisfies jwksValidator for tests
type fakeJWKSValidator struct {
	claims *urm.Claims
	err    error
}

func (v *fakeJWKSValidator) ValidateToken(_ context.Context, _ string) (*urm.Claims, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v.claims, nil
}

func TestBearerToken(t *testing.T) {
	if got := bearerToken("Bearer abc.def"); got != "abc.def" {
		t.Fatalf("bearerToken = %q, want abc.def", got)
	}
	if got := bearerToken("bearer token"); got != "token" {
		t.Fatalf("bearerToken lowercase = %q, want token", got)
	}
	if got := bearerToken("Token abc"); got != "" {
		t.Fatalf("bearerToken invalid scheme = %q, want empty", got)
	}
}

func TestAdminAuthAcceptsURMAdminBearer(t *testing.T) {
	server := &Server{
		logger:    slog.Default(),
		urmClient: &fakeURMClient{},
		jwksValidator: &fakeJWKSValidator{
			claims: &urm.Claims{UserID: "admin-1", Username: "admin", UserType: 1},
		},
	}

	nextCalled := false
	handler := server.adminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if !nextCalled {
		t.Fatal("next handler was not called")
	}
}

func TestAdminAuthRejectsURMNonAdminBearer(t *testing.T) {
	server := &Server{
		logger:    slog.Default(),
		urmClient: &fakeURMClient{},
		jwksValidator: &fakeJWKSValidator{
			claims: &urm.Claims{UserID: "user-1", Username: "user", UserType: 4},
		},
	}

	handler := server.adminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuthRejectsURMError(t *testing.T) {
	server := &Server{
		logger:    slog.Default(),
		urmClient: &fakeURMClient{},
		jwksValidator: &fakeJWKSValidator{
			err: errors.New("jwks unavailable"),
		},
	}

	handler := server.adminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	req.Header.Set("Authorization", "Bearer broken-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
