package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v2"
)

type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	URM      URMConfig
	Security SecurityConfig
}

type ServerConfig struct {
	Addr string // e.g. ":13010"
}

type PostgresConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
}

type URMConfig struct {
	BaseURL     string
	ClientID    string
	DisplayName string
	Description string
	Timeout     time.Duration
}

type SecurityConfig struct {
	ProviderKeyMaster string
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Addr: ":13010",
		},
		Postgres: PostgresConfig{
			MaxConns:        20,
			MinConns:        2,
			MaxConnLifetime: time.Hour,
		},
		URM: URMConfig{
			Timeout: 10 * time.Second,
		},
	}

	if err := applyYAML(cfg, "config.yaml"); err != nil {
		return nil, err
	}
	applyEnv(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

type yamlFile struct {
	Server struct {
		Port int    `yaml:"port"`
		Addr string `yaml:"addr"`
	} `yaml:"server"`
	Database struct {
		DSN             string `yaml:"dsn"`
		MaxConns        int32  `yaml:"max_conns"`
		MinConns        int32  `yaml:"min_conns"`
		MaxConnLifetime string `yaml:"max_conn_lifetime"`
	} `yaml:"database"`
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
	} `yaml:"redis"`
	URM struct {
		BaseURL     string `yaml:"base_url"`
		ClientID    string `yaml:"client_id"`
		DisplayName string `yaml:"display_name"`
		Description string `yaml:"description"`
		Timeout     string `yaml:"timeout"`
	} `yaml:"urm"`
	Security struct {
		ProviderKeyMaster string `yaml:"provider_key_master"`
	} `yaml:"security"`
}

func applyYAML(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var f yamlFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return err
	}

	if f.Server.Addr != "" {
		cfg.Server.Addr = f.Server.Addr
	} else if f.Server.Port != 0 {
		cfg.Server.Addr = ":" + strconv.Itoa(f.Server.Port)
	}
	if f.Database.DSN != "" {
		cfg.Postgres.DSN = f.Database.DSN
	}
	if f.Database.MaxConns != 0 {
		cfg.Postgres.MaxConns = f.Database.MaxConns
	}
	if f.Database.MinConns != 0 {
		cfg.Postgres.MinConns = f.Database.MinConns
	}
	if f.Database.MaxConnLifetime != "" {
		if d, err := time.ParseDuration(f.Database.MaxConnLifetime); err == nil {
			cfg.Postgres.MaxConnLifetime = d
		}
	}
	if f.Redis.Addr != "" {
		cfg.Redis.Addr = f.Redis.Addr
	}
	if f.Redis.Password != "" {
		cfg.Redis.Password = f.Redis.Password
	}
	if f.URM.BaseURL != "" {
		cfg.URM.BaseURL = f.URM.BaseURL
	}
	if f.URM.ClientID != "" {
		cfg.URM.ClientID = f.URM.ClientID
	}
	if f.URM.DisplayName != "" {
		cfg.URM.DisplayName = f.URM.DisplayName
	}
	if f.URM.Description != "" {
		cfg.URM.Description = f.URM.Description
	}
	if f.URM.Timeout != "" {
		if d, err := time.ParseDuration(f.URM.Timeout); err == nil {
			cfg.URM.Timeout = d
		}
	}
	if f.Security.ProviderKeyMaster != "" {
		cfg.Security.ProviderKeyMaster = f.Security.ProviderKeyMaster
	}
	return nil
}

func applyEnv(cfg *Config) {
	setStr(&cfg.Server.Addr, "PORT", func(v string) string {
		if !strings.HasPrefix(v, ":") {
			return ":" + v
		}
		return v
	})
	setStr(&cfg.Server.Addr, "SERVER_ADDR")

	setStr(&cfg.Postgres.DSN, "DATABASE_URL")
	setI32(&cfg.Postgres.MaxConns, "DB_MAX_CONNS")
	setI32(&cfg.Postgres.MinConns, "DB_MIN_CONNS")
	setDur(&cfg.Postgres.MaxConnLifetime, "DB_MAX_CONN_LIFETIME")

	setStr(&cfg.Redis.Addr, "REDIS_ADDR")
	setStr(&cfg.Redis.Password, "REDIS_PASSWORD")

	setStr(&cfg.URM.BaseURL, "URM_BASE_URL")
	setStr(&cfg.URM.ClientID, "URM_CLIENT_ID")
	setStr(&cfg.URM.DisplayName, "URM_DISPLAY_NAME")
	setStr(&cfg.URM.Description, "URM_DESCRIPTION")
	setDur(&cfg.URM.Timeout, "URM_TIMEOUT")

	setStr(&cfg.Security.ProviderKeyMaster, "PROVIDER_KEY_MASTER")
}

func validate(cfg *Config) error {
	if cfg.Postgres.DSN == "" {
		return errors.New("DATABASE_URL is required")
	}
	if cfg.URM.BaseURL == "" {
		return errors.New("URM_BASE_URL is required")
	}
	if cfg.URM.ClientID == "" {
		return errors.New("URM_CLIENT_ID is required")
	}
	if cfg.Security.ProviderKeyMaster == "" {
		return errors.New("PROVIDER_KEY_MASTER is required")
	}
	return nil
}

func setStr(target *string, key string, transform ...func(string) string) {
	v := os.Getenv(key)
	if v == "" {
		return
	}
	if len(transform) > 0 {
		v = transform[0](v)
	}
	*target = v
}

func setI32(target *int32, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			*target = int32(n)
		}
	}
}

func setDur(target *time.Duration, key string) {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*target = d
		}
	}
}
