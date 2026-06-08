package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
	URM      URMConfig      `mapstructure:"urm"`
	Security SecurityConfig `mapstructure:"security"`
	Log      LogConfig      `mapstructure:"log"`
	Serving  ServingConfig  `mapstructure:"serving"`
	Pricing  PricingConfig  `mapstructure:"pricing"`
}

// PricingConfig holds price-book / billing tunables.
type PricingConfig struct {
	// LiteLLMURL is the source for the "import from LiteLLM" admin action.
	// Empty falls back to pricebook.DefaultLiteLLMURL.
	LiteLLMURL string `mapstructure:"litellm_url"`
}

// ServingConfig holds request-execution-layer tuning.
type ServingConfig struct {
	Timeouts ServingTimeouts `mapstructure:"timeouts"`
}

// ServingTimeouts are the global default 三段式 timeouts. They sit at the
// lowest priority of the route > model > global resolution chain — an
// ai_model_routes or ai_models row may override any individual value.
type ServingTimeouts struct {
	Connect     time.Duration `mapstructure:"connect"`      // 发出请求 → 收到响应头
	FirstByte   time.Duration `mapstructure:"first_byte"`   // 响应头 → 首个 body 字节
	Idle        time.Duration `mapstructure:"idle"`         // 流式相邻 chunk 间隔
	MaxDuration time.Duration `mapstructure:"max_duration"` // 单次响应总时长上限
}

type AppConfig struct {
	Env string `mapstructure:"env"` // development | production
}

type ServerConfig struct {
	Addr string `mapstructure:"addr"` // e.g. ":13010"
}

type PostgresConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxConns        int32         `mapstructure:"max_conns"`
	MinConns        int32         `mapstructure:"min_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
}

type URMConfig struct {
	BaseURL     string        `mapstructure:"base_url"`
	ClientID    string        `mapstructure:"client_id"`
	DisplayName string        `mapstructure:"display_name"`
	Description string        `mapstructure:"description"`
	Timeout     time.Duration `mapstructure:"timeout"`
}

type SecurityConfig struct {
	ProviderKeyMaster string `mapstructure:"provider_key_master"`
}

type LogConfig struct {
	Level      string   `mapstructure:"level"`       // debug | info | warn | error
	File       string   `mapstructure:"file"`        // 日志文件路径，空表示只输出到控制台
	MaxSize    int      `mapstructure:"max_size"`    // lumberjack max size in MB (default 100)
	MaxBackups int      `mapstructure:"max_backups"` // lumberjack max backup count (default 30)
	MaxAge     int      `mapstructure:"max_age"`     // lumberjack max age in days (default 30)
	Redact     []string `mapstructure:"redact"`      // custom redact field names; empty → built-in defaults
}

func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("app.env", "development")
	v.SetDefault("server.addr", ":13010")
	v.SetDefault("postgres.max_conns", 20)
	v.SetDefault("postgres.min_conns", 2)
	v.SetDefault("postgres.max_conn_lifetime", "1h")
	v.SetDefault("urm.timeout", "10s")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.file", "")
	v.SetDefault("serving.timeouts.connect", "10s")
	v.SetDefault("serving.timeouts.first_byte", "60s")
	v.SetDefault("serving.timeouts.idle", "60s")
	v.SetDefault("serving.timeouts.max_duration", "15m")

	// Config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	// Environment variables
	v.SetEnvPrefix("AI")
	v.AutomaticEnv()
	bindEnvs(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func bindEnvs(v *viper.Viper) {
	envBindings := map[string]string{
		"AI_APP_ENV":              "app.env",
		"AI_SERVER_ADDR":          "server.addr",
		"AI_DATABASE_URL":         "postgres.dsn",
		"AI_DB_MAX_CONNS":         "postgres.max_conns",
		"AI_DB_MIN_CONNS":         "postgres.min_conns",
		"AI_DB_MAX_CONN_LIFETIME": "postgres.max_conn_lifetime",
		"AI_REDIS_ADDR":           "redis.addr",
		"AI_REDIS_PASSWORD":       "redis.password",
		"AI_URM_BASE_URL":         "urm.base_url",
		"AI_URM_CLIENT_ID":        "urm.client_id",
		"AI_URM_DISPLAY_NAME":     "urm.display_name",
		"AI_URM_DESCRIPTION":      "urm.description",
		"AI_URM_TIMEOUT":          "urm.timeout",
		"AI_PROVIDER_KEY_MASTER":  "security.provider_key_master",
		"AI_LOG_LEVEL":            "log.level",
		"AI_LOG_FILE":             "log.file",
		"AI_LOG_REDACT":           "log.redact",
		"AI_PRICING_LITELLM_URL":  "pricing.litellm_url",
	}
	for env, key := range envBindings {
		_ = v.BindEnv(key, env)
	}
}

func validate(cfg *Config) error {
	if cfg.Postgres.DSN == "" {
		return fmt.Errorf("postgres.dsn is required")
	}
	if cfg.URM.BaseURL == "" {
		return fmt.Errorf("urm.base_url is required")
	}
	if cfg.URM.ClientID == "" {
		return fmt.Errorf("urm.client_id is required")
	}
	if cfg.Security.ProviderKeyMaster == "" {
		return fmt.Errorf("security.provider_key_master is required")
	}
	return nil
}
