package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 是 D-AI 单进程应用配置。
type Config struct {
	App        AppConfig       `mapstructure:"app"`
	Server     ServerConfig    `mapstructure:"server"`
	Database   DatabaseConfig  `mapstructure:"database"`
	Redis      RedisConfig     `mapstructure:"redis"`
	JWT        JWTConfig       `mapstructure:"jwt"`
	Security   SecurityConfig  `mapstructure:"security"`
	Portal     PortalConfig    `mapstructure:"portal"`
	Legal      LegalConfig     `mapstructure:"legal"`
	Log        LogConfig       `mapstructure:"log"`
	Pricing    PricingConfig   `mapstructure:"pricing"`
	Image      ImageConfig     `mapstructure:"image_assets"`
	AsyncTasks AsyncTaskConfig `mapstructure:"async_tasks"`
	Files      FileStoreConfig `mapstructure:"file_store"`
	Audit      AuditConfig     `mapstructure:"audit"`
}

// ─── 通用配置 ──────────────────────────────────────────

type AppConfig struct {
	Env string `mapstructure:"env"` // development | production
}

type ServerConfig struct {
	Addr         string `mapstructure:"addr"` // e.g. ":19641"
	Port         int    `mapstructure:"port"`
	ReadTimeout  int    `mapstructure:"read_timeout"`  // 秒
	WriteTimeout int    `mapstructure:"write_timeout"` // 秒
	IdleTimeout  int    `mapstructure:"idle_timeout"`  // 秒
}

