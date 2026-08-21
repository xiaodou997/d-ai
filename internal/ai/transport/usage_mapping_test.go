package transport

import (
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func TestUserUsageLogToDTOPreservesDomainProjection(t *testing.T) {
	t.Parallel()

	httpStatus := int32(200)
	latencyMs := int32(420)
	firstTokenLatencyMs := int32(80)
	createdAt := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

	got := userUsageLogToDTO(domain.UsageLog{
		ID:                              "log-1",
		RequestID:                       "request-1",
		TraceID:                         "trace-1",
		TenantID:                        "tenant-1",
		UserID:                          "user-1",
		RequestSource:                   "workspace",
		GroupID:                         "group-1",
		GroupNameSnapshot:               "default",
		EffectiveUserMultiplierSnapshot: 1.25,
		BillingGroupLabelSnapshot:       "Default",
		ModelCode:                       "gpt-test",
		Stream:                          true,
		PromptTokens:                    11,
		CompletionTokens:                13,
		CacheWriteTokens:                17,
		CacheReadTokens:                 19,
		ReasoningTokens:                 23,
		ReasoningEffort:                 "high",
		TotalTokens:                     83,
		BillableUnitType:                "token",
		BillableUnits:                   83,
		UserChargedMicro:                3_100_000,
		ServiceTier:                     "fast",
		BillingStatus:                   "settled",
		RefundStatus:                    "none",
		BillingSource:                   "payg",
		RequestStatus:                   "success",
		HTTPStatus:                      &httpStatus,
		LatencyMs:                       &latencyMs,
		FirstTokenLatencyMs:             &firstTokenLatencyMs,
		ErrorCode:                       "code",
		ErrorMessage:                    "message",
		CreatedAt:                       createdAt,
	})

	if got.ID != "log-1" || got.RequestID != "request-1" || got.TraceID == nil || *got.TraceID != "trace-1" {
		t.Fatalf("identity fields = %+v", got)
	}
	if got.TenantID != "tenant-1" || got.UserID == nil || *got.UserID != "user-1" || got.RequestSource != "workspace" || got.GroupID != "group-1" {
		t.Fatalf("scope fields = %+v", got)
	}
	if !got.Stream || got.CacheWriteTokens != 17 || got.CacheReadTokens != 19 || got.ReasoningTokens != 23 || got.ReasoningEffort == nil || *got.ReasoningEffort != "high" {
		t.Fatalf("token fields = %+v", got)
	}
	if got.UserChargedUSD != 3.1 || got.BillingStatus != "settled" || got.RefundStatus != "none" || got.BillingSource != "payg" {
		t.Fatalf("billing fields = %+v", got)
	}
	if got.HTTPStatus != &httpStatus || got.LatencyMs != &latencyMs || got.FirstTokenLatencyMs != &firstTokenLatencyMs {
		t.Fatalf("latency fields = %+v", got)
	}
	if got.ErrorCode == nil || *got.ErrorCode != "code" || got.ErrorMessage == nil || *got.ErrorMessage != "message" {
		t.Fatalf("error fields = %+v", got)
	}
	if got.CreatedAt == nil || *got.CreatedAt != createdAt.UnixMilli() {
		t.Fatalf("created_at = %v", got.CreatedAt)
	}
}
