package postgres

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
	"xiaodou/dai/internal/ai/audit"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/billing/ledger"
)

// Real PriceBook -> durable settlement -> ledger, including both API-key
// and subscription meters. Stale estimates deliberately conflict with evidence.
func TestReportedUsageSettlementAcrossAllMeters(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	for _, mode := range []string{subscription.BillingSourcePayg, subscription.BillingSourceSubscription} {
		for _, tc := range []struct {
			name   string
			status domain.RequestStatus
			fields map[string]int
			charge bool
		}{
			{"failure_missing", domain.RequestFailed, nil, false},
			{"success_missing", domain.RequestSuccess, nil, false},
			{"failure_reported", domain.RequestFailed, map[string]int{"input_tokens": 9, "output_tokens": 2}, false},
			{"cancelled_reported", domain.RequestCancelled, map[string]int{"input_tokens": 9, "output_tokens": 2}, true},
			{"explicit_zero", domain.RequestSuccess, map[string]int{"input_tokens": 0, "output_tokens": 0}, false},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				id := uuid.NewString()
				tenantID, userID := "tenant-"+id, "user-"+id
				seedDirectBillingAccounts(t, ctx, pool, tenantID, userID, id)
				grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindTenant, ID: tenantID, TenantID: tenantID}, 10000)
				grantTestBalance(t, ctx, pool, ledger.Ref{Kind: ledger.KindUser, ID: userID, TenantID: tenantID}, 10000)
				req := usageCompletionRequest(id, tenantID, userID)
				groupID, keyID, subID := uuid.NewString(), uuid.NewString(), uuid.NewString()
				mustExec := func(sql string, args ...any) {
					t.Helper()
					if _, err := pool.Exec(ctx, sql, args...); err != nil {
						t.Fatal(err)
					}
				}
				mustExec(`INSERT INTO ai_groups(id,tenant_id,name,retail_price_book_id) VALUES($1::uuid,$2,$3,gen_random_uuid())`, groupID, tenantID, id)
				mustExec(`INSERT INTO ai_api_keys(id,owner_type,tenant_id,user_id,group_id,key_hash,key_ciphertext,name) VALUES($1::uuid,'user',$2,$3,$4::uuid,$1,'test','test')`, keyID, tenantID, userID, groupID)
				mustExec(`INSERT INTO ai_sub_subscriptions(id,tenant_id,user_id,plan_id,order_id,plan_name_snapshot,duration_days,total_limit_micro) VALUES($1::uuid,$2,$3,gen_random_uuid(),gen_random_uuid(),'test',30,10000)`, subID, tenantID, userID)
				req.Subject.AuthMethod = coreidentity.AuthMethodAPIKey
				req.Subject.APIKeyID = keyID
				req.Candidate.GroupID = groupID
				req.Candidate.TenantMultiplier = 0.27
				req.BillingSource = mode
				req.SubscriptionID = subID
				req.SubscriptionGroupQuotaDebitMultipliers = map[string]float64{groupID: 2}
				req.RequestStatus = tc.status
				if tc.status == domain.RequestFailed {
					req.ProviderTerminalState = domain.ProviderTerminalFailed
					req.ErrorCode = "provider_terminal_error"
				}
				req.UsageEvidence = domain.UsageEvidence{Protocol: domain.ProtocolOpenAIResponses, Fields: tc.fields, Event: "response.failed", Terminal: true}
				req.TokenUsage = domain.TokenUsage{PromptTokens: 1875203, CompletionTokens: 99}
				if tc.status == domain.RequestCancelled {
					req.CancellationOrigin = domain.CancellationClient
				}
				entry := domain.PriceBookEntry{TokenPriceTiers: []domain.TokenPriceTier{{InputPerToken: 0.00001, OutputPerToken: 0.000045}}}
				req.BillingSnapshots = map[string]domain.BillingSnapshot{req.Candidate.RouteID: {AccountEntry: entry, RetailEntry: entry, GroupDefaultUserMultiplier: 0.3, EffectiveUserMultiplier: 0.3}}
				body, _ := json.Marshal(strings.Repeat("x", 5_625_609))
				req.AuditPayload = &audit.Payload{RequestID: id, ClientProtocol: "openai_responses", CapabilityType: "chat", RequestMessages: body, ErrorCode: "provider_terminal_error", FailedStep: "execute"}
				logger := NewUsageLogger(pool, &PriceBookBiller{})
				for range 2 {
					if err := logger.Log(ctx, req); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := logger.DrainSettlements(ctx, 20); err != nil {
					t.Fatal(err)
				}
				var userCharge, tenantCharge, keyUsed, subUsed, rows, outboxRows int64
				var source, billingStatus string
				var breakdown []byte
				if err := pool.QueryRow(ctx, `SELECT user_charged,tenant_payable,token_usage_source,billing_status,billing_breakdown FROM ai_consumption_projection WHERE request_id=$1`, id).Scan(&userCharge, &tenantCharge, &source, &billingStatus, &breakdown); err != nil {
					t.Fatal(err)
				}
				if err := pool.QueryRow(ctx, `SELECT quota_used FROM ai_api_keys WHERE id=$1::uuid`, keyID).Scan(&keyUsed); err != nil {
					t.Fatal(err)
				}
				if err := pool.QueryRow(ctx, `SELECT total_used_micro FROM ai_sub_subscriptions WHERE id=$1::uuid`, subID).Scan(&subUsed); err != nil {
					t.Fatal(err)
				}
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_consumption_projection WHERE request_id=$1`, id).Scan(&rows); err != nil {
					t.Fatal(err)
				}
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM bill_settlements WHERE request_id=$1 AND state='posted'`, id).Scan(&outboxRows); err != nil {
					t.Fatal(err)
				}
				wantTenant, wantUser, wantKey, wantSub, wantOutbox := int64(0), int64(0), int64(0), int64(0), int64(0)
				if tc.charge {
					wantTenant, wantKey, wantOutbox = 49, 54, 1
					if mode == subscription.BillingSourceSubscription {
						wantSub = 360
					} else {
						wantUser = 54
					}
				}
				if tenantCharge != wantTenant || userCharge != wantUser || keyUsed != wantKey || subUsed != wantSub || rows != 1 || outboxRows != wantOutbox {
					t.Fatalf("meters tenant/user/key/sub=%d/%d/%d/%d rows/outbox=%d/%d", tenantCharge, userCharge, keyUsed, subUsed, rows, outboxRows)
				}
				if accountBalance(t, ctx, pool, tenantID) != 10000-wantTenant || accountBalance(t, ctx, pool, userID) != 10000-wantUser {
					t.Fatal("actual balances mismatch")
				}
				if tc.fields == nil && (source != "missing" || billingStatus != "void") {
					t.Fatalf("missing usage status=%s source=%s", billingStatus, source)
				}
				if tc.fields != nil && source != "upstream" {
					t.Fatalf("reported source=%s", source)
				}
				var metadata map[string]json.RawMessage
				_ = json.Unmarshal(breakdown, &metadata)
				if len(metadata["usage_evidence"]) == 0 {
					t.Fatalf("missing evidence: %s", breakdown)
				}
			})
		}
	}
}
