package transport

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/auth"
)

type revocationCheckerStub struct {
	revokedJTI string
	logoutTime int64
}

func (s revocationCheckerStub) IsBlacklisted(_ context.Context, tokenID string) bool {
	return tokenID == s.revokedJTI
}

func (s revocationCheckerStub) GetUserLogoutTime(context.Context, string) int64 {
	return s.logoutTime
}

func TestTokenRevoked(t *testing.T) {
	issuedAt := time.Unix(100, 0)
	claims := &auth.Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:       "token-1",
			IssuedAt: jwt.NewNumericDate(issuedAt),
		},
	}

	ctx := context.Background()
	if tokenRevoked(ctx, nil, claims) {
		t.Fatal("nil checker must not revoke tokens")
	}
	if !tokenRevoked(ctx, revocationCheckerStub{revokedJTI: "token-1"}, claims) {
		t.Fatal("blacklisted token ID must be rejected")
	}
	if !tokenRevoked(ctx, revocationCheckerStub{logoutTime: issuedAt.Unix() + 1}, claims) {
		t.Fatal("token issued before user logout must be rejected")
	}
	if tokenRevoked(ctx, revocationCheckerStub{logoutTime: issuedAt.Unix()}, claims) {
		t.Fatal("token issued at logout timestamp must remain valid")
	}
}

type platformAuthTokenVerifierStub struct{}

func (platformAuthTokenVerifierStub) ParseToken(context.Context, string) (*auth.Claims, error) {
	return &auth.Claims{
		PrincipalType: "user",
		TokenUse:      "access",
		SessionID:     "session-1",
		UserID:        "admin-1",
		UserType:      int(auth.UserTypePlatformAdmin),
	}, nil
}

func TestPlatformUserAuthRecentAuthenticationOnlyBlocksMutations(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	recent := auth.NewRecentAuthService(client)
	deps := HTTPAuthDeps{TokenVerifier: platformAuthTokenVerifierStub{}, RecentAuth: recent}

	_, api := humatest.New(t)
	called := 0
	type output struct {
		Body struct {
			OK bool
		}
	}
	for _, route := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/read"},
		{method: http.MethodPost, path: "/write"},
	} {
		route := route
		huma.Register(api, huma.Operation{
			OperationID: "recent-auth-" + route.method,
			Method:      route.method,
			Path:        route.path,
			Middlewares: huma.Middlewares{platformUserAuth(api, deps)},
		}, func(context.Context, *struct{}) (*output, error) {
			called++
			out := &output{}
			out.Body.OK = true
			return out, nil
		})
	}

	if response := api.Post("/write", "Authorization: Bearer token"); response.Code != http.StatusUnauthorized {
		t.Fatalf("mutation without recent auth status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if called != 0 {
		t.Fatal("mutation handler ran without recent authentication")
	}
	if response := api.Get("/read", "Authorization: Bearer token"); response.Code != http.StatusOK {
		t.Fatalf("read without recent auth status = %d, want %d", response.Code, http.StatusOK)
	}
	if err := recent.Mark(context.Background(), "admin-1", "test"); err != nil {
		t.Fatal(err)
	}
	if response := api.Post("/write", "Authorization: Bearer token"); response.Code != http.StatusOK {
		t.Fatalf("mutation with recent auth status = %d, want %d", response.Code, http.StatusOK)
	}
	if called != 2 {
		t.Fatalf("handler calls = %d, want 2", called)
	}
}
