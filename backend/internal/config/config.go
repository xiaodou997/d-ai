package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SourcePath string         `yaml:"-"`
	App        AppConfig      `yaml:"app"`
	Server     ServerConfig   `yaml:"server"`
	Logging    LoggingConfig  `yaml:"logging"`
	Postgres   PostgresConfig `yaml:"postgres"`
	Redis      RedisConfig    `yaml:"redis"`
	URM        URMConfig      `yaml:"urm"`
	Security   SecurityConfig `yaml:"security"`
}

type AppConfig struct {
	Env         string `yaml:"env"`
	ServiceName string `yaml:"serviceName"`
	Version     string `yaml:"version"`
}

type ServerConfig struct {
	HTTPAddr string `yaml:"httpAddr"`
}

type LoggingConfig struct {
	Level         string `yaml:"level"`
	Format        string `yaml:"format"`
	AccessLog     bool   `yaml:"accessLog"`
	SlowRequestMs int64  `yaml:"slowRequestMs"`
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
	BaseURL             string        `yaml:"baseUrl"`
	AppKey              string        `yaml:"appKey"`
	AppSecret           string        `yaml:"appSecret"`
	Timeout             time.Duration `yaml:"timeout"`
	JWKSRefreshInterval time.Duration `yaml:"jwksRefreshInterval"`
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
		cfg.SourcePath = configPath
	}

	applyEnv(cfg)
	normalize(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Env:         "development",
			ServiceName: "uni-ai-api",
			Version:     "dev",
		},
		Server: ServerConfig{
			HTTPAddr: ":13010",
		},
		Logging: LoggingConfig{
			Level:         "info",
			Format:        "json",
			AccessLog:     true,
			SlowRequestMs: 1000,
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
			Timeout:             10 * time.Second,
			JWKSRefreshInterval: 24 * time.Hour,
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
	setString(&cfg.App.ServiceName, "UNI_AI_API_SERVICE_NAME")
	setString(&cfg.App.Version, "UNI_AI_API_VERSION")

	setString(&cfg.Server.HTTPAddr, "UNI_AI_API_SERVER_HTTP_ADDR")

	setString(&cfg.Logging.Level, "UNI_AI_API_LOGGING_LEVEL")
	setString(&cfg.Logging.Format, "UNI_AI_API_LOGGING_FORMAT")
	setBool(&cfg.Logging.AccessLog, "UNI_AI_API_LOGGING_ACCESS_LOG")
	setInt64(&cfg.Logging.SlowRequestMs, "UNI_AI_API_LOGGING_SLOW_REQUEST_MS")

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
	setDuration(&cfg.URM.JWKSRefreshInterval, "UNI_AI_API_URM_JWKS_REFRESH_INTERVAL")

	setString(&cfg.Security.ProviderKeyMaster, "UNI_AI_API_PROVIDER_KEY_MASTER")
	setString(&cfg.Security.AdminToken, "UNI_AI_API_ADMIN_TOKEN")
}

func normalize(cfg *Config) {
	cfg.App.Env = strings.ToLower(strings.TrimSpace(cfg.App.Env))
	cfg.App.ServiceName = strings.TrimSpace(cfg.App.ServiceName)
	cfg.App.Version = strings.TrimSpace(cfg.App.Version)
	cfg.Server.HTTPAddr = strings.TrimSpace(cfg.Server.HTTPAddr)
	cfg.Logging.Level = strings.ToLower(strings.TrimSpace(cfg.Logging.Level))
	cfg.Logging.Format = strings.ToLower(strings.TrimSpace(cfg.Logging.Format))
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
}

func validate(cfg *Config) error {
	if cfg.App.ServiceName == "" {
		return errors.New("app serviceName is required")
	}
	if cfg.Server.HTTPAddr == "" {
		return errors.New("server httpAddr is required")
	}
	switch cfg.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid logging level %q", cfg.Logging.Level)
	}
	switch cfg.Logging.Format {
	case "json", "console":
	default:
		return fmt.Errorf("invalid logging format %q", cfg.Logging.Format)
	}
	if cfg.Postgres.DSN == "" {
		return errors.New("postgres dsn is required")
	}
	if cfg.Security.ProviderKeyMaster == "" {
		return errors.New("provider key master is required")
	}
	return nil
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

func setInt64(target *int64, key string) {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
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
