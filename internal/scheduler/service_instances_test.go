package scheduler

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCleanupServiceInstancesBefore(t *testing.T) {
	dsn := os.Getenv("URM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set URM_TEST_DATABASE_URL to run this DB-backed test")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TEMP TABLE gov_service_instances (
			service_id text NOT NULL,
			instance_id text NOT NULL,
			last_seen timestamptz NOT NULL,
			PRIMARY KEY (service_id, instance_id)
		);
	`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO gov_service_instances (service_id, instance_id, last_seen)
		VALUES ('service-1', 'stale', $1), ('service-1', 'recent', $2)
	`, now.Add(-25*time.Hour), now.Add(-23*time.Hour)); err != nil {
		t.Fatal(err)
	}

	s := &Scheduler{pool: pool}
	deleted, locked, err := s.cleanupServiceInstancesBefore(ctx, now.Add(-serviceInstanceRetention))
	if err != nil {
		t.Fatal(err)
	}
	if !locked || deleted != 1 {
		t.Fatalf("locked = %t, deleted = %d", locked, deleted)
	}
	var remaining string
	if err := pool.QueryRow(ctx, `SELECT instance_id FROM gov_service_instances`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != "recent" {
		t.Fatalf("remaining instance = %q", remaining)
	}
}
