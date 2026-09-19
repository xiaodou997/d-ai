package proxy

import (
	"context"
	"testing"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/dbtest"
)

func TestUpsertUpdatePreservesDisabledStatusAndPasswordWhenOmitted(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	service := NewService(pool, nil)
	created, err := service.Upsert(ctx, "", UpsertInput{
		Name:      "proxy-status-preserve",
		ProxyType: "http",
		Endpoint:  "http://proxy-preserve.example.com:8080",
		Username:  "proxy-user",
		Password:  "proxy-long-lived-secret",
		Weight:    1,
		Status:    "disabled",
	}, "security-test")
	if err != nil {
		t.Fatal(err)
	}

	var before string
	if err := pool.QueryRow(ctx, `
		SELECT proxy_password_enc FROM ai_proxy_nodes WHERE id = $1::uuid
	`, created.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before == "" {
		t.Fatal("expected encrypted proxy password")
	}

	updated, err := service.Upsert(ctx, created.ID, UpsertInput{
		Name:      "proxy-status-preserve-renamed",
		ProxyType: "http",
		Endpoint:  "http://proxy-preserve.example.com:8080",
		Username:  "proxy-user",
		Weight:    2,
	}, "security-test")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "disabled" {
		t.Fatalf("omitted update status = %q, want disabled", updated.Status)
	}

	var after, status string
	if err := pool.QueryRow(ctx, `
		SELECT proxy_password_enc, status FROM ai_proxy_nodes WHERE id = $1::uuid
	`, created.ID).Scan(&after, &status); err != nil {
		t.Fatal(err)
	}
	if status != "disabled" {
		t.Fatalf("persisted update status = %q, want disabled", status)
	}
	if after != before {
		t.Fatal("proxy password changed when update omitted password")
	}
	selected, err := service.SelectProxy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if selected != nil {
		t.Fatalf("disabled proxy was selected after metadata update: %v", selected)
	}
}

func TestUpsertCreateStillDefaultsStatusActive(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewService(pool, nil)
	created, err := service.Upsert(ctx, "", UpsertInput{
		Name:      "proxy-create-default",
		ProxyType: "http",
		Endpoint:  "http://proxy-create-default.example.com:8080",
		Weight:    1,
	}, "security-test")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "active" {
		t.Fatalf("create status = %q, want active", created.Status)
	}
}
