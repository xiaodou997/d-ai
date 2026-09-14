package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestRequestRecordsSeparateErrorsAndScopeFinancialEvidence(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{CatalogBaseMicro: 50, TenantPayableMicro: 100, UserChargedMicro: 200}}, nil)
	for _, id := range []string{"normal", "disconnect", "error", "foreign"} {
		req := usageCompletionRequest(id, "t", "u")
		req.InternalErrorDetail = "private upstream credential diagnostics"
		if id == "disconnect" {
			req.CancellationOrigin = domain.CancellationClient
			req.ErrorCode = "client_disconnected"
		}
		if id == "error" {
			req.ProviderTerminalState = domain.ProviderTerminalFailed
			req.ErrorCode = "upstream_error"
			req.ErrorMessage = "上游报错"
		}
		if id == "foreign" {
			req.Subject.TenantID = "other"
			req.Subject.UserID = "other"
		}
		if err = store.Log(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	mustExecUsageRepo(t, ctx, pool, `UPDATE bill_settlements SET last_error='private settlement diagnostic' WHERE request_id='normal'`)
	adminRecord, e := store.Record(ctx, domain.RecordScope{Admin: true}, "normal")
	if e != nil || adminRecord.Charge.ProcessingDetail == "" {
		t.Fatalf("missing admin settlement diagnostic: %+v %v", adminRecord, e)
	}
	scope := domain.RecordScope{TenantID: "t", UserID: "u", EndUser: true}
	page, err := store.Records(ctx, domain.RecordQuery{RecordScope: scope, Kind: "requests", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.NextCursor == "" {
		t.Fatalf("cursor page=%+v", page)
	}
	next, err := store.Records(ctx, domain.RecordQuery{RecordScope: scope, Kind: "requests", Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Records) != 1 || next.Records[0].RequestID == page.Records[0].RequestID || next.NextCursor != "" {
		t.Fatalf("unstable cursor: %+v", next)
	}
	for _, r := range append(page.Records, next.Records...) {
		if r.IsError || r.Charge.TenantCharged != nil || r.Charge.ReferenceCost != nil || len(r.Pricing) > 0 || r.InternalDetail != "" || r.Charge.ProcessingDetail != "" {
			t.Fatalf("role projection leak: %+v", r)
		}
		if r.Charge.UserCharged != 0 || r.Charge.State != "pending" {
			t.Fatal("pending amount displayed as charged")
		}
	}
	failures, err := store.Records(ctx, domain.RecordQuery{RecordScope: scope, Kind: "errors"})
	if err != nil {
		t.Fatal(err)
	}
	if len(failures.Records) != 1 || failures.Records[0].RequestID != "error" || failures.Records[0].Charge.State != "waived" {
		t.Fatalf("error classification: %+v", failures)
	}
	if _, err = store.Record(ctx, scope, "foreign"); err != domain.ErrNotFound {
		t.Fatalf("foreign detail allowed: %v", err)
	}
	raw, _ := json.Marshal(failures)
	var shape map[string]any
	_ = json.Unmarshal(raw, &shape)
	if len(shape) == 0 {
		t.Fatal("invalid response")
	}
	summary, err := store.RecordSummary(ctx, domain.RecordQuery{RecordScope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Requests != 3 || summary.Errors != 1 || summary.Interruptions != 1 || summary.UserCharged != 0 {
		t.Fatalf("summary=%+v", summary)
	}
	if _, err = store.Records(ctx, domain.RecordQuery{Kind: "errors", Cursor: page.NextCursor}); err != domain.ErrInvalidRecordCursor {
		t.Fatalf("cross-list cursor accepted: %v", err)
	}
}
