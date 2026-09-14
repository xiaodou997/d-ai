package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
)

func mustExecUsageRepo(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec sql failed: %v", err)
	}
}

func TestUsageLogFromUserRowPreservesSelfServiceProjection(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC)
	var id pgtype.UUID
	if err := id.Scan("10000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatalf("scan id: %v", err)
	}
	var groupID pgtype.UUID
	if err := groupID.Scan("20000000-0000-0000-0000-000000000002"); err != nil {
		t.Fatalf("scan group id: %v", err)
	}
	var multiplier pgtype.Numeric
	if err := multiplier.Scan("1.25"); err != nil {
		t.Fatalf("scan multiplier: %v", err)
	}

	got := usageLogFromUserRow(dbgen.ListUsageLogsByTenantUserRow{
		ID:                              id,
		RequestID:                       "request-1",
		TraceID:                         pgtype.Text{String: "trace-1", Valid: true},
		TenantID:                        "tenant-1",
		UserID:                          pgtype.Text{String: "user-1", Valid: true},
		RequestSource:                   "workspace",
		GroupID:                         groupID,
		GroupNameSnapshot:               "default",
		EffectiveUserMultiplierSnapshot: multiplier,
		BillingGroupLabelSnapshot:       "Default",
		ModelCode:                       "gpt-test",
		Stream:                          true,
		PromptTokens:                    11,
		CompletionTokens:                13,
		CacheWriteTokens:                17,
		CacheReadTokens:                 19,
		ReasoningTokens:                 23,
		ReasoningEffort:                 pgtype.Text{String: "high", Valid: true},
		TotalTokens:                     83,
		BillableUnitType:                "token",
		BillableUnits:                   83,
		UserPayable:                     2900,
		UserCharged:                     3100,
		ServiceTier:                     "fast",
		BillingStatus:                   "settled",
		RefundStatus:                    "none",
		BillingSource:                   "payg",
		RequestStatus:                   "success",
		HttpStatus:                      pgtype.Int4{Int32: 200, Valid: true},
		LatencyMs:                       pgtype.Int4{Int32: 400, Valid: true},
		FirstTokenLatencyMs:             pgtype.Int4{Int32: 75, Valid: true},
		ErrorCode:                       pgtype.Text{String: "code", Valid: true},
		ErrorMessage:                    pgtype.Text{String: "message", Valid: true},
		CreatedAt:                       pgtype.Timestamptz{Time: createdAt, Valid: true},
	})

	if got.ID != "10000000-0000-0000-0000-000000000001" || got.GroupID != "20000000-0000-0000-0000-000000000002" {
		t.Fatalf("ids = %q %q", got.ID, got.GroupID)
	}
	if got.RequestID != "request-1" || got.TraceID != "trace-1" || got.TenantID != "tenant-1" || got.UserID != "user-1" || got.RequestSource != "workspace" {
		t.Fatalf("identity projection = %+v", got)
	}
	if got.EffectiveUserMultiplierSnapshot != 1.25 || got.UserPayableMicro != 2900 || got.UserChargedMicro != 3100 {
		t.Fatalf("billing projection = %+v", got)
	}
	if !got.Stream || got.CacheWriteTokens != 17 || got.CacheReadTokens != 19 || got.ReasoningTokens != 23 || got.ReasoningEffort != "high" {
		t.Fatalf("token projection = %+v", got)
	}
	if got.HTTPStatus == nil || *got.HTTPStatus != 200 || got.LatencyMs == nil || *got.LatencyMs != 400 || got.FirstTokenLatencyMs == nil || *got.FirstTokenLatencyMs != 75 {
		t.Fatalf("latency projection = %+v", got)
	}
	if got.ErrorCode != "code" || got.ErrorMessage != "message" || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("result projection = %+v", got)
	}
}
