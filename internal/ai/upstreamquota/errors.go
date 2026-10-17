package upstreamquota

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type ErrorFacts struct {
	QuotaExhausted    bool
	CapacityExhausted bool
	RetryAt           int64
}

// ClassifyError separates confirmed quota violations from shared capacity.
// RESOURCE_EXHAUSTED alone is ambiguous and remains ordinary rate limiting.
func ClassifyError(status int, headers http.Header, body string, observed time.Time) ErrorFacts {
	facts := ErrorFacts{}
	if status != http.StatusTooManyRequests && status != http.StatusServiceUnavailable && status != http.StatusPaymentRequired {
		return facts
	}
	var envelope struct {
		Error struct {
			Code       json.RawMessage `json:"code"`
			Type       string          `json:"type"`
			Status     string          `json:"status"`
			Message    string          `json:"message"`
			ResetsAt   json.RawMessage `json:"resets_at"`
			ResetAt    json.RawMessage `json:"reset_at"`
			RetryAfter json.RawMessage `json:"retry_after"`
			Details    []struct {
				Type       string            `json:"@type"`
				RetryDelay string            `json:"retryDelay"`
				Violations []json.RawMessage `json:"violations"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal([]byte(body), &envelope)
	e := envelope.Error
	code := strings.ToLower(string(e.Code) + " " + e.Type + " " + e.Status)
	for _, known := range []string{"usage_limit_reached", "insufficient_quota", "quota_exceeded", "quota_exhausted", "billing_hard_limit_reached"} {
		if strings.Contains(code, known) {
			facts.QuotaExhausted = true
		}
	}
	for _, detail := range e.Details {
		if strings.HasSuffix(detail.Type, "google.rpc.QuotaFailure") && len(detail.Violations) > 0 {
			facts.QuotaExhausted = true
		}
		if strings.HasSuffix(detail.Type, "google.rpc.RetryInfo") {
			if duration, err := time.ParseDuration(detail.RetryDelay); err == nil && duration > 0 && duration <= 31*24*time.Hour {
				facts.RetryAt = max(facts.RetryAt, observed.Add(duration).UnixMilli())
			}
		}
	}
	for _, raw := range []json.RawMessage{e.ResetsAt, e.ResetAt} {
		facts.RetryAt = max(facts.RetryAt, ResetAt(strings.Trim(string(raw), "\""), observed))
	}
	if seconds := number(strings.Trim(string(e.RetryAfter), "\"")); seconds != nil && *seconds <= 31*24*3600 {
		facts.RetryAt = max(facts.RetryAt, observed.Add(time.Duration(*seconds*float64(time.Second))).UnixMilli())
	}
	if !facts.QuotaExhausted {
		message := strings.ToLower(e.Message)
		facts.CapacityExhausted = strings.Contains(code, "capacity_exhausted") || strings.Contains(code, "model_overloaded") || strings.Contains(message, "resource exhausted, please try again later") || strings.Contains(message, "model is at capacity") || strings.Contains(message, "shared capacity")
	}
	for _, key := range []string{"primary", "secondary"} {
		prefix := "x-codex-" + key + "-"
		if used := number(headers.Get(prefix + "used-percent")); used != nil && *used >= 100 {
			facts.RetryAt = max(facts.RetryAt, ResetAt(headers.Get(prefix+"reset-at"), observed))
		}
	}
	for _, key := range []string{"requests", "tokens", "input-tokens", "output-tokens"} {
		prefix := "anthropic-ratelimit-" + key + "-"
		if remaining := number(headers.Get(prefix + "remaining")); remaining != nil && *remaining == 0 {
			facts.RetryAt = max(facts.RetryAt, ResetAt(headers.Get(prefix+"reset"), observed))
		}
	}
	return facts
}
