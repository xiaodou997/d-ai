package config

import "testing"

func TestLoadUsesUnifiedSecretAndStorageEnvironment(t *testing.T) {
	setRequiredEnvironment(t, "development")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY", "urm-secret")
	t.Setenv("DAI_DATA_DIR", "/data")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Security.SecretMasterKey != "urm-secret" {
		t.Fatalf("secret master key = %q, want unified value", cfg.Security.SecretMasterKey)
	}
	if cfg.Storage.DataDir != "/data" {
		t.Fatalf("data dir = %q, want /data", cfg.Storage.DataDir)
	}
}

func TestLoadRequiresSecretMasterKeyInProduction(t *testing.T) {
	setRequiredEnvironment(t, "production")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without the production secret master key")
	}
}

func setRequiredEnvironment(t *testing.T, appEnv string) {
	t.Helper()
	t.Setenv("DAI_APP_ENV", appEnv)
	t.Setenv("DAI_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/dai?sslmode=disable")
	t.Setenv("DAI_REDIS_ADDR", "127.0.0.1:6379")
}
