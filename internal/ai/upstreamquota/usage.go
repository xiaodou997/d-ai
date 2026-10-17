package upstreamquota

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

func percent(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 100 {
		return nil
	}
	return v
}

// CodexUsage follows openai/codex's RateLimitStatusPayload and window snapshot.
func CodexUsage(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	type window struct {
		Used       *float64 `json:"used_percent"`
		Seconds    int      `json:"limit_window_seconds"`
		ResetAt    int64    `json:"reset_at"`
		ResetAfter *int64   `json:"reset_after_seconds"`
	}
	var doc struct {
		RateLimit *struct {
			Primary   *window `json:"primary_window"`
			Secondary *window `json:"secondary_window"`
		} `json:"rate_limit"`
		Credits *struct {
			HasCredits bool            `json:"has_credits"`
			Unlimited  bool            `json:"unlimited"`
			Balance    json.RawMessage `json:"balance"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("invalid codex quota response")
	}
	q := domain.CredentialQuota{Source: "quota_api", ObservedAt: now.UnixMilli(), Windows: []domain.CredentialQuotaWindow{}}
	if doc.RateLimit != nil {
		for _, entry := range []struct {
			key   string
			value *window
		}{{"primary", doc.RateLimit.Primary}, {"secondary", doc.RateLimit.Secondary}} {
			w := entry.value
			if w == nil || percent(w.Used) == nil {
				continue
			}
			reset := ResetAt(strconv.FormatInt(w.ResetAt, 10), now)
			if reset == 0 && w.ResetAfter != nil && *w.ResetAfter > 0 && *w.ResetAfter <= 31*24*3600 {
				reset = now.Add(time.Duration(*w.ResetAfter) * time.Second).UnixMilli()
			}
			out := domain.CredentialQuotaWindow{Key: entry.key, UsedPercent: w.Used, ResetAt: reset}
			if w.Seconds > 0 && w.Seconds <= 31*24*3600 && w.Seconds%60 == 0 {
				minutes := w.Seconds / 60
				out.WindowMinutes = &minutes
			}
			q.Windows = append(q.Windows, out)
		}
	}
	if c := doc.Credits; c != nil {
		balance := number(strings.Trim(string(c.Balance), `"`))
		q.CreditsAvailable = c.Unlimited || c.HasCredits && (balance == nil || *balance > 0)
	}
	if len(q.Windows) == 0 {
		return nil, nil
	}
	return []domain.CredentialQuota{q}, nil
}

// Only the shared Claude windows can block all models. Product-specific
// windows must not accidentally disable other model families.
func ClaudeUsage(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("invalid claude quota response")
	}
	q := domain.CredentialQuota{Source: "quota_api", ObservedAt: now.UnixMilli(), Windows: []domain.CredentialQuotaWindow{}}
	for _, key := range []string{"five_hour", "seven_day"} {
		var w struct {
			Utilization *float64 `json:"utilization"`
			ResetsAt    string   `json:"resets_at"`
		}
		if json.Unmarshal(doc[key], &w) != nil || percent(w.Utilization) == nil {
			continue
		}
		q.Windows = append(q.Windows, domain.CredentialQuotaWindow{Key: key, UsedPercent: w.Utilization, ResetAt: ResetAt(w.ResetsAt, now)})
	}
	if len(q.Windows) == 0 {
		return nil, nil
	}
	return []domain.CredentialQuota{q}, nil
}

// GeminiUsage follows google-gemini/gemini-cli's RetrieveUserQuotaResponse.
func GeminiUsage(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	var doc struct {
		Buckets []struct {
			RemainingAmount string   `json:"remainingAmount"`
			Fraction        *float64 `json:"remainingFraction"`
			Reset           string   `json:"resetTime"`
			TokenType       string   `json:"tokenType"`
			Model           string   `json:"modelId"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("invalid gemini quota response")
	}
	byModel := map[string]*domain.CredentialQuota{}
	for _, b := range doc.Buckets {
		model := strings.TrimPrefix(strings.TrimSpace(b.Model), "models/")
		if model == "" {
			continue
		} // An unnamed bucket is not proof of account-wide exhaustion.
		w := domain.CredentialQuotaWindow{Key: b.TokenType, Remaining: number(b.RemainingAmount), ResetAt: ResetAt(b.Reset, now)}
		if w.Key == "" {
			w.Key = "quota"
		}
		if b.Fraction != nil && *b.Fraction >= 0 && *b.Fraction <= 1 {
			used := (1 - *b.Fraction) * 100
			w.UsedPercent = &used
		}
		if w.Remaining == nil && w.UsedPercent == nil {
			continue
		}
		q := byModel[model]
		if q == nil {
			q = &domain.CredentialQuota{Model: model, Source: "quota_api", ObservedAt: now.UnixMilli(), Windows: []domain.CredentialQuotaWindow{}}
			byModel[model] = q
		}
		q.Windows = append(q.Windows, w)
	}
	return sortedQuotas(byModel), nil
}

// Antigravity's model manifest includes quotaInfo for each exact model ID.
func AntigravityUsage(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	var doc struct {
		Models map[string]struct {
			Quota *struct {
				Fraction *float64 `json:"remainingFraction"`
				Reset    string   `json:"resetTime"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("invalid antigravity quota response")
	}
	out := map[string]*domain.CredentialQuota{}
	for id, model := range doc.Models {
		id = strings.TrimPrefix(strings.TrimSpace(id), "models/")
		if id == "" || model.Quota == nil || model.Quota.Fraction == nil {
			continue
		}
		fraction := *model.Quota.Fraction
		if math.IsNaN(fraction) || math.IsInf(fraction, 0) || fraction < 0 || fraction > 1 {
			continue
		}
		used := (1 - fraction) * 100
		out[id] = &domain.CredentialQuota{Model: id, Source: "quota_api", ObservedAt: now.UnixMilli(), Windows: []domain.CredentialQuotaWindow{{Key: "quota", UsedPercent: &used, ResetAt: ResetAt(model.Quota.Reset, now)}}}
	}
	return sortedQuotas(out), nil
}

func sortedQuotas(in map[string]*domain.CredentialQuota) []domain.CredentialQuota {
	out := make([]domain.CredentialQuota, 0, len(in))
	for _, q := range in {
		out = append(out, *q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}
