package proxy

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/clientsecret"
)

func TestUpsertKeepsPasswordWhenUpdateOmitsIt(t *testing.T) {
	dsn := os.Getenv("GOPAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GOPAW_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	var tableExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.ai_proxy_nodes') IS NOT NULL`).Scan(&tableExists); err != nil || !tableExists {
		t.Skip("ai_proxy_nodes is not available in the test database")
	}
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("configure test cipher: %v", err)
	}

	service := NewService(pool, nil)
	created, err := service.Upsert(ctx, "", UpsertInput{
		Name:      "password-regression",
		ProxyType: "http",
		Endpoint:  "http://proxy.example.com:8080",
		Username:  "tester",
		Password:  "initial-secret",
		Weight:    1,
		Status:    "disabled",
	}, "test")
	if err != nil {
		t.Fatalf("create proxy node: %v", err)
	}
	t.Cleanup(func() { _ = service.Delete(context.Background(), created.ID) })

	var before string
	if err := pool.QueryRow(ctx, `SELECT proxy_password_enc FROM ai_proxy_nodes WHERE id = $1`, created.ID).Scan(&before); err != nil {
		t.Fatalf("read initial password: %v", err)
	}
	if before == "" {
		t.Fatal("expected encrypted password")
	}

	_, err = service.Upsert(ctx, created.ID, UpsertInput{
		Name:      "password-regression-updated",
		ProxyType: "http",
		Endpoint:  "http://proxy.example.com:8080",
		Username:  "tester",
		Weight:    2,
		Status:    "active",
	}, "test")
	if err != nil {
		t.Fatalf("update proxy node: %v", err)
	}
	var after string
	if err := pool.QueryRow(ctx, `SELECT proxy_password_enc FROM ai_proxy_nodes WHERE id = $1`, created.ID).Scan(&after); err != nil {
		t.Fatalf("read updated password: %v", err)
	}
	if after != before {
		t.Fatalf("password changed when update omitted it")
	}
}
