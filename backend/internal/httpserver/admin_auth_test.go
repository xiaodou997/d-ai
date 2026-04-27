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

type fakeURMUserInfoClient struct {
	userInfo *urm.UserInfoResponse
	err      error
	token    string
}

func (c *fakeURMUserInfoClient) UserInfo(_ context.Context, token string) (*urm.UserInfoResponse, error) {
	c.token = token
	if c.err != nil {
		return nil, c.err
	}
	return c.userInfo, nil
}

func (c *fakeURMUserInfoClient) Freeze(context.Context, urm.FreezeRequest) (*urm.FreezeResponse, error) {
	return &urm.FreezeResponse{}, nil
}

func (c *fakeURMUserInfoClient) Confirm(context.Context, urm.ConfirmRequest) (*urm.ConfirmResponse, error) {
	return &urm.ConfirmResponse{}, nil
}

func (c *fakeURMUserInfoClient) Cancel(context.Context, string) error {
	return nil
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
	fakeURM := &fakeURMUserInfoClient{
		userInfo: &urm.UserInfoResponse{Subject: "admin-1", Username: "admin", UserType: 1},
	}

	server := &Server{
		logger:    slog.Default(),
		urmClient: fakeURM,
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
	if fakeURM.token != "valid-token" {
		t.Fatalf("URM token = %q, want valid-token", fakeURM.token)
	}
}

func TestAdminAuthRejectsURMNonAdminBearer(t *testing.T) {
	server := &Server{
		logger: slog.Default(),
		urmClient: &fakeURMUserInfoClient{
			userInfo: &urm.UserInfoResponse{Subject: "user-1", Username: "user", UserType: 4},
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
		urmClient: &fakeURMUserInfoClient{err: errors.New("urm unavailable")},
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
