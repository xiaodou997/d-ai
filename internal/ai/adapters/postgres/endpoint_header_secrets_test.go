package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/dbtest"
)

func TestProtectSensitiveEndpointHeadersBackfillsAndSecretReaderDecrypts(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		oldKey     = "0123456789abcdef0123456789abcdef"
		newKey     = "abcdef0123456789abcdef0123456789"
		accountID  = "49000000-0000-0000-0000-000000000001"
		endpointID = "49000000-0000-0000-0000-000000000002"
	)
	if err := clientsecret.Configure(oldKey); err != nil {
		t.Fatal(err)
	}
	oldCiphertext, err := clientsecret.Encrypt("legacy-api-key")
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := clientsecret.NewKeyring("v2", newKey, map[string]string{"legacy": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := clientsecret.ConfigureKeyring(keyring); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts (
			id, name, tenant_display_name, api_key_ciphertext, status
		) VALUES ($1::uuid, 'header-secret-account', 'Header Secret Account', 'account-cipher', 'active')
	`, accountID); err != nil {
		t.Fatal(err)
	}
	seed, err := json.Marshal(map[string]string{
		"Authorization": "Bearer historical-secret",
		"X-Api-Key": oldCiphertext,
		"X-Trace": "trace-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_account_endpoints (
			id, account_id, api_format, base_url, extra_headers, status
		) VALUES ($1::uuid, $2::uuid, 'openai_responses', 'https://example.test', $3::jsonb, 'active')
	`, endpointID, accountID, seed); err != nil {
		t.Fatal(err)
	}

	if err := ProtectSensitiveEndpointHeaders(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var storedRaw []byte
	if err := pool.QueryRow(ctx, `
		SELECT extra_headers FROM ai_upstream_account_endpoints WHERE id = $1::uuid
	`, endpointID).Scan(&storedRaw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(storedRaw), "historical-secret") || strings.Contains(string(storedRaw), "legacy-api-key") {
		t.Fatalf("database still contains plaintext sensitive headers: %s", storedRaw)
	}
	var stored map[string]string
	if err := json.Unmarshal(storedRaw, &stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored["Authorization"], "enc:v1:v2:") || !strings.HasPrefix(stored["X-Api-Key"], "enc:v1:v2:") {
		t.Fatalf("sensitive headers were not migrated to active key: %#v", stored)
	}
	if stored["X-Trace"] != "trace-1" {
		t.Fatalf("non-sensitive header changed: %#v", stored)
	}

	repo := NewAccountRepo(NewQueries(pool), pool)
	secret, err := repo.GetAccountSecret(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(secret.Endpoints) != 1 {
		t.Fatalf("secret endpoints = %d, want 1", len(secret.Endpoints))
	}
	var plaintext map[string]string
	if err := json.Unmarshal(secret.Endpoints[0].ExtraHeaders, &plaintext); err != nil {
		t.Fatal(err)
	}
	if plaintext["Authorization"] != "Bearer historical-secret" || plaintext["X-Api-Key"] != "legacy-api-key" || plaintext["X-Trace"] != "trace-1" {
		t.Fatalf("secret reader headers = %#v", plaintext)
	}

	before := string(storedRaw)
	if err := ProtectSensitiveEndpointHeaders(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT extra_headers FROM ai_upstream_account_endpoints WHERE id = $1::uuid
	`, endpointID).Scan(&storedRaw); err != nil {
		t.Fatal(err)
	}
	if string(storedRaw) != before {
		t.Fatalf("second protection pass rewrote active ciphertext: before=%s after=%s", before, storedRaw)
	}
}
