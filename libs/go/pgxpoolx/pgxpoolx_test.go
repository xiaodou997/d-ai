package pgxpoolx

import (
	"testing"
	"time"
)

func TestConfigFromDSNAppliesOptions(t *testing.T) {
	cfg, err := ConfigFromDSN("postgres://user:pass@localhost:5432/app", Options{
		MaxConns:              20,
		MinConns:              3,
		MaxConnLifetime:       30 * time.Minute,
		MaxConnIdleTime:       5 * time.Minute,
		MaxConnLifetimeJitter: time.Minute,
		HealthCheckPeriod:     15 * time.Second,
	})
	if err != nil {
		t.Fatalf("ConfigFromDSN returned error: %v", err)
	}

	if cfg.MaxConns != 20 || cfg.MinConns != 3 {
		t.Fatalf("pool sizes = max:%d min:%d, want max:20 min:3", cfg.MaxConns, cfg.MinConns)
	}
	if cfg.MaxConnLifetime != 30*time.Minute {
		t.Fatalf("MaxConnLifetime = %s, want 30m", cfg.MaxConnLifetime)
	}
	if cfg.MaxConnIdleTime != 5*time.Minute {
		t.Fatalf("MaxConnIdleTime = %s, want 5m", cfg.MaxConnIdleTime)
	}
	if cfg.MaxConnLifetimeJitter != time.Minute {
		t.Fatalf("MaxConnLifetimeJitter = %s, want 1m", cfg.MaxConnLifetimeJitter)
	}
	if cfg.HealthCheckPeriod != 15*time.Second {
		t.Fatalf("HealthCheckPeriod = %s, want 15s", cfg.HealthCheckPeriod)
	}
}

func TestMaskURL(t *testing.T) {
	tests := map[string]string{
		"postgres://user:pass@localhost:5432/app": "postgres://***:***@localhost:5432/app",
		"user:pass@localhost:5432/app":            "***:***@localhost:5432/app",
		"localhost:5432/app":                      "localhost:5432/app",
		"":                                        "",
	}

	for input, want := range tests {
		if got := MaskURL(input); got != want {
			t.Fatalf("MaskURL(%q) = %q, want %q", input, got, want)
		}
	}
}
