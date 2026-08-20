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
	if cfg.Server.MaxBodyBytes != 64<<20 || cfg.Server.MaxHeaderBytes != 32<<10 {
		t.Fatalf("HTTP limits = body:%d header:%d", cfg.Server.MaxBodyBytes, cfg.Server.MaxHeaderBytes)
	}
}

func TestLoadParsesManagementListenerAndHTTPLimits(t *testing.T) {
	setRequiredEnvironment(t, "development")
	t.Setenv("DAI_SERVER_MANAGEMENT_ADDR", "127.0.0.1:19699")
	t.Setenv("DAI_SERVER_MAX_BODY_BYTES", "1048576")
	t.Setenv("DAI_SERVER_MAX_HEADER_BYTES", "8192")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ManagementAddr != "127.0.0.1:19699" || cfg.Server.MaxBodyBytes != 1048576 || cfg.Server.MaxHeaderBytes != 8192 {
		t.Fatalf("server security config = %+v", cfg.Server)
	}
}

func TestLoadParsesVersionedSecretKeyring(t *testing.T) {
	setRequiredEnvironment(t, "production")
	active := "0123456789abcdef0123456789abcdef"
	previous := "abcdef0123456789abcdef0123456789"
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY", active)
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY_ID", "v2")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY_PREVIOUS", "v1="+previous)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Security.SecretMasterKeyID != "v2" || len(cfg.Security.SecretMasterKeyPrevious) != 1 {
		t.Fatalf("secret keyring = %#v", cfg.Security)
	}
}

func TestLoadRejectsMalformedPreviousSecretKey(t *testing.T) {
	setRequiredEnvironment(t, "production")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY_PREVIOUS", "v1-without-value")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted malformed previous secret key")
	}
}

func TestLoadRequiresSecretMasterKeyInProduction(t *testing.T) {
	setRequiredEnvironment(t, "production")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without the production secret master key")
	}
}

func TestLoadRequiresPublicBaseURLInProduction(t *testing.T) {
	setRequiredEnvironment(t, "production")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY", "production-secret")
	t.Setenv("DAI_PUBLIC_BASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without the production public base URL")
	}
}

func TestLoadParsesTrustedProxyCIDRs(t *testing.T) {
	setRequiredEnvironment(t, "development")
	t.Setenv("DAI_PUBLIC_BASE_URL", "https://dai.example.test/")
	t.Setenv("DAI_TRUSTED_PROXY_CIDRS", "127.0.0.1/32, 10.0.0.0/8")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.PublicBaseURL != "https://dai.example.test/" {
		t.Fatalf("public base URL = %q", cfg.Server.PublicBaseURL)
	}
	if got, want := len(cfg.Server.TrustedProxyCIDRs), 2; got != want {
		t.Fatalf("trusted proxy CIDRs = %d, want %d", got, want)
	}
}

func TestLoadRejectsHTTPPublicBaseURLInProduction(t *testing.T) {
	setRequiredEnvironment(t, "production")
	t.Setenv("DAI_SECURITY_SECRET_MASTER_KEY", "production-secret")
	t.Setenv("DAI_PUBLIC_BASE_URL", "http://portal.example.test")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted an HTTP public base URL in production")
	}
}

func setRequiredEnvironment(t *testing.T, appEnv string) {
	t.Helper()
	t.Setenv("DAI_APP_ENV", appEnv)
	t.Setenv("DAI_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/dai?sslmode=disable")
	t.Setenv("DAI_REDIS_ADDR", "127.0.0.1:6379")
	if appEnv == "production" {
		t.Setenv("DAI_PUBLIC_BASE_URL", "https://dai.example.test")
	}
}
