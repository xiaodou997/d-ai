// Package upstreamquota interprets documented quota facts without estimating
// subscription allowance from local token billing.
package upstreamquota

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func number(raw string) *float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	return &v
}

// ResetAt accepts Unix seconds, RFC3339 and duration-form rate-limit headers.
func ResetAt(raw string, observed time.Time) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var at time.Time
	if seconds := number(raw); seconds != nil {
		if *seconds > 1e12 {
			at = time.UnixMilli(int64(*seconds))
		} else {
			at = time.Unix(int64(*seconds), 0)
		}
	} else if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		at = t
	} else if t, err := http.ParseTime(raw); err == nil {
		at = t
	} else if duration, err := time.ParseDuration(raw); err == nil && duration > 0 && duration <= 31*24*time.Hour {
		at = observed.Add(duration)
	}
	if !at.After(observed) || at.After(observed.Add(31*24*time.Hour)) {
		return 0
	}
	return at.UnixMilli()
}

// FromHeaders follows OpenAI Codex's rate_limits.rs and Anthropic's rate-limit
// headers. Window duration is supplied by the provider, never inferred as 5h/7d.
func FromHeaders(provider domain.FixedProviderType, model string, headers http.Header, observed time.Time) *domain.CredentialQuota {
	q := &domain.CredentialQuota{Model: model, Source: "response_headers", ObservedAt: observed.UnixMilli(), Windows: []domain.CredentialQuotaWindow{}}
	switch provider {
	case domain.FixedProviderCodex:
		q.Model = "" // The base Codex allowance is shared by the account.
		for _, key := range []string{"primary", "secondary"} {
			prefix := "x-codex-" + key + "-"
			used := number(headers.Get(prefix + "used-percent"))
			if used == nil || *used > 100 {
				continue
			}
			w := domain.CredentialQuotaWindow{Key: key, UsedPercent: used, ResetAt: ResetAt(headers.Get(prefix+"reset-at"), observed)}
			if minutes, err := strconv.Atoi(headers.Get(prefix + "window-minutes")); err == nil && minutes > 0 {
				w.WindowMinutes = &minutes
			}
			q.Windows = append(q.Windows, w)
		}
		hasCredits, _ := strconv.ParseBool(headers.Get("x-codex-credits-has-credits"))
		unlimited, _ := strconv.ParseBool(headers.Get("x-codex-credits-unlimited"))
		balance := number(headers.Get("x-codex-credits-balance"))
		q.CreditsAvailable = unlimited || hasCredits && (balance == nil || *balance > 0)
	case domain.FixedProviderClaudeOAuth:
		for _, key := range []string{"requests", "tokens", "input-tokens", "output-tokens"} {
			prefix := "anthropic-ratelimit-" + key + "-"
			remaining := number(headers.Get(prefix + "remaining"))
			if remaining == nil {
				continue
			}
			q.Windows = append(q.Windows, domain.CredentialQuotaWindow{Key: key, Remaining: remaining, Limit: number(headers.Get(prefix + "limit")), ResetAt: ResetAt(headers.Get(prefix+"reset"), observed)})
		}
	}
	if len(q.Windows) == 0 {
		return nil
	}
	return q
}
