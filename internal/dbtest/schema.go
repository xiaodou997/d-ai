package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	daidb "xiaodou/dai/internal/db"
)

const schemaInitLockID int64 = 2264023271298249

// EnsureCanonicalSchema initializes an empty integration-test database once.
// It is deliberately outside the application startup path.
func EnsureCanonicalSchema(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire schema init connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, schemaInitLockID); err != nil {
		return fmt.Errorf("lock schema initialization: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, schemaInitLockID)
	}()

	var initialized bool
	if err := conn.QueryRowContext(ctx, `
		SELECT to_regclass('public.dai_schema_metadata') IS NOT NULL
	`).Scan(&initialized); err != nil {
		return fmt.Errorf("inspect test schema: %w", err)
	}
	if !initialized {
		schema, err := loadCanonicalSchema()
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("initialize test schema: %w", err)
		}
		return nil
	}

	var version int
	if err := conn.QueryRowContext(ctx, `
		SELECT version FROM dai_schema_metadata WHERE singleton = TRUE
	`).Scan(&version); err != nil {
		return fmt.Errorf("read test schema version: %w", err)
	}
	if version != daidb.ExpectedSchemaVersion {
		return fmt.Errorf("test schema version %d does not match expected %d", version, daidb.ExpectedSchemaVersion)
	}
	return nil
}

func loadCanonicalSchema() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "internal", "db", "init.sql")
		raw, readErr := os.ReadFile(candidate)
		if readErr == nil {
			return string(raw), nil
		}
		if !os.IsNotExist(readErr) {
			return "", readErr
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate internal/db/init.sql above the working directory")
		}
		dir = parent
	}
}
