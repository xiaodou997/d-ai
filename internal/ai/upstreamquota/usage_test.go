package upstreamquota

import (
	"testing"
	"time"
	"xiaodou/dai/internal/ai/domain"
)

func TestSubscriptionQuotaContractsAndUnknownSafety(t *testing.T) {
	now := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name, body      string
		decode          func([]byte, time.Time) ([]domain.CredentialQuota, error)
		models, windows int
		blocked         bool
	}{
		{"codex actual reset and duration", `{"rate_limit":{"primary_window":{"used_percent":100,"limit_window_seconds":18000,"reset_after_seconds":120},"secondary_window":null}}`, CodexUsage, 1, 1, true},
		{"codex credits permit overflow", `{"rate_limit":{"primary_window":{"used_percent":100,"reset_after_seconds":120}},"credits":{"has_credits":true,"balance":"5"}}`, CodexUsage, 1, 1, false},
		{"codex missing is unknown", `{"rate_limit":{"primary_window":{}}}`, CodexUsage, 0, 0, false},
		{"codex invalid percentage is unknown", `{"rate_limit":{"primary_window":{"used_percent":101}}}`, CodexUsage, 0, 0, false},
		{"claude shared windows", `{"five_hour":{"utilization":100,"resets_at":"2026-10-17T09:00:00Z"},"seven_day":{"utilization":25,"resets_at":"2026-10-20T09:00:00Z"}}`, ClaudeUsage, 1, 2, true},
		{"claude family must not block all models", `{"seven_day_sonnet":{"utilization":100,"resets_at":"2026-10-17T09:00:00Z"},"five_hour":null}`, ClaudeUsage, 0, 0, false},
		{"gemini exact bucket", `{"buckets":[{"modelId":"models/gemini-3-pro","remainingAmount":"0","resetTime":"2026-10-17T09:00:00Z","tokenType":"REQUESTS"}]}`, GeminiUsage, 1, 1, true},
		{"gemini missing amount is unknown", `{"buckets":[{"modelId":"gemini-3-pro","remainingFraction":null}]}`, GeminiUsage, 0, 0, false},
		{"gemini unnamed bucket cannot block account", `{"buckets":[{"remainingFraction":0,"resetTime":"2026-10-17T09:00:00Z"}]}`, GeminiUsage, 0, 0, false},
		{"antigravity exact model", `{"models":{"gemini-3-pro":{"quotaInfo":{"remainingFraction":0,"resetTime":"2026-10-17T09:00:00Z"}}}}`, AntigravityUsage, 1, 1, true},
		{"antigravity missing fraction is unknown", `{"models":{"gemini-3-pro":{"quotaInfo":{"resetTime":"2026-10-17T09:00:00Z"}}}}`, AntigravityUsage, 0, 0, false},
		{"antigravity impossible fraction is unknown", `{"models":{"gemini-3-pro":{"quotaInfo":{"remainingFraction":-1}}}}`, AntigravityUsage, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quotas, err := tc.decode([]byte(tc.body), now)
			if err != nil || len(quotas) != tc.models {
				t.Fatalf("quotas=%+v err=%v", quotas, err)
			}
			if len(quotas) == 0 {
				return
			}
			q := quotas[0]
			if len(q.Windows) != tc.windows || (q.BlockedUntil(now.UnixMilli()) > 0) != tc.blocked || q.Source != "quota_api" || q.ObservedAt != now.UnixMilli() {
				t.Fatalf("quota=%+v", q)
			}
			if tc.name == "codex actual reset and duration" && (q.Windows[0].WindowMinutes == nil || *q.Windows[0].WindowMinutes != 300 || q.Windows[0].ResetAt != now.Add(2*time.Minute).UnixMilli()) {
				t.Fatalf("window=%+v", q.Windows[0])
			}
		})
	}
}
