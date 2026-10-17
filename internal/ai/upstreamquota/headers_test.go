package upstreamquota

import (
	"net/http"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func TestQuotaHeadersPreserveWindowDurationsAndUnknownValues(t *testing.T) {
	now := time.Unix(1800000000, 0)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-primary-reset-at", "1800003600")
	headers.Set("x-codex-secondary-used-percent", "25")
	headers.Set("x-codex-secondary-window-minutes", "300")
	q := FromHeaders(domain.FixedProviderCodex, "model", headers, now)
	if q == nil || q.Model != "" || len(q.Windows) != 2 || *q.Windows[0].WindowMinutes != 10080 || *q.Windows[1].WindowMinutes != 300 {
		t.Fatalf("quota=%+v", q)
	}
	if q.BlockedUntil(now.UnixMilli()) != 1800003600000 {
		t.Fatal("exhausted window did not block until reset")
	}
	if q.BlockedUntil(now.Add(time.Hour).UnixMilli()) != 0 {
		t.Fatal("expired window stayed blocked")
	}
	headers.Set("x-codex-credits-has-credits", "true")
	headers.Set("x-codex-credits-balance", "12.5")
	if FromHeaders(domain.FixedProviderCodex, "model", headers, now).BlockedUntil(now.UnixMilli()) != 0 {
		t.Fatal("available credits were ignored")
	}
	for _, invalid := range []string{"", "NaN", "+Inf", "-1", "101"} {
		headers.Set("x-codex-primary-used-percent", invalid)
		headers.Del("x-codex-secondary-used-percent")
		if FromHeaders(domain.FixedProviderCodex, "model", headers, now) != nil {
			t.Fatalf("invalid quota %q became data", invalid)
		}
	}
	if FromHeaders(domain.FixedProviderGeminiCLI, "model", headers, now) != nil {
		t.Fatal("unsupported quota was fabricated")
	}
}

func TestAnthropicQuotaIsScopedToObservedModel(t *testing.T) {
	now := time.Unix(1800000000, 0)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-requests-limit", "50")
	headers.Set("anthropic-ratelimit-requests-remaining", "0")
	headers.Set("anthropic-ratelimit-requests-reset", now.Add(time.Minute).Format(time.RFC3339))
	q := FromHeaders(domain.FixedProviderClaudeOAuth, "claude-model", headers, now)
	if q == nil || q.Model != "claude-model" || q.BlockedUntil(now.UnixMilli()) != now.Add(time.Minute).UnixMilli() {
		t.Fatalf("quota=%+v", q)
	}
}

func TestQuotaAndCapacityErrorsRemainDistinct(t *testing.T) {
	now := time.Unix(1800000000, 0)
	cases := []struct {
		body            string
		quota, capacity bool
		retry           int64
	}{
		{`{"error":{"type":"usage_limit_reached","resets_at":1800003600}}`, true, false, now.Add(time.Hour).UnixMilli()},
		{`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted, please try again later.","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"1.5s"}]}}`, false, true, now.Add(1500 * time.Millisecond).UnixMilli()},
		{`{"error":{"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaMetric":"requests_per_day"}]}]}}`, true, false, 0},
		{`{"error":{"status":"RESOURCE_EXHAUSTED"}}`, false, false, 0},
		{`not json`, false, false, 0},
	}
	for _, test := range cases {
		got := ClassifyError(429, nil, test.body, now)
		if got.QuotaExhausted != test.quota || got.CapacityExhausted != test.capacity || got.RetryAt != test.retry {
			t.Fatalf("facts=%+v for %s", got, test.body)
		}
	}
}
