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
	Level      string `mapstructure:"level"`       // debug | info | warn | error
	File       string `mapstructure:"file"`        // 日志文件路径，空表示只输出到控制台
	MaxSize    int    `mapstructure:"max_size"`    // lumberjack max size in MB (default 100)
	MaxBackups int    `mapstructure:"max_backups"` // lumberjack max backup count (default 30)
	MaxAge     int    `mapstructure:"max_age"`     // lumberjack max age in days (default 30)
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
		"AI_APP_ENV":                "app.env",
		"AI_SERVER_ADDR":            "server.addr",
		"AI_DATABASE_URL":           "postgres.dsn",
		"AI_DB_MAX_CONNS":           "postgres.max_conns",
		"AI_DB_MIN_CONNS":           "postgres.min_conns",
		"AI_DB_MAX_CONN_LIFETIME":   "postgres.max_conn_lifetime",
		"AI_REDIS_ADDR":             "redis.addr",
		"AI_REDIS_PASSWORD":         "redis.password",
		"AI_URM_BASE_URL":           "urm.base_url",
		"AI_URM_CLIENT_ID":          "urm.client_id",
		"AI_URM_DISPLAY_NAME":       "urm.display_name",
		"AI_URM_DESCRIPTION":        "urm.description",
		"AI_URM_TIMEOUT":            "urm.timeout",
		"AI_PROVIDER_KEY_MASTER":    "security.provider_key_master",
		"AI_LOG_LEVEL":              "log.level",
		"AI_LOG_FILE":               "log.file",
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
