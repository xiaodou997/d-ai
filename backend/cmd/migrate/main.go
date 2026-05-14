package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"uni-ai-api/backend/internal/config"
	"uni-ai-api/backend/internal/db"
)

const migrationsDir = "migrations"

type migrationFile struct {
	version string
	path    string
	upSQL   string
	downSQL string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate up|down")
		os.Exit(2)
	}

	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		fmt.Fprintln(os.Stderr, "usage: migrate up|down")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer pg.Close()

	migrations, err := loadMigrations()
	if err != nil {
		slog.Error("load migrations failed", "error", err)
		os.Exit(1)
	}

	if err := ensureMigrationsTable(ctx, pg); err != nil {
		slog.Error("ensure migrations table failed", "error", err)
		os.Exit(1)
	}

	switch direction {
	case "up":
		err = applyUp(ctx, pg, migrations)
	case "down":
		err = applyDown(ctx, pg, migrations)
	}
	if err != nil {
		slog.Error("execute migration failed", "direction", direction, "error", err)
		os.Exit(1)
	}

	slog.Info("migration completed", "direction", direction)
}

func loadMigrations() ([]migrationFile, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	migrations := make([]migrationFile, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		path := filepath.Join(migrationsDir, entry.Name())
		version := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if version == "" {
			return nil, fmt.Errorf("empty migration version for %s", entry.Name())
		}
		if _, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %s", version)
		}
		seen[version] = struct{}{}

		upSQL, downSQL, err := parseMigrationFile(path)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, migrationFile{
			version: version,
			path:    path,
			upSQL:   upSQL,
			downSQL: downSQL,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	if len(migrations) == 0 {
		return nil, errors.New("no migration sql files found")
	}

	return migrations, nil
}

func parseMigrationFile(path string) (string, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read migration file %s: %w", path, err)
	}

	text := string(content)
	upMarker := "-- +goose Up"
	downMarker := "-- +goose Down"
	upIndex := strings.Index(text, upMarker)
	downIndex := strings.Index(text, downMarker)
	if upIndex < 0 || downIndex < 0 || downIndex <= upIndex {
		return "", "", fmt.Errorf("migration file %s must contain goose up and down markers", path)
	}

	return strings.TrimSpace(text[upIndex+len(upMarker) : downIndex]),
		strings.TrimSpace(text[downIndex+len(downMarker):]),
		nil
}

func ensureMigrationsTable(ctx context.Context, pg *pgxpool.Pool) error {
	_, err := pg.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`)
	return err
}

func listAppliedVersions(ctx context.Context, pg *pgxpool.Pool) ([]string, error) {
	rows, err := pg.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make([]string, 0)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

func applyUp(ctx context.Context, pg *pgxpool.Pool, migrations []migrationFile) error {
	appliedVersions, err := listAppliedVersions(ctx, pg)
	if err != nil {
		return err
	}

	applied := make(map[string]struct{}, len(appliedVersions))
	for _, version := range appliedVersions {
		applied[version] = struct{}{}
	}

	for _, migration := range migrations {
		if _, ok := applied[migration.version]; ok {
			continue
		}
		if err := applySingleUp(ctx, pg, migration); err != nil {
			return err
		}
		slog.Info("applied migration", "version", migration.version, "path", migration.path)
	}

	return nil
}

func applySingleUp(ctx context.Context, pg *pgxpool.Pool, migration migrationFile) error {
	tx, err := pg.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, migration.upSQL); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.version, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, migration.version); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.version, err)
	}

	return tx.Commit(ctx)
}

func applyDown(ctx context.Context, pg *pgxpool.Pool, migrations []migrationFile) error {
	appliedVersions, err := listAppliedVersions(ctx, pg)
	if err != nil {
		return err
	}
	if len(appliedVersions) == 0 {
		slog.Info("no applied migrations to roll back")
		return nil
	}

	latest := appliedVersions[len(appliedVersions)-1]
	migrationByVersion := make(map[string]migrationFile, len(migrations))
	for _, migration := range migrations {
		migrationByVersion[migration.version] = migration
	}

	migration, ok := migrationByVersion[latest]
	if !ok {
		return fmt.Errorf("applied migration %s not found in %s", latest, migrationsDir)
	}

	tx, err := pg.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if migration.downSQL != "" {
		if _, err := tx.Exec(ctx, migration.downSQL); err != nil {
			return fmt.Errorf("rollback migration %s: %w", migration.version, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, migration.version); err != nil {
		return fmt.Errorf("delete migration record %s: %w", migration.version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	slog.Info("rolled back migration", "version", migration.version, "path", migration.path)
	return nil
}
