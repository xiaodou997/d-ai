package db_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/db"
	"xiaodou/dai/internal/dbtest"
)

// 54135ad is the last pre-migration v1 baseline. The earlier 8d8f38a snapshot
// predates unversioned billing changes and cannot be the input to migration 0002.
const replayBaselineCommit = "54135ad"

func TestMigrationChainReplaysHistoricalV1IntoCanonicalSchema(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	root := repositoryRoot(t)
	baseline := gitFile(t, root, replayBaselineCommit+":internal/db/init.sql")
	// pgcrypto is database-global. dbtest has already installed it through the
	// race-safe current baseline; replaying the old plain CREATE EXTENSION line
	// concurrently with other packages can otherwise collide on the global name.
	baseline = strings.Replace(baseline, "CREATE EXTENSION IF NOT EXISTS pgcrypto;", "-- pgcrypto provisioned by dbtest canonical schema", 1)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "db", "changes"))
	if err != nil {
		t.Fatal(err)
	}
	migrations := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, filepath.Join(root, "internal", "db", "changes", entry.Name()))
		}
	}
	sort.Strings(migrations)
	if len(migrations) != db.ExpectedSchemaVersion-1 {
		t.Fatalf("found %d migrations, want %d", len(migrations), db.ExpectedSchemaVersion-1)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Release() })
	canonicalSchema := currentSchema(t, ctx, conn.Conn())
	replaySchema := fmt.Sprintf("dai_replay_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, `CREATE SCHEMA `+quoteIdentifier(replaySchema)); err != nil {
		t.Fatalf("create replay schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+quoteIdentifier(replaySchema)+` CASCADE`)
	})

	if err := execSchemaSQL(ctx, conn.Conn(), replaySchema, baseline); err != nil {
		t.Fatalf("apply historical v1 baseline: %v", err)
	}
	assertSchemaVersion(t, ctx, conn.Conn(), replaySchema, 1)

	expected := 2
	for _, path := range migrations {
		base := filepath.Base(path)
		prefix := strings.SplitN(base, "_", 2)[0]
		number, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatalf("parse migration %s: %v", base, err)
		}
		if number != expected {
			t.Fatalf("migration chain expected v%d, found %s", expected, base)
		}
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := execSchemaSQL(ctx, conn.Conn(), replaySchema, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", base, err)
		}
		assertSchemaVersion(t, ctx, conn.Conn(), replaySchema, expected)
		expected++
	}
	if expected-1 != db.ExpectedSchemaVersion {
		t.Fatalf("migration chain ended at v%d, want v%d", expected-1, db.ExpectedSchemaVersion)
	}

	canonicalSignature := schemaSignature(t, ctx, conn.Conn(), canonicalSchema)
	replaySignature := schemaSignature(t, ctx, conn.Conn(), replaySchema)
	if diff := signatureDiff(canonicalSignature, replaySignature); diff != "" {
		t.Fatal("schema-only replay differs:\n" + diff)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func gitFile(t *testing.T, root, revision string) string {
	t.Helper()
	cmd := exec.Command("git", "show", revision)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("read historical baseline %s: %v", revision, err)
	}
	return string(output)
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func execSchemaSQL(ctx context.Context, conn *pgx.Conn, schema, sql string) error {
	_, err := conn.Exec(ctx, "SET search_path TO "+quoteIdentifier(schema)+";\n"+sql)
	return err
}

func currentSchema(t *testing.T, ctx context.Context, conn *pgx.Conn) string {
	t.Helper()
	var schema string
	if err := conn.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}
	return schema
}

func assertSchemaVersion(t *testing.T, ctx context.Context, conn *pgx.Conn, schema string, want int) {
	t.Helper()
	var got int
	if err := conn.QueryRow(ctx, `SELECT version FROM `+quoteIdentifier(schema)+`.dai_schema_metadata WHERE singleton = TRUE`).Scan(&got); err != nil {
		t.Fatalf("read %s schema version: %v", schema, err)
	}
	if got != want {
		t.Fatalf("schema %s version = %d, want %d", schema, got, want)
	}
}

func schemaSignature(t *testing.T, ctx context.Context, conn *pgx.Conn, schema string) []string {
	t.Helper()
	queries := []string{
		`SELECT format('relation|%s|%s', c.relname, c.relkind)
		 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')`,
		`SELECT format('column|%s|%s|%s|%s|%s|%s|%s', table_name, column_name,
			data_type, udt_name, is_nullable,
			COALESCE(column_default, ''), COALESCE(is_identity, ''))
		 FROM information_schema.columns
		 WHERE table_schema = $1`,
		`SELECT format('constraint|%s|%s|%s|%s', c.relname, con.conname,
			con.contype, pg_get_constraintdef(con.oid, true))
		 FROM pg_constraint con
		 JOIN pg_class c ON c.oid = con.conrelid
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1`,
		`SELECT format('index|%s|%s|%s', table_class.relname, index_class.relname,
			pg_get_indexdef(index_info.indexrelid))
		 FROM pg_index index_info
		 JOIN pg_class index_class ON index_class.oid = index_info.indexrelid
		 JOIN pg_class table_class ON table_class.oid = index_info.indrelid
		 JOIN pg_namespace n ON n.oid = table_class.relnamespace
		 WHERE n.nspname = $1`,
		`SELECT format('function|%s|%s', p.proname, pg_get_functiondef(p.oid))
		 FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		 WHERE n.nspname = $1 AND COALESCE(p.probin, '') = ''`,
		`SELECT format('trigger|%s|%s|%s', c.relname, t.tgname, pg_get_triggerdef(t.oid, true))
		 FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND NOT t.tgisinternal`,
	}

	var signature []string
	for _, query := range queries {
		rows, err := conn.Query(ctx, query, schema)
		if err != nil {
			t.Fatalf("read schema signature for %s: %v", schema, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				t.Fatalf("scan schema signature for %s: %v", schema, err)
			}
			normalized := strings.ReplaceAll(value, schema, "SCHEMA_NORMALIZED")
			normalized = strings.ReplaceAll(normalized, "SCHEMA_NORMALIZED.", "")
			signature = append(signature, normalized)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate schema signature for %s: %v", schema, err)
		}
		rows.Close()
	}
	sort.Strings(signature)
	return signature
}

func signatureDiff(canonical, replay []string) string {
	replaySet := make(map[string]struct{}, len(replay))
	for _, value := range replay {
		replaySet[value] = struct{}{}
	}
	canonicalSet := make(map[string]struct{}, len(canonical))
	for _, value := range canonical {
		canonicalSet[value] = struct{}{}
	}
	var missing, extra []string
	for value := range canonicalSet {
		if _, ok := replaySet[value]; !ok {
			missing = append(missing, value)
		}
	}
	for value := range replaySet {
		if _, ok := canonicalSet[value]; !ok {
			extra = append(extra, value)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	var b strings.Builder
	for _, value := range missing {
		fmt.Fprintf(&b, "missing in replay: %s\n", value)
	}
	for _, value := range extra {
		fmt.Fprintf(&b, "extra in replay: %s\n", value)
	}
	return b.String()
}
