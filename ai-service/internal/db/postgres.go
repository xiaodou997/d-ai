package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/unihub/ai-service/internal/config"
	sharedpgxpool "xiaodou/unihub/shared/pgxpoolx"
)

func Open(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	return sharedpgxpool.Open(ctx, cfg.DSN, sharedpgxpool.Options{
		MaxConns:        cfg.MaxConns,
		MinConns:        cfg.MinConns,
		MaxConnLifetime: cfg.MaxConnLifetime,
	})
}
