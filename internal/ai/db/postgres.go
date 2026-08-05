package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/config"
	sharedpgxpool "xiaodou/dai/libs/go/pgxpoolx"
)

func Open(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	dsn := cfg.DSNString()
	return sharedpgxpool.Open(ctx, dsn, sharedpgxpool.Options{
		MaxConns:        cfg.MaxConns,
		MinConns:        cfg.MinConns,
		MaxConnLifetime: cfg.MaxConnLifetime,
	})
}
