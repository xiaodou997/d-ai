package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func mustExecRiskControl(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql); err != nil {
		t.Fatalf("exec sql failed: %v\nsql: %s", err, sql)
	}
}

// setupRiskControlSchema creates the risk-control tables inside the
// disposable test schema (search_path already points at it via the pool
// returned by testsupport.OpenAsyncTaskTestPool). Mirrors db/init.sql's
// definitions closely enough to exercise real pgtype round-tripping.
func setupRiskControlSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	mustExecRiskControl(t, ctx, pool, `
		CREATE TABLE ai_settings (
		  key        TEXT        PRIMARY KEY,
		  value      JSONB       NOT NULL,
		  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	mustExecRiskControl(t, ctx, pool, `
		CREATE TABLE ai_content_moderation_logs (
		  id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		  request_id           TEXT,
		  tenant_id            TEXT,
		  user_id              TEXT,
		  api_key_id           UUID,
		  model_code           TEXT,
		  capability_type      TEXT,
		  mode                 TEXT        NOT NULL,
		  action               TEXT        NOT NULL,
		  flagged              BOOLEAN     NOT NULL DEFAULT false,
		  matched_keyword      TEXT,
		  highest_category     TEXT,
		  highest_score        NUMERIC(8,6),
		  category_scores      JSONB,
		  threshold_snapshot   JSONB,
		  input_excerpt        TEXT,
		  upstream_latency_ms  INTEGER,
		  error                TEXT,
		  hit_layer            TEXT,
		  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	mustExecRiskControl(t, ctx, pool, `
		CREATE TABLE ai_risk_events (
		  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		  event_type       TEXT        NOT NULL DEFAULT 'content_violation',
		  severity         TEXT        NOT NULL DEFAULT 'medium',
		  tenant_id        TEXT,
		  user_id          TEXT,
		  source_log_id    UUID,
		  summary          TEXT        NOT NULL,
		  detail           JSONB       NOT NULL DEFAULT '{}',
		  status           TEXT        NOT NULL DEFAULT 'open',
		  resolved_by      TEXT,
		  resolved_at      TIMESTAMPTZ,
		  resolution_note  TEXT,
		  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
}

func TestRiskControlRepo_SettingRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{})
	if err != nil {
		t.Skipf("open risk control test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	setupRiskControlSchema(t, ctx, pool)

	repo := NewRiskControlRepo(dbgen.New(pool))

	if _, err := repo.GetSetting(ctx, "risk_control_config"); err == nil || err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound before first write, got %v", err)
	}

	cfg := domain.RiskControlConfig{Enabled: true, Mode: domain.RiskControlModeObserve}
	raw, _ := json.Marshal(cfg)
	if err := repo.UpsertSetting(ctx, "risk_control_config", raw); err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	got, err := repo.GetSetting(ctx, "risk_control_config")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	var roundTripped domain.RiskControlConfig
	if err := json.Unmarshal(got, &roundTripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !roundTripped.Enabled || roundTripped.Mode != domain.RiskControlModeObserve {
		t.Fatalf("got %#v", roundTripped)
	}
}

func TestRiskControlRepo_LogInsertListCountFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{})
	if err != nil {
		t.Skipf("open risk control test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	setupRiskControlSchema(t, ctx, pool)

	repo := NewRiskControlRepo(dbgen.New(pool))
	score := 0.91
	scores, _ := json.Marshal(map[string]float64{"violence": 0.91})

	id, createdAt, err := repo.InsertLog(ctx, domain.ContentModerationLog{
		RequestID:       "req-1",
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ModelCode:       "gpt-test",
		CapabilityType:  "chat",
		Mode:            domain.RiskControlModePreBlock,
		Action:          domain.RiskControlActionBlock,
		Flagged:         true,
		HitLayer:        domain.HitLayerKeyword,
		HighestCategory: "violence",
		HighestScore:    &score,
		CategoryScores:  scores,
		InputExcerpt:    "some violent text",
	})
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}
	if id == "" || createdAt.IsZero() {
		t.Fatalf("expected generated id/created_at, got id=%q createdAt=%v", id, createdAt)
	}

	// A second, non-flagged log for the same user to exercise the flagged filter.
	if _, _, err := repo.InsertLog(ctx, domain.ContentModerationLog{
		TenantID: "tenant-1", UserID: "user-1", Mode: domain.RiskControlModePreBlock, Action: domain.RiskControlActionAllow,
	}); err != nil {
		t.Fatalf("insert second log: %v", err)
	}

	total, err := repo.CountLogs(ctx, domain.ContentModerationLogFilter{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}

	flagged := true
	flaggedOnly, err := repo.ListLogs(ctx, domain.ContentModerationLogFilter{TenantID: "tenant-1", Flagged: &flagged}, 10, 0)
	if err != nil {
		t.Fatalf("list flagged logs: %v", err)
	}
	if len(flaggedOnly) != 1 || flaggedOnly[0].ID != id {
		t.Fatalf("flagged logs = %#v", flaggedOnly)
	}
	if flaggedOnly[0].HighestScore == nil || *flaggedOnly[0].HighestScore != score {
		t.Fatalf("highest score = %#v, want %v", flaggedOnly[0].HighestScore, score)
	}
	if flaggedOnly[0].HighestCategory != "violence" {
		t.Fatalf("highest category = %q", flaggedOnly[0].HighestCategory)
	}
	if flaggedOnly[0].HitLayer != domain.HitLayerKeyword {
		t.Fatalf("hit layer = %q, want %q", flaggedOnly[0].HitLayer, domain.HitLayerKeyword)
	}

	keywordOnly, err := repo.ListLogs(ctx, domain.ContentModerationLogFilter{
		TenantID: "tenant-1",
		HitLayer: domain.HitLayerKeyword,
	}, 10, 0)
	if err != nil {
		t.Fatalf("list keyword-layer logs: %v", err)
	}
	if len(keywordOnly) != 1 || keywordOnly[0].ID != id {
		t.Fatalf("keyword-layer logs = %#v", keywordOnly)
	}

	count, err := repo.CountFlaggedSince(ctx, "user-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("count flagged since: %v", err)
	}
	if count != 1 {
		t.Fatalf("flagged-since count = %d, want 1", count)
	}
}

func TestRiskControlRepo_EventInsertListResolve(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{})
	if err != nil {
		t.Skipf("open risk control test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	setupRiskControlSchema(t, ctx, pool)

	repo := NewRiskControlRepo(dbgen.New(pool))
	detail, _ := json.Marshal(map[string]any{"violation_count": 3})

	id, err := repo.InsertEvent(ctx, domain.RiskEvent{
		EventType: "content_violation",
		Severity:  domain.RiskEventSeverityMedium,
		TenantID:  "tenant-1",
		UserID:    "user-1",
		Summary:   "累计违规达到阈值",
		Detail:    detail,
	})
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if id == "" {
		t.Fatal("expected generated event id")
	}

	openEvents, err := repo.ListEvents(ctx, domain.RiskEventFilter{Status: domain.RiskEventStatusOpen}, 10, 0)
	if err != nil {
		t.Fatalf("list open events: %v", err)
	}
	if len(openEvents) != 1 || openEvents[0].ID != id || openEvents[0].Status != domain.RiskEventStatusOpen {
		t.Fatalf("open events = %#v", openEvents)
	}

	resolved, err := repo.ResolveEvent(ctx, id, domain.RiskEventStatusResolved, "admin-1", "已人工核实，误报")
	if err != nil {
		t.Fatalf("resolve event: %v", err)
	}
	if resolved.Status != domain.RiskEventStatusResolved || resolved.ResolvedBy != "admin-1" || resolved.ResolvedAt == nil {
		t.Fatalf("resolved event = %#v", resolved)
	}

	openCount, err := repo.CountEvents(ctx, domain.RiskEventFilter{Status: domain.RiskEventStatusOpen})
	if err != nil {
		t.Fatalf("count open events: %v", err)
	}
	if openCount != 0 {
		t.Fatalf("open count after resolve = %d, want 0", openCount)
	}

	if _, err := repo.ResolveEvent(ctx, "00000000-0000-0000-0000-000000000000", domain.RiskEventStatusResolved, "admin-1", ""); err != domain.ErrNotFound {
		t.Fatalf("resolve missing event: err = %v, want ErrNotFound", err)
	}
}
