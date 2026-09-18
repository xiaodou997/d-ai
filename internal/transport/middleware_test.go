package transport

import (
	"context"
	"net/http"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/auth"
)

func TestIsUserAccessClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims *auth.Claims
		want   bool
	}{
		{name: "access token", claims: &auth.Claims{PrincipalType: "user", TokenUse: "access", SessionID: "session-1"}, want: true},
		{name: "legacy access without session", claims: &auth.Claims{PrincipalType: "user", TokenUse: "access"}},
		{name: "refresh token", claims: &auth.Claims{PrincipalType: "user", TokenUse: "refresh"}},
		{name: "non-user principal", claims: &auth.Claims{PrincipalType: "admin", TokenUse: "access"}},
		{name: "missing claims", claims: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUserAccessClaims(tt.claims); got != tt.want {
				t.Fatalf("isUserAccessClaims() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCapabilityMiddlewareEnforcesServerSideAuthorization(t *testing.T) {
	_, api := humatest.New(t)
	called := false
	type probeOutput struct {
		Body struct {
			Authorized bool `json:"authorized"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "authorization-probe",
		Method:      http.MethodGet,
		Path:        "/authorization-probe",
		Middlewares: huma.Middlewares{requireCapability(api, auth.CapabilityPlatformAdmin)},
	}, func(context.Context, *struct{}) (*probeOutput, error) {
		called = true
		out := &probeOutput{}
		out.Body.Authorized = true
		return out, nil
	})

	tests := []struct {
		name       string
		claims     *auth.Claims
		wantStatus int
		wantCall   bool
	}{
		{name: "super admin", claims: &auth.Claims{UserID: "sa", UserType: 1}, wantStatus: http.StatusOK, wantCall: true},
		{name: "platform admin", claims: &auth.Claims{UserID: "pa", UserType: 2}, wantStatus: http.StatusOK, wantCall: true},
		{name: "tenant", claims: &auth.Claims{UserID: "tenant", TenantID: "t1", UserType: 3}, wantStatus: http.StatusForbidden},
		{name: "customer", claims: &auth.Claims{UserID: "customer", TenantID: "t1", UserType: 4}, wantStatus: http.StatusForbidden},
		{name: "unknown role", claims: &auth.Claims{UserID: "unknown", UserType: 99}, wantStatus: http.StatusForbidden},
		{name: "missing claims", wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			ctx := context.Background()
			if tt.claims != nil {
				ctx = context.WithValue(ctx, userClaimsCtxKey, tt.claims)
			}
			response := api.GetCtx(ctx, "/authorization-probe")
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if called != tt.wantCall {
				t.Fatalf("handler called = %v, want %v", called, tt.wantCall)
			}
		})
	}
}

func TestRequireRecentAuthIsSessionBound(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mini.Close)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	recent := auth.NewRecentAuthService(client)
	if err := recent.Mark(context.Background(), "admin-1", "session-a", "password"); err != nil {
		t.Fatal(err)
	}

	_, api := humatest.New(t)
	called := false
	type probeOutput struct {
		Body struct {
			OK bool
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "recent-auth-session-probe",
		Method:      http.MethodGet,
		Path:        "/recent-auth-session-probe",
		Middlewares: huma.Middlewares{requireRecentAuth(api, recent)},
	}, func(context.Context, *struct{}) (*probeOutput, error) {
		called = true
		out := &probeOutput{}
		out.Body.OK = true
		return out, nil
	})

	sessionA := context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{UserID: "admin-1", SessionID: "session-a"})
	response := api.GetCtx(sessionA, "/recent-auth-session-probe")
	if response.Code != http.StatusOK || !called {
		t.Fatalf("session A status/called = %d/%v, want 200/true; body=%s", response.Code, called, response.Body.String())
	}

	called = false
	sessionB := context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{UserID: "admin-1", SessionID: "session-b"})
	response = api.GetCtx(sessionB, "/recent-auth-session-probe")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("session B status = %d, want 401; body=%s", response.Code, response.Body.String())
	}
	if called {
		t.Fatal("session B inherited recent-auth from session A")
	}
}
