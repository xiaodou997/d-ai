package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/weborigin"
)

// Config 是 D-AI 单进程应用配置。
type Config struct {
	App        AppConfig       `mapstructure:"app"`
	Server     ServerConfig    `mapstructure:"server"`
	Database   DatabaseConfig  `mapstructure:"database"`
	Redis      RedisConfig     `mapstructure:"redis"`
	JWT        JWTConfig       `mapstructure:"jwt"`
	Auth       AuthConfig      `mapstructure:"auth"`
	Security   SecurityConfig  `mapstructure:"security"`
	Legal      LegalConfig     `mapstructure:"legal"`
	Storage    StorageConfig   `mapstructure:"storage"`
	Log        LogConfig       `mapstructure:"log"`
	Pricing    PricingConfig   `mapstructure:"pricing"`
	Image      ImageConfig     `mapstructure:"image_assets"`
	AsyncTasks AsyncTaskConfig `mapstructure:"async_tasks"`
	Files      FileStoreConfig `mapstructure:"file_store"`
	Audit      AuditConfig     `mapstructure:"audit"`
	Runtime    RuntimeConfig   `mapstructure:"runtime"`
}

// RuntimeConfig holds admission limits that apply when no explicit per-scope
// policy is configured.
type RuntimeConfig struct {
	// DefaultInFlightPerAccount caps concurrent billed requests for one tenant
	// or one end user. It is what makes billing overshoot a finite number:
	// settlement is post-paid, so an account can overdraw by at most this many
	// requests' worth before admission refuses the next one. 0 removes the cap
	// and with it the bound.
	DefaultInFlightPerAccount int `mapstructure:"default_in_flight_per_account"`
}

// ─── 通用配置 ──────────────────────────────────────────

type AppConfig struct {
	Env string `mapstructure:"env"` // development | production
}

type ServerConfig struct {
	Addr              string   `mapstructure:"addr"`            // e.g. ":19641"
	ManagementAddr    string   `mapstructure:"management_addr"` // metrics/probes listener, loopback by default
	ReadTimeout       int      `mapstructure:"read_timeout"`
	IdleTimeout       int      `mapstructure:"idle_timeout"`
	MaxBodyBytes      int64    `mapstructure:"max_body_bytes"`
	MaxHeaderBytes    int      `mapstructure:"max_header_bytes"`
	PublicBaseURL     string   `mapstructure:"public_base_url"`
	TrustedProxyCIDRs []string `mapstructure:"trusted_proxy_cidrs"`
}

