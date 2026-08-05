package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

const appKeysTestSchema = `
	CREATE TABLE ai_app_keys (
	  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	  owner_type TEXT NOT NULL,
	  tenant_id TEXT NOT NULL,
	  user_id TEXT NOT NULL DEFAULT '',
	  name TEXT NOT NULL,
	  key_hash TEXT NOT NULL UNIQUE,
	  key_ciphertext TEXT NOT NULL,
	  last_four TEXT NOT NULL,
	  status TEXT NOT NULL,
	  app_id UUID NOT NULL,
	  expires_at TIMESTAMPTZ,
	  created_by TEXT,
	  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)
`

func TestApplicationInvokeKeyRepoCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, fixture, cleanup, err := testsupport.OpenAppLayerTestPool(ctx)
	if err != nil {
		t.Skipf("open app layer test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()

	mustExecAppRuntime(t, ctx, pool, appKeysTestSchema)

	repo := NewApplicationInvokeKeyRepo(pool)
	expiresAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	created, err := repo.CreateInvokeKey(ctx, application.InvokeKeyWrite{
		OwnerScope:    "tenant",
		TenantID:      "tenant-a",
		Name:          "ak-1",
		KeyHash:       "hash-app-1",
		KeyCiphertext: "ciphertext-app-1",
		LastFour:      "1234",
		Status:        application.StatusActive,
		AppID:         fixture.TenantAAgentID,
		ExpiresAt:     &expiresAt,
		CreatedBy:     "tester",
	})
	if err != nil {
		t.Fatalf("CreateInvokeKey: %v", err)
	}
	if created.OwnerScope != "tenant" || created.AppID != fixture.TenantAAgentID {
		t.Fatalf("unexpected created key: %#v", created)
	}

	listed, err := repo.ListInvokeKeys(ctx, application.InvokeKeyFilter{
		OwnerScope: "tenant",
		TenantID:   "tenant-a",
	})
	if err != nil {
		t.Fatalf("ListInvokeKeys: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed invoke keys = %#v", listed)
	}

	loaded, err := repo.GetInvokeKeyByID(ctx, "tenant", "tenant-a", "", created.ID)
	if err != nil {
		t.Fatalf("GetInvokeKeyByID: %v", err)
	}
	if loaded.KeyHash != "hash-app-1" || loaded.AppID != fixture.TenantAAgentID {
		t.Fatalf("unexpected loaded key: %#v", loaded)
	}

	updated, err := repo.UpdateInvokeKey(ctx, created.ID, application.InvokeKeyWrite{
		OwnerScope: "tenant",
		TenantID:   "tenant-a",
		Name:       "ak-1-updated",
		Status:     application.StatusDisabled,
		AppID:      fixture.TenantAAgentID,
		ExpiresAt:  nil,
	})
	if err != nil {
		t.Fatalf("UpdateInvokeKey: %v", err)
	}
	if updated.Name != "ak-1-updated" || updated.Status != application.StatusDisabled {
		t.Fatalf("unexpected updated key: %#v", updated)
	}

	ciphertext, err := repo.RevealInvokeKey(ctx, "tenant", "tenant-a", "", created.ID)
	if err != nil {
		t.Fatalf("RevealInvokeKey: %v", err)
	}
	if ciphertext != "ciphertext-app-1" {
		t.Fatalf("unexpected revealed ciphertext: %q", ciphertext)
	}

	rotated, err := repo.RotateInvokeKey(ctx, created.ID, application.InvokeKeyRotate{
		OwnerScope:    "tenant",
		TenantID:      "tenant-a",
		KeyHash:       "hash-app-1-rotated",
		KeyCiphertext: "ciphertext-app-1-rotated",
		LastFour:      "5678",
	})
	if err != nil {
		t.Fatalf("RotateInvokeKey: %v", err)
	}
	if rotated.KeyHash != "hash-app-1-rotated" || rotated.LastFour != "5678" || rotated.AppID != fixture.TenantAAgentID {
		t.Fatalf("unexpected rotated key: %#v", rotated)
	}

	if err := repo.DeleteInvokeKey(ctx, "tenant", "tenant-a", "", created.ID); err != nil {
		t.Fatalf("DeleteInvokeKey: %v", err)
	}
	_, err = repo.GetInvokeKeyByID(ctx, "tenant", "tenant-a", "", created.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestApplicationInvokeKeyRepoGetByHash(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, fixture, cleanup, err := testsupport.OpenAppLayerTestPool(ctx)
	if err != nil {
		t.Skipf("open app layer test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()

	mustExecAppRuntime(t, ctx, pool, appKeysTestSchema)

	repo := NewApplicationInvokeKeyRepo(pool)
	created, err := repo.CreateInvokeKey(ctx, application.InvokeKeyWrite{
		OwnerScope:    "user",
		TenantID:      "tenant-a",
		UserID:        "user-a",
		Name:          "ak-app",
		KeyHash:       "hash-app",
		KeyCiphertext: "ciphertext-app",
		LastFour:      "5678",
		Status:        application.StatusActive,
		AppID:         fixture.TenantAAgentID,
		CreatedBy:     "tester",
	})
	if err != nil {
		t.Fatalf("CreateInvokeKey(app): %v", err)
	}

	loaded, err := repo.GetInvokeKeyByHash(ctx, "hash-app")
	if err != nil {
		t.Fatalf("GetInvokeKeyByHash: %v", err)
	}
	if loaded.ID != created.ID || loaded.OwnerScope != "user" || loaded.AppID != fixture.TenantAAgentID {
		t.Fatalf("unexpected loaded invoke key: %#v", loaded)
	}
}
