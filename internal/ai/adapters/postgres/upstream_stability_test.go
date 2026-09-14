package postgres

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"time"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestUpstreamStabilityCountsAttemptsOnceAcrossEarlyAndRepeatedSeal(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	ids := []string{uuid.NewString(), uuid.NewString()}
	ends := []string{uuid.NewString(), uuid.NewString()}
	for i, id := range ids {
		if _, err = pool.Exec(ctx, `INSERT INTO ai_upstream_accounts(id,name,tenant_display_name,api_key_ciphertext,status) VALUES($1,$2,$2,'test-secret','active')`, id, "stability-"+id); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO ai_upstream_account_endpoints(id,account_id,api_format,base_url) VALUES($1,$2,'openai_chat','https://example.test')`, ends[i], id); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO ai_upstream_models(upstream_kind,upstream_id,model_code,capability_type,upstream_model_name) VALUES('direct_upstream',$1,'test-model','chat','model')`, id); err != nil {
			t.Fatal(err)
		}
	}
	store := NewRequestStore(pool, fixedUsageBiller{}, nil)
	req := usageCompletionRequest(uuid.NewString(), "tenant", "user")
	req.Attempts = nil
	if err = store.Execute(ctx, req); err != nil {
		t.Fatal(err)
	}
	defer req.RecordLeaseStop()
	now := time.Now()
	req.Attempts = []serving.AttemptRecord{
		{CandidateID: "a", AccountID: ids[0], EndpointID: ends[0], ModelCode: "test-model", UpstreamModel: "model", Operation: "openai_chat", Outcome: serving.ResultServerError, OutcomeFinal: true, CompletedAt: now, TotalMs: 10, CooldownEntered: true, ErrorMsg: "provider failed"},
		{CandidateID: "b", AccountID: ids[1], EndpointID: ends[1], ModelCode: "test-model", UpstreamModel: "model", Operation: "openai_chat", Outcome: serving.ResultSuccess, CompletedAt: now, TotalMs: 400},
	}
	if err = store.RecordAttempt(ctx, req, 0); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordAttempt(ctx, req, 0); err != nil {
		t.Fatal(err)
	}
	// Early financial sealing must neither count an unfinished second attempt nor
	// stop its final result from being recorded by a later seal.
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM ai_request_attempts WHERE request_id=$1`, req.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unfinished count=%d err=%v", count, err)
	}
	req.Attempts[1].OutcomeFinal = true
	req.SkippedAttempts = []serving.AttemptRecord{{AccountID: ids[0], EndpointID: ends[0], Outcome: serving.ResultCircuitOpen, CompletedAt: now}}
	for i := 0; i < 2; i++ {
		if err = store.Log(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	for _, window := range []string{"1h", "24h", "7d"} {
		items, e := store.ListUpstreamStability(ctx, "direct_upstream", window)
		if e != nil {
			t.Fatal(e)
		}
		matched := 0
		for _, item := range items {
			switch item.ResourceID {
			case ids[0]:
				matched++
				if item.Samples != 1 || item.Failures != 1 || item.Successes != 0 || item.Excluded != 1 || item.CooldownsLastHour != 1 || item.SuccessRate == nil || *item.SuccessRate != 0 {
					t.Fatalf("A: %+v", item)
				}
			case ids[1]:
				matched++
				if item.Samples != 1 || item.Successes != 1 || *item.SuccessRate != 100 || len(item.Paths) != 1 {
					t.Fatalf("B: %+v", item)
				}
			}
		}
		if matched != 2 {
			t.Fatal(items)
		}
	}
	details, e := store.UpstreamStabilityDetails(ctx, "direct_upstream", ids[1], "24h")
	if e != nil || len(details) != 1 || details[0].P50Ms != 400 {
		t.Fatalf("details=%+v err=%v", details, e)
	}
	// HTTP 200 can still finish as an upstream stream failure. A repaired final
	// attempt replaces the provisional outcome without adding another sample.
	req.Attempts[1].Outcome = serving.ResultServerError
	req.Attempts[1].HTTPStatus = 200
	if err = store.RecordAttempt(ctx, req, 1); err != nil {
		t.Fatal(err)
	}
	details, e = store.UpstreamStabilityDetails(ctx, "direct_upstream", ids[1], "24h")
	if e != nil || len(details) != 1 || details[0].Outcome != "server_error" || details[0].Count != 1 || details[0].P50Ms != 0 {
		t.Fatalf("details=%+v err=%v", details, e)
	}
}
