package authn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"xiaodou/dai/libs/go/httpx"
)

const testKID = "k1"

// testEnv 起一个返回单把 RSA 公钥的 JWKS server，并提供签发 token 的私钥。
type testEnv struct {
	priv *rsa.PrivateKey
	mgr  *JWKSManager
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewJWKSManager("http://unused.invalid")
	mgr.keys[testKID] = &priv.PublicKey
	return &testEnv{priv: priv, mgr: mgr}
}

func TestVerifyServiceAudienceTypeAndSourceBinding(t *testing.T) {
	env := newTestEnv(t)
	v := NewVerifier(env.mgr, "urm")
	now := time.Now()
	valid := Claims{
		PrincipalType: "service", TokenUse: "access", ClientID: "ai-service",
		InstanceID: "instance-1", SourceCIDR: "192.0.2.0/24",
		RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{ServiceAudience}, NotBefore: jwt.NewNumericDate(now.Add(-time.Second))},
	}
	if _, err := v.VerifyService(context.Background(), env.sign(t, valid, testKID), netip.MustParseAddr("192.0.2.7")); err != nil {
		t.Fatalf("valid service token rejected: %v", err)
	}
	if _, err := v.VerifyService(context.Background(), env.sign(t, valid, testKID), netip.MustParseAddr("198.51.100.7")); err == nil {
		t.Fatal("expected source replay rejection")
	}
	wrongAudience := valid
	wrongAudience.Audience = jwt.ClaimStrings{"other-service"}
	if _, err := v.VerifyService(context.Background(), env.sign(t, wrongAudience, testKID), netip.MustParseAddr("192.0.2.7")); err == nil {
		t.Fatal("expected audience rejection")
	}
	user := valid
	user.PrincipalType, user.TokenUse = "user", "access"
	if _, err := v.VerifyService(context.Background(), env.sign(t, user, testKID), netip.MustParseAddr("192.0.2.7")); err == nil {
		t.Fatal("expected user-token rejection")
	}
	expired := valid
	expired.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute))
	if _, err := v.VerifyService(context.Background(), env.sign(t, expired, testKID), netip.MustParseAddr("192.0.2.7")); err == nil {
		t.Fatal("expected expired-token rejection")
	}
}

func (e *testEnv) sign(t *testing.T, c Claims, kid string) string {
	t.Helper()
	if c.Issuer == "" {
		c.Issuer = "urm"
	}
	if c.ExpiresAt == nil {
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, &c)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(e.priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestVerify_ValidUserToken(t *testing.T) {
	env := newTestEnv(t)
	v := NewVerifier(env.mgr, "urm")
	tokenStr := env.sign(t, Claims{UserID: "u1", TenantID: "t1", Role: "admin", UserType: 1}, testKID)

	claims, err := v.Verify(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != "u1" || claims.TenantID != "t1" || claims.Role != "admin" {
		t.Errorf("unexpected claims: %+v", claims)
	}
}

func TestVerify_RejectsWrongIssuerAndBadKey(t *testing.T) {
	env := newTestEnv(t)
	v := NewVerifier(env.mgr, "urm")

	if _, err := v.Verify(context.Background(), env.sign(t, Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: "evil"}}, testKID)); err == nil {
		t.Error("expected wrong-issuer rejection")
	}
	if _, err := v.Verify(context.Background(), env.sign(t, Claims{UserID: "u1"}, "unknown-kid")); err == nil {
		t.Error("expected unknown-kid rejection")
	}
}

func TestMiddleware_InjectsClaims(t *testing.T) {
	env := newTestEnv(t)
	v := NewVerifier(env.mgr, "urm")

	var got *Claims
	h := v.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+env.sign(t, Claims{UserID: "u9"}, testKID))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got == nil || got.UserID != "u9" {
		t.Fatalf("claims not injected: %+v", got)
	}
}

func TestMiddleware_MissingTokenIsProblem401(t *testing.T) {
	env := newTestEnv(t)
	v := NewVerifier(env.mgr, "urm")
	h := v.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Errorf("content-type = %q", ct)
	}
}

func TestRequireService(t *testing.T) {
	env := newTestEnv(t)
	v := NewVerifier(env.mgr, "urm")

	chain := func(clientID string) http.Handler {
		return v.Middleware()(RequireService(clientID)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))
	}
	do := func(h http.Handler, token string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	svcTok := env.sign(t, Claims{PrincipalType: "service", TokenUse: "access", ClientID: "proxy-service"}, testKID)
	userTok := env.sign(t, Claims{UserID: "u1"}, testKID)

	if code := do(chain("proxy-service"), svcTok); code != http.StatusOK {
		t.Errorf("matching service token: status = %d, want 200", code)
	}
	if code := do(chain("ai-service"), svcTok); code != http.StatusForbidden {
		t.Errorf("mismatched client_id: status = %d, want 403", code)
	}
	if code := do(chain(""), userTok); code != http.StatusForbidden {
		t.Errorf("user token on service-only route: status = %d, want 403", code)
	}
}