type DatabaseConfig struct {
	URL             string        `mapstructure:"url"`
	DSN             string        `mapstructure:"dsn"`
	MaxConns        int32         `mapstructure:"max_conns"`
	MinConns        int32         `mapstructure:"min_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Expiration        time.Duration `mapstructure:"expiration"`
	RefreshExpiration time.Duration `mapstructure:"refresh_expiration"`
	Issuer            string        `mapstructure:"issuer"`
}

type AuthConfig struct {
	ActivationExpiration time.Duration `mapstructure:"activation_expiration"`
}

type SecurityConfig struct {
	SecretMasterKey         string   `mapstructure:"secret_master_key"`          // URM 与 AI 敏感配置加密
	SecretMasterKeyID       string   `mapstructure:"secret_master_key_id"`       // 当前密钥版本
	SecretMasterKeyPrevious []string `mapstructure:"secret_master_key_previous"` // 旧密钥，格式为 keyID=key
}

type LegalConfig struct {
	TermsVersion   string `mapstructure:"terms_version"`
	PrivacyVersion string `mapstructure:"privacy_version"`
}

type StorageConfig struct {
	DataDir string `mapstructure:"data_dir"`
}

type PricingConfig struct {
	LiteLLMURL string `mapstructure:"litellm_url"`
}

type ImageConfig struct {
	Retention time.Duration `mapstructure:"retention"`
}

type AsyncTaskConfig struct {
	Workers              int           `mapstructure:"workers"`
	PollInterval         time.Duration `mapstructure:"poll_interval"`
	LeaseTTL             time.Duration `mapstructure:"lease_ttl"`
	MaxInFlightPerTenant int           `mapstructure:"max_in_flight_per_tenant"`
	Retention            time.Duration `mapstructure:"retention"`
	ReapInterval         time.Duration `mapstructure:"reap_interval"`
	ReapBatch            int           `mapstructure:"reap_batch"`
	MaxUploadBodySize    int64         `mapstructure:"max_upload_body_size"`
	WebhookWorkers       int           `mapstructure:"webhook_workers"`
	WebhookPollInterval  time.Duration `mapstructure:"webhook_poll_interval"`
	WebhookLeaseTTL      time.Duration `mapstructure:"webhook_lease_ttl"`
}

type FileStoreConfig struct {
	AssetTTL time.Duration `mapstructure:"asset_ttl"`
	URLTTL   time.Duration `mapstructure:"url_ttl"`
	MaxBytes int64         `mapstructure:"max_bytes"`
}

type AuditConfig struct {
	StoreImageBlobs bool `mapstructure:"store_image_blobs"`
}

type LogConfig struct {
	Level  string   `mapstructure:"level"`
	File   string   `mapstructure:"file"`
	Redact []string `mapstructure:"redact"`
}

// ─── Load ──────────────────────────────────────────────

// Load 加载配置
// 优先级：环境变量 > config.yaml > 默认值
// 环境变量前缀：DAI_
func Load() (*Config, error) {
	v := viper.New()

	// 默认值 —— 通用
	v.SetDefault("app.env", "development")
	v.SetDefault("server.addr", ":19641")
	v.SetDefault("server.management_addr", "127.0.0.1:19642")
	v.SetDefault("server.read_timeout", 30)
	v.SetDefault("server.idle_timeout", 60)
	v.SetDefault("server.max_body_bytes", int64(64<<20))
	v.SetDefault("server.max_header_bytes", 32<<10)
	v.SetDefault("server.public_base_url", "")
	v.SetDefault("server.trusted_proxy_cidrs", []string{})

	// 默认值 —— 数据库（统一用 URL，兼容 DSN）
	v.SetDefault("database.url", "postgres://postgres:postgres@localhost:5432/dai?sslmode=disable")
	v.SetDefault("database.max_conns", 20)
	v.SetDefault("database.min_conns", 2)
	v.SetDefault("database.max_conn_lifetime", "1h")

	// 默认值 —— Redis
	v.SetDefault("redis.addr", "")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	// 默认值 —— JWT
	v.SetDefault("jwt.expiration", "15m")
	v.SetDefault("jwt.refresh_expiration", "168h")
	v.SetDefault("jwt.issuer", "dai")
	v.SetDefault("auth.activation_expiration", "24h")

	// 默认值 —— Security
	v.SetDefault("security.secret_master_key", "")
	v.SetDefault("security.secret_master_key_id", "v1")
	v.SetDefault("security.secret_master_key_previous", []string{})

	// 默认值 —— Legal
	v.SetDefault("legal.terms_version", "2026-07-19")
	v.SetDefault("legal.privacy_version", "2026-07-19")

	// 默认值 —— Storage
	v.SetDefault("storage.data_dir", "data")

	// 默认值 —— Log
	v.SetDefault("log.level", "info")
	v.SetDefault("log.file", "")
	v.SetDefault("log.redact", []string{})

	// 默认值 —— AI 域
	v.SetDefault("pricing.litellm_url", "")
	v.SetDefault("audit.store_image_blobs", false)
	v.SetDefault("image_assets.retention", "24h")
	v.SetDefault("runtime.default_in_flight_per_account", 32)

	v.SetDefault("async_tasks.workers", 2)
	v.SetDefault("async_tasks.poll_interval", "2s")
	v.SetDefault("async_tasks.lease_ttl", "60s")
	v.SetDefault("async_tasks.max_in_flight_per_tenant", 16)
	v.SetDefault("async_tasks.retention", "24h")
	v.SetDefault("async_tasks.reap_interval", "30s")
	v.SetDefault("async_tasks.reap_batch", 64)
	v.SetDefault("async_tasks.max_upload_body_size", 64<<20)
	v.SetDefault("async_tasks.webhook_workers", 2)
	v.SetDefault("async_tasks.webhook_poll_interval", "2s")
	v.SetDefault("async_tasks.webhook_lease_ttl", "30s")
	v.SetDefault("file_store.asset_ttl", "24h")
	v.SetDefault("file_store.url_ttl", "24h")
	v.SetDefault("file_store.max_bytes", 32<<20)

	// 配置文件
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	// 环境变量（前缀：DAI_）
	v.SetEnvPrefix("DAI")
	v.AutomaticEnv()
	bindEnvs(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// 显式覆盖关键字段
	applyEnvOverrides(&cfg)

	// 校验
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func bindEnvs(v *viper.Viper) {
	envBindings := map[string]string{
		// 通用
		"DAI_APP_ENV":                               "app.env",
		"DAI_SERVER_ADDR":                           "server.addr",
		"DAI_SERVER_MANAGEMENT_ADDR":                "server.management_addr",
		"DAI_SERVER_MAX_BODY_BYTES":                 "server.max_body_bytes",
		"DAI_SERVER_MAX_HEADER_BYTES":               "server.max_header_bytes",
		"DAI_PUBLIC_BASE_URL":                       "server.public_base_url",
		"DAI_TRUSTED_PROXY_CIDRS":                   "server.trusted_proxy_cidrs",
		"DAI_DATABASE_URL":                          "database.url",
		"DAI_DATABASE_DSN":                          "database.dsn",
		"DAI_DB_MAX_CONNS":                          "database.max_conns",
		"DAI_DB_MIN_CONNS":                          "database.min_conns",
		"DAI_DB_MAX_CONN_LIFETIME":                  "database.max_conn_lifetime",
		"DAI_RUNTIME_DEFAULT_IN_FLIGHT_PER_ACCOUNT": "runtime.default_in_flight_per_account",
		"DAI_REDIS_ADDR":                            "redis.addr",
		"DAI_REDIS_PASSWORD":                        "redis.password",
		"DAI_REDIS_DB":                              "redis.db",
		"DAI_JWT_EXPIRATION":                        "jwt.expiration",
		"DAI_JWT_REFRESH_EXPIRATION":                "jwt.refresh_expiration",
		"DAI_AUTH_ACTIVATION_EXPIRATION":            "auth.activation_expiration",
		// Security
		"DAI_SECURITY_SECRET_MASTER_KEY":          "security.secret_master_key",
		"DAI_SECURITY_SECRET_MASTER_KEY_ID":       "security.secret_master_key_id",
		"DAI_SECURITY_SECRET_MASTER_KEY_PREVIOUS": "security.secret_master_key_previous",
		// Legal
		"DAI_LEGAL_TERMS_VERSION":   "legal.terms_version",
		"DAI_LEGAL_PRIVACY_VERSION": "legal.privacy_version",
		// Storage
		"DAI_DATA_DIR": "storage.data_dir",
		// Log
		"DAI_LOG_LEVEL":  "log.level",
		"DAI_LOG_FILE":   "log.file",
		"DAI_LOG_REDACT": "log.redact",
		// AI
		"DAI_PRICING_LITELLM_URL":          "pricing.litellm_url",
		"DAI_AUDIT_STORE_IMAGE_BLOBS":      "audit.store_image_blobs",
		"DAI_IMAGE_ASSET_RETENTION":        "image_assets.retention",
		"DAI_ASYNC_TASK_WORKERS":           "async_tasks.workers",
		"DAI_ASYNC_TASK_POLL_INTERVAL":     "async_tasks.poll_interval",
		"DAI_ASYNC_TASK_LEASE_TTL":         "async_tasks.lease_ttl",
		"DAI_ASYNC_TASK_MAX_IN_FLIGHT":     "async_tasks.max_in_flight_per_tenant",
		"DAI_ASYNC_TASK_RETENTION":         "async_tasks.retention",
		"DAI_ASYNC_TASK_REAP_INTERVAL":     "async_tasks.reap_interval",
		"DAI_ASYNC_TASK_REAP_BATCH":        "async_tasks.reap_batch",
		"DAI_ASYNC_TASK_MAX_UPLOAD_BYTES":  "async_tasks.max_upload_body_size",
		"DAI_ASYNC_TASK_WEBHOOK_WORKERS":   "async_tasks.webhook_workers",
		"DAI_ASYNC_TASK_WEBHOOK_POLL":      "async_tasks.webhook_poll_interval",
		"DAI_ASYNC_TASK_WEBHOOK_LEASE_TTL": "async_tasks.webhook_lease_ttl",
		"DAI_FILE_ASSET_TTL":               "file_store.asset_ttl",
		"DAI_FILE_URL_TTL":                 "file_store.url_ttl",
		"DAI_FILE_MAX_BYTES":               "file_store.max_bytes",
	}
	for env, key := range envBindings {
		_ = v.BindEnv(key, env)
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("DAI_APP_ENV"); v != "" {
		cfg.App.Env = v
	}
	if v := os.Getenv("DAI_SERVER_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v, ok := os.LookupEnv("DAI_SERVER_MANAGEMENT_ADDR"); ok {
		cfg.Server.ManagementAddr = strings.TrimSpace(v)
	}
	if v := os.Getenv("DAI_PUBLIC_BASE_URL"); v != "" {
		cfg.Server.PublicBaseURL = v
	}
	if v := os.Getenv("DAI_TRUSTED_PROXY_CIDRS"); v != "" {
		cfg.Server.TrustedProxyCIDRs = splitList(v)
	}
	if v := os.Getenv("DAI_DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("DAI_DATABASE_DSN"); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("DAI_REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("DAI_REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("DAI_REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = n
		}
	}
	if v := os.Getenv("DAI_SECURITY_SECRET_MASTER_KEY"); v != "" {
		cfg.Security.SecretMasterKey = v
	}
	if v := os.Getenv("DAI_SECURITY_SECRET_MASTER_KEY_ID"); v != "" {
		cfg.Security.SecretMasterKeyID = v
	}
	if v := os.Getenv("DAI_SECURITY_SECRET_MASTER_KEY_PREVIOUS"); v != "" {
		cfg.Security.SecretMasterKeyPrevious = splitList(v)
	}
	if v := os.Getenv("DAI_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}

// ─── Validation ────────────────────────────────────────

func validate(cfg *Config) error {
	// 数据库：URL 或 DSN 至少一个
	dbConn := cfg.Database.URL
	if dbConn == "" {
		dbConn = cfg.Database.DSN
	}
	if dbConn == "" {
		return fmt.Errorf("database.url or database.dsn is required")
	}

	// Redis 必填
	if strings.TrimSpace(cfg.Redis.Addr) == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if cfg.App.Env == "production" && strings.TrimSpace(cfg.Security.SecretMasterKey) == "" {
		return fmt.Errorf("security.secret_master_key is required in production")
	}
	if cfg.App.Env == "production" {
		previous, err := ParsePreviousSecretKeys(cfg.Security.SecretMasterKeyPrevious)
		if err != nil {
			return err
		}
		if _, err := clientsecret.NewKeyring(cfg.Security.SecretMasterKeyID, cfg.Security.SecretMasterKey, previous); err != nil {
			return fmt.Errorf("invalid security secret keyring: %w", err)
		}
	}
	if cfg.App.Env == "production" && strings.TrimSpace(cfg.Server.PublicBaseURL) == "" {
		return fmt.Errorf("server.public_base_url is required in production")
	}
	if cfg.App.Env == "production" && strings.TrimSpace(cfg.Server.ManagementAddr) == "" {
		return fmt.Errorf("server.management_addr is required in production")
	}
	if cfg.Server.MaxBodyBytes <= 0 {
		return fmt.Errorf("server.max_body_bytes must be positive")
	}
	if cfg.Server.MaxHeaderBytes < 1024 {
		return fmt.Errorf("server.max_header_bytes must be at least 1024")
	}
	normalizedOrigin, err := weborigin.NormalizePublicOrigin(cfg.Server.PublicBaseURL)
	if err != nil {
		return fmt.Errorf("invalid HTTP origin/proxy configuration: %w", err)
	}
	if cfg.App.Env == "production" {
		parsed, _ := url.Parse(normalizedOrigin)
		if parsed == nil || parsed.Scheme != "https" {
			return fmt.Errorf("server.public_base_url must use https in production")
		}
	}
	if _, err := weborigin.NewResolver(normalizedOrigin, cfg.Server.TrustedProxyCIDRs); err != nil {
		return fmt.Errorf("invalid HTTP origin/proxy configuration: %w", err)
	}

	return nil
}

// ParsePreviousSecretKeys converts configuration entries of the form
// "keyID=key" into the keyring map. Split only at the first '=' so padded
// base64 values remain intact.
func ParsePreviousSecretKeys(entries []string) (map[string]string, error) {
	previous := make(map[string]string, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		id, key, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("security.secret_master_key_previous entry %q must use keyID=key format", entry)
		}
		id = strings.TrimSpace(id)
		if _, exists := previous[id]; exists {
			return nil, fmt.Errorf("duplicate previous secret master key ID %q", id)
		}
		previous[id] = strings.TrimSpace(key)
	}
	return previous, nil
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if normalized := strings.TrimSpace(part); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

// ─── Helpers ───────────────────────────────────────────

// DSN 返回最终用于 pgx 连接的 DSN（URL 优先，DSN 后备）
func (c *DatabaseConfig) DSNString() string {
	if c.URL != "" {
		return c.URL
	}
	return c.DSN
}
