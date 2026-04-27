package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig      `yaml:"app"`
	Postgres PostgresConfig `yaml:"postgres"`
	Redis    RedisConfig    `yaml:"redis"`
	URM      URMConfig      `yaml:"urm"`
	Security SecurityConfig `yaml:"security"`
}

type AppConfig struct {
	Env      string `yaml:"env"`
	HTTPAddr string `yaml:"httpAddr"`
	LogLevel string `yaml:"logLevel"`
}

type PostgresConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxConns        int32         `yaml:"maxConns"`
	MinConns        int32         `yaml:"minConns"`
	MaxConnLifetime time.Duration `yaml:"maxConnLifetime"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	Enabled  bool   `yaml:"enabled"`
}

type URMConfig struct {
	BaseURL   string        `yaml:"baseUrl"`
	AppKey    string        `yaml:"appKey"`
	AppSecret string        `yaml:"appSecret"`
	Timeout   time.Duration `yaml:"timeout"`
}

type SecurityConfig struct {
	ProviderKeyMaster string `yaml:"providerKeyMaster"`
	AdminToken        string `yaml:"adminToken"`
}

func Load() (*Config, error) {
	cfg := defaultConfig()

	configPath := os.Getenv("UNI_AI_API_CONFIG")
	if configPath != "" {
		if err := loadYAML(configPath, cfg); err != nil {
			return nil, err
		}
	}

	applyEnv(cfg)

	if cfg.Postgres.DSN == "" {
		return nil, errors.New("postgres dsn is required")
	}
	if cfg.Security.ProviderKeyMaster == "" {
		return nil, errors.New("provider key master is required")
	}

	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Env:      "development",
			HTTPAddr: ":13010",
			LogLevel: "info",
		},
		Postgres: PostgresConfig{
			MaxConns:        10,
			MinConns:        1,
			MaxConnLifetime: time.Hour,
		},
		Redis: RedisConfig{
			Enabled: true,
			Addr:    "127.0.0.1:6379",
		},
		URM: URMConfig{
			Timeout: 10 * time.Second,
		},
	}
}

func loadYAML(path string, cfg *Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}
	return nil
}

func applyEnv(cfg *Config) {
	setString(&cfg.App.Env, "UNI_AI_API_ENV")
	setString(&cfg.App.HTTPAddr, "UNI_AI_API_HTTP_ADDR")
	setString(&cfg.App.LogLevel, "UNI_AI_API_LOG_LEVEL")

	setString(&cfg.Postgres.DSN, "UNI_AI_API_POSTGRES_DSN")
	setInt32(&cfg.Postgres.MaxConns, "UNI_AI_API_POSTGRES_MAX_CONNS")
	setInt32(&cfg.Postgres.MinConns, "UNI_AI_API_POSTGRES_MIN_CONNS")
	setDuration(&cfg.Postgres.MaxConnLifetime, "UNI_AI_API_POSTGRES_MAX_CONN_LIFETIME")

	setBool(&cfg.Redis.Enabled, "UNI_AI_API_REDIS_ENABLED")
	setString(&cfg.Redis.Addr, "UNI_AI_API_REDIS_ADDR")
	setString(&cfg.Redis.Password, "UNI_AI_API_REDIS_PASSWORD")
	setInt(&cfg.Redis.DB, "UNI_AI_API_REDIS_DB")

	setString(&cfg.URM.BaseURL, "UNI_AI_API_URM_BASE_URL")
	setString(&cfg.URM.AppKey, "UNI_AI_API_URM_APP_KEY")
	setString(&cfg.URM.AppSecret, "UNI_AI_API_URM_APP_SECRET")
	setDuration(&cfg.URM.Timeout, "UNI_AI_API_URM_TIMEOUT")

	setString(&cfg.Security.ProviderKeyMaster, "UNI_AI_API_PROVIDER_KEY_MASTER")
	setString(&cfg.Security.AdminToken, "UNI_AI_API_ADMIN_TOKEN")
}

func setString(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

func setInt(target *int, key string) {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			*target = parsed
		}
	}
}

func setInt32(target *int32, key string) {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 32)
		if err == nil {
			*target = int32(parsed)
		}
	}
}

func setBool(target *bool, key string) {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			*target = parsed
		}
	}
}

func setDuration(target *time.Duration, key string) {
	if v := os.Getenv(key); v != "" {
		parsed, err := time.ParseDuration(v)
		if err == nil {
			*target = parsed
		}
	}
}
