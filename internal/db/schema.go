package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const ExpectedSchemaVersion = 16

// VerifySchema checks the database contract without modifying it.
func VerifySchema(ctx context.Context, pool *pgxpool.Pool) error {
	var version int
	if err := pool.QueryRow(ctx, `
		SELECT version
		FROM dai_schema_metadata
		WHERE singleton = TRUE
	`).Scan(&version); err != nil {
		return fmt.Errorf("database schema is not initialized; apply internal/db/init.sql manually: %w", err)
	}
	if version != ExpectedSchemaVersion {
		return fmt.Errorf("database schema version %d is incompatible; expected %d", version, ExpectedSchemaVersion)
	}
	return nil
}
