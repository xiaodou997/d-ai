package auth_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/config"
)

// 委托 Token 集成测试；未设 URM_TEST_DATABASE_URL 时跳过（JWTService 需从 DB 加载签名密钥）。
func newJWTService(t *testing.T) *auth.JWTService {
	t.Helper()
	url := os.Getenv("URM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("URM_TEST_DATABASE_URL not set; skipping delegation token test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return auth.NewJWTService(config.JWTConfig{Expiration: time.Hour, Issuer: "urm"}, pool)
}

func TestGenerateDelegatedTokenUserScope(t *testing.T) {
	svc := newJWTService(t)
	tokenStr, expiresIn, err := svc.GenerateDelegatedToken(auth.DelegationRequest{
		ClientID:     "creative-service",
		InstanceID:   "creative-service-1",
		TenantID:     "tenant-123",
		UserID:       "user-456",
		BillingScope: "user",
		Audience:     []string{"ai-service"},
		Scope:        "ai.invoke",
		Expiration:   3 * time.Minute,
	})
	if err != nil {
		t.Fatalf("GenerateDelegatedToken: %v", err)
	}
	if expiresIn != 180 {
		t.Fatalf("expiresIn = %d, want 180", expiresIn)
	}

	// 委托 Token 由同一 URM 密钥签发，ParseToken 应成功并保留全部委托声明。
	claims, err := svc.ParseToken(tokenStr)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.PrincipalType != "delegated" || claims.TokenUse != "access" {
		t.Fatalf("unexpected principal/use: %s/%s", claims.PrincipalType, claims.TokenUse)
	}
	if claims.ClientID != "creative-service" || claims.InstanceID != "creative-service-1" {
		t.Fatalf("unexpected client identity: %+v", claims)
	}
	if claims.TenantID != "tenant-123" || claims.UserID != "user-456" || claims.BillingScope != "user" {
		t.Fatalf("unexpected subject: %+v", claims)
	}
	if claims.Scope != "ai.invoke" || !slices.Contains(claims.Audience, "ai-service") {
		t.Fatalf("unexpected scope/aud: %s / %v", claims.Scope, claims.Audience)
	}
	if claims.Act == nil || claims.Act.Sub != "creative-service" {
		t.Fatalf("unexpected act claim: %+v", claims.Act)
	}
	if claims.Subject != "user-456" {
		t.Fatalf("subject should be the delegated user, got %q", claims.Subject)
	}
}

func TestGenerateDelegatedTokenTenantScope(t *testing.T) {
	svc := newJWTService(t)
	tokenStr, _, err := svc.GenerateDelegatedToken(auth.DelegationRequest{
		ClientID:     "creative-service",
		TenantID:     "tenant-123",
		BillingScope: "tenant",
		Audience:     []string{"ai-service"},
		Scope:        "ai.invoke",
		Expiration:   3 * time.Minute,
	})
	if err != nil {
		t.Fatalf("GenerateDelegatedToken: %v", err)
	}
	claims, err := svc.ParseToken(tokenStr)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.BillingScope != "tenant" || claims.UserID != "" {
		t.Fatalf("tenant scope must not carry user: %+v", claims)
	}
	if claims.Subject != "tenant-123" {
		t.Fatalf("subject should be tenant, got %q", claims.Subject)
	}
}

func TestGenerateDelegatedTokenClampsTTL(t *testing.T) {
	svc := newJWTService(t)
	// 超过 5 分钟应被夹到默认 3 分钟。
	_, expiresIn, err := svc.GenerateDelegatedToken(auth.DelegationRequest{
		ClientID:     "creative-service",
		TenantID:     "tenant-123",
		BillingScope: "tenant",
		Audience:     []string{"ai-service"},
		Scope:        "ai.invoke",
		Expiration:   time.Hour,
	})
	if err != nil {
		t.Fatalf("GenerateDelegatedToken: %v", err)
	}
	if expiresIn != 180 {
		t.Fatalf("TTL should be clamped to 180s, got %d", expiresIn)
	}
}
