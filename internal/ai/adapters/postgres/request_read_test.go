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

func TestRecordSearchFiltersListsAndSummary(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO iam_tenants(tenant_id,tenant_name) VALUES ('search-t','Acme Search'),('search-other','Other')`)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO iam_accounts(user_id,tenant_id,username,password_hash,user_type,nickname) VALUES ('search-u','search-t','alice-search','hash',4,'小明')`)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_price_books(id,owner_type,status,name) VALUES ('00000000-0000-0000-0000-000000000003','platform','active','Search price book')`)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_groups(id,tenant_id,name,retail_price_book_id) VALUES ('00000000-0000-0000-0000-000000000002','search-t','Current Group','00000000-0000-0000-0000-000000000003')`)
	mustExecUsageRepo(t, ctx, pool, `INSERT INTO ai_api_keys(id,owner_type,tenant_id,user_id,group_id,key_hash,key_ciphertext,name) VALUES ('00000000-0000-0000-0000-000000000001','user','search-t','search-u','00000000-0000-0000-0000-000000000002','search-hash','cipher','Production Key')`)
	store := NewRequestStore(pool, fixedUsageBiller{result: domain.BillingResult{GroupNameSnapshot: "Premium Search"}}, nil)
	for _, id := range []string{"match", "foreign"} {
		req := usageCompletionRequest("search-"+id, "search-t", "search-u")
		req.Candidate.GroupID = "00000000-0000-0000-0000-000000000002"
		req.Subject.APIKeyID = "00000000-0000-0000-0000-000000000001"
		if id == "foreign" {
			req.Subject.TenantID = "search-other"
			req.Subject.UserID = "other"
		}
		if err := store.Log(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		query domain.RecordQuery
		want  int
	}{
		{"tenant", domain.RecordQuery{TenantName: "aCmE"}, 1},
		{"username", domain.RecordQuery{UserName: "ALICE"}, 1},
		{"nickname", domain.RecordQuery{UserName: "小明"}, 1},
		{"key tenant correlation", domain.RecordQuery{APIKeyName: "production"}, 1},
		{"current group name", domain.RecordQuery{Group: "Current"}, 1},
		{"group ID", domain.RecordQuery{Group: "00000000-0000-0000-0000-000000000002"}, 2},
		{"group snapshot", domain.RecordQuery{Group: "premium"}, 2},
		{"literal wildcard", domain.RecordQuery{APIKeyName: "%"}, 0},
		{"combined", domain.RecordQuery{TenantName: "Acme", UserName: "alice", Group: "Premium", APIKeyName: "Key"}, 1},
		{"no match", domain.RecordQuery{UserName: "missing"}, 0},
		{"scoped", domain.RecordQuery{RecordScope: domain.RecordScope{TenantID: "search-other"}, UserName: "alice"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.query
			q.Kind = "requests"
			page, err := store.Records(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			total, err := store.RecordSummary(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Records) != tc.want || total.Requests != int64(tc.want) {
				t.Fatalf("list=%+v summary=%+v want=%d", page, total, tc.want)
			}
		})
	}
}
