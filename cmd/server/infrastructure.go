package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/db"
)

type infrastructure struct {
	pool        *pgxpool.Pool
	billingPool *pgxpool.Pool
	redis       *redis.Client
}

// openInfrastructure owns only connectivity and schema validation. Module
// constructors stay out of this function so a future control-api or gateway
// role can choose a smaller infrastructure set without importing the whole
// composition root.
func openInfrastructure(ctx context.Context, cfg *config.Config, log *zap.Logger) (*infrastructure, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is nil")
	}
	if log == nil {
		log = zap.NewNop()
	}

	dsn := cfg.Database.DSNString()
	if dsn == "" {
		return nil, fmt.Errorf("database connection string is required")
	}

	pool, err := openPostgresPool(ctx, cfg, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	closePoolOnError := true
	billingPool := pool
	closeBillingPoolOnError := false
	defer func() {
		if closePoolOnError {
			pool.Close()
		}
		if closeBillingPoolOnError {
			billingPool.Close()
		}
	}()

	if err := db.VerifySchema(ctx, pool); err != nil {
		return nil, fmt.Errorf("verify database schema: %w", err)
	}
	log.Info("database schema verified", zap.Int("version", db.ExpectedSchemaVersion))

	billingDSN := cfg.Database.BillingDSNString()
	if billingDSN != dsn {
		billingPool, err = openPostgresPool(ctx, cfg, billingDSN)
		if err != nil {
			return nil, fmt.Errorf("connect billing postgres: %w", err)
		}
		closeBillingPoolOnError = true
		if err := db.VerifySchema(ctx, billingPool); err != nil {
			return nil, fmt.Errorf("verify billing database schema: %w", err)
		}
		log.Info("billing database schema verified", zap.Int("version", db.ExpectedSchemaVersion))
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	log.Info("redis connected", zap.String("addr", cfg.Redis.Addr))

	closePoolOnError = false
	closeBillingPoolOnError = false
	return &infrastructure{pool: pool, billingPool: billingPool, redis: redisClient}, nil
}

func openPostgresPool(ctx context.Context, cfg *config.Config, dsn string) (*pgxpool.Pool, error) {
	pgConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database DSN: %w", err)
	}
	if cfg.Database.MaxConns > 0 {
		pgConfig.MaxConns = cfg.Database.MaxConns
	}
	if cfg.Database.MinConns > 0 {
		pgConfig.MinConns = cfg.Database.MinConns
	}
	if cfg.Database.MaxConnLifetime > 0 {
		pgConfig.MaxConnLifetime = cfg.Database.MaxConnLifetime
	}
	return pgxpool.NewWithConfig(ctx, pgConfig)
}
