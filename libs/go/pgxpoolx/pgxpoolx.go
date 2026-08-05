package pgxpoolx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	MaxConns              int32
	MinConns              int32
	MaxConnLifetime       time.Duration
	MaxConnIdleTime       time.Duration
	MaxConnLifetimeJitter time.Duration
	HealthCheckPeriod     time.Duration
}

func ConfigFromDSN(dsn string, opts Options) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	ApplyOptions(cfg, opts)
	return cfg, nil
}

func ApplyOptions(cfg *pgxpool.Config, opts Options) {
	if opts.MaxConns > 0 {
		cfg.MaxConns = opts.MaxConns
	}
	if opts.MinConns > 0 {
		cfg.MinConns = opts.MinConns
	}
	if opts.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = opts.MaxConnLifetime
	}
	if opts.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = opts.MaxConnIdleTime
	}
	if opts.MaxConnLifetimeJitter > 0 {
		cfg.MaxConnLifetimeJitter = opts.MaxConnLifetimeJitter
	}
	if opts.HealthCheckPeriod > 0 {
		cfg.HealthCheckPeriod = opts.HealthCheckPeriod
	}
}

func Open(ctx context.Context, dsn string, opts Options) (*pgxpool.Pool, error) {
	cfg, err := ConfigFromDSN(dsn, opts)
	if err != nil {
		return nil, err
	}
	return OpenWithConfig(ctx, cfg)
}

func OpenWithConfig(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

func MaskURL(rawURL string) string {
	const scheme = "postgres://"
	start := 0
	if strings.HasPrefix(rawURL, scheme) {
		start = len(scheme)
	}

	at := strings.IndexByte(rawURL[start:], '@')
	if at == -1 {
		return rawURL
	}
	at += start
	if at <= start {
		return rawURL
	}

	return rawURL[:start] + "***:***@" + rawURL[at+1:]
}