type DatabaseConfig struct {
	URL             string        `mapstructure:"url"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int           `mapstructure:"conn_max_lifetime"` // 分钟
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

type SecurityConfig struct {
	SecretMasterKey   string `mapstructure:"secret_master_key"`   // 微信支付等敏感配置加密
	ProviderKeyMaster string `mapstructure:"provider_key_master"` // provider API key 加密
}

type PortalConfig struct {
	BaseURL string `mapstructure:"base_url"`
}

type LegalConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TermsVersion   string `mapstructure:"terms_version"`
	PrivacyVersion string `mapstructure:"privacy_version"`
}

type PricingConfig struct {
	LiteLLMURL string `mapstructure:"litellm_url"`
}

type ImageConfig struct {
	StorageDir     string        `mapstructure:"storage_dir"`
	Retention      time.Duration `mapstructure:"retention"`
	PublicBasePath string        `mapstructure:"public_base_path"`
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
	StorageDir    string        `mapstructure:"storage_dir"`
	AssetTTL      time.Duration `mapstructure:"asset_ttl"`
	URLTTL        time.Duration `mapstructure:"url_ttl"`
	PublicBaseURL string        `mapstructure:"public_base_url"`
	MaxBytes      int64         `mapstructure:"max_bytes"`
}

type AuditConfig struct {
	StoreImageBlobs bool `mapstructure:"store_image_blobs"`
}

type LogConfig struct {
	Level      string   `mapstructure:"level"`
	File       string   `mapstructure:"file"`
	MaxSize    int      `mapstructure:"max_size"`
	MaxBackups int      `mapstructure:"max_backups"`
	MaxAge     int      `mapstructure:"max_age"`
	Redact     []string `mapstructure:"redact"`
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
	v.SetDefault("server.port", 19641)
	v.SetDefault("server.read_timeout", 30)
	v.SetDefault("server.write_timeout", 0) // AI 网关需要长写超时
	v.SetDefault("server.idle_timeout", 60)

	// 默认值 —— 数据库（统一用 URL，兼容 DSN）
	v.SetDefault("database.url", "postgres://postgres:postgres@localhost:5432/dai?sslmode=disable")
	v.SetDefault("database.max_open_conns", 20)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 30)
	v.SetDefault("database.max_conns", 20)
	v.SetDefault("database.min_conns", 2)
	v.SetDefault("database.max_conn_lifetime", "1h")

	// 默认值 —— Redis
	v.SetDefault("redis.addr", "")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	// 默认值 —— JWT
	v.SetDefault("jwt.expiration", "2h")
	v.SetDefault("jwt.refresh_expiration", "168h")
	v.SetDefault("jwt.issuer", "dai")

	// 默认值 —— Security
	v.SetDefault("security.secret_master_key", "")
	v.SetDefault("security.provider_key_master", "")

	// 默认值 —— Portal
	v.SetDefault("portal.base_url", "")

	// 默认值 —— Legal
	v.SetDefault("legal.base_url", "http://localhost:19641/legal")
	v.SetDefault("legal.terms_version", "2026-07-19")
	v.SetDefault("legal.privacy_version", "2026-07-19")

	// 默认值 —— Log
	v.SetDefault("log.level", "info")
	v.SetDefault("log.file", "")
	v.SetDefault("log.max_size", 100)
	v.SetDefault("log.max_backups", 30)
	v.SetDefault("log.max_age", 30)
	v.SetDefault("log.redact", []string{})

	// 默认值 —— AI 域
	v.SetDefault("pricing.litellm_url", "")
	v.SetDefault("audit.store_image_blobs", false)
	v.SetDefault("image_assets.storage_dir", "images")
	v.SetDefault("image_assets.retention", "24h")
	v.SetDefault("image_assets.public_base_path", "/runtime/v1/images/tasks")
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
	v.SetDefault("file_store.storage_dir", "files")
	v.SetDefault("file_store.asset_ttl", "24h")
	v.SetDefault("file_store.url_ttl", "24h")
	v.SetDefault("file_store.public_base_url", "http://127.0.0.1:19641")
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
		"DAI_APP_ENV":                "app.env",
		"DAI_SERVER_ADDR":            "server.addr",
		"DAI_SERVER_PORT":            "server.port",
		"DAI_DATABASE_URL":           "database.url",
		"DAI_DATABASE_DSN":           "database.dsn",
		"DAI_DB_MAX_CONNS":           "database.max_conns",
		"DAI_DB_MIN_CONNS":           "database.min_conns",
		"DAI_DB_MAX_CONN_LIFETIME":   "database.max_conn_lifetime",
		"DAI_REDIS_ADDR":             "redis.addr",
		"DAI_REDIS_PASSWORD":         "redis.password",
		"DAI_REDIS_DB":               "redis.db",
		"DAI_JWT_EXPIRATION":         "jwt.expiration",
		"DAI_JWT_REFRESH_EXPIRATION": "jwt.refresh_expiration",
		// Security
		"DAI_SECURITY_SECRET_MASTER_KEY": "security.secret_master_key",
		"DAI_PROVIDER_KEY_MASTER":        "security.provider_key_master",
		// Portal
		"DAI_PORTAL_BASE_URL": "portal.base_url",
		// Legal
		"DAI_LEGAL_BASE_URL":        "legal.base_url",
		"DAI_LEGAL_TERMS_VERSION":   "legal.terms_version",
		"DAI_LEGAL_PRIVACY_VERSION": "legal.privacy_version",
		// Log
		"DAI_LOG_LEVEL":  "log.level",
		"DAI_LOG_FILE":   "log.file",
		"DAI_LOG_REDACT": "log.redact",
		// AI
		"DAI_PRICING_LITELLM_URL":          "pricing.litellm_url",
		"DAI_AUDIT_STORE_IMAGE_BLOBS":      "audit.store_image_blobs",
		"DAI_IMAGE_ASSET_STORAGE_DIR":      "image_assets.storage_dir",
		"DAI_IMAGE_ASSET_RETENTION":        "image_assets.retention",
		"DAI_IMAGE_ASSET_BASE_PATH":        "image_assets.public_base_path",
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
		"DAI_FILE_STORAGE_DIR":             "file_store.storage_dir",
		"DAI_FILE_ASSET_TTL":               "file_store.asset_ttl",
		"DAI_FILE_URL_TTL":                 "file_store.url_ttl",
		"DAI_PUBLIC_BASE_URL":              "file_store.public_base_url",
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
	if v := os.Getenv("DAI_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = n
		}
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
	if v := os.Getenv("DAI_PROVIDER_KEY_MASTER"); v != "" {
		cfg.Security.ProviderKeyMaster = v
	}
	if v := os.Getenv("DAI_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("DAI_PORTAL_BASE_URL"); v != "" {
		cfg.Portal.BaseURL = v
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

	// Security: 两个主密钥都可选（但生产建议都配）

	// Portal 校验
	if strings.TrimSpace(cfg.Portal.BaseURL) != "" {
		parsed, err := url.Parse(cfg.Portal.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("portal.base_url must be an absolute URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("portal.base_url must use http or https")
		}
		cfg.Portal.BaseURL = strings.TrimRight(cfg.Portal.BaseURL, "/")
	}

	// Legal 校验
	if baseURL := strings.TrimSpace(cfg.Legal.BaseURL); baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("legal.base_url must be an absolute URL")
		}
		cfg.Legal.BaseURL = strings.TrimRight(baseURL, "/")
	}

	return nil
}

// ─── Helpers ───────────────────────────────────────────

// DSN 返回最终用于 pgx 连接的 DSN（URL 优先，DSN 后备）
func (c *DatabaseConfig) DSNString() string {
	if c.URL != "" {
		return c.URL
	}
	return c.DSN
}
