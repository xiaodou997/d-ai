package domain

// CredentialQuota contains only numeric quota facts reported by the upstream.
// Empty/unknown values never imply that an account has exhausted its quota.
type CredentialQuota struct {
	Model            string                  `json:"model,omitempty"`
	Source           string                  `json:"source"`
	ObservedAt       int64                   `json:"observed_at"`
	Windows          []CredentialQuotaWindow `json:"windows"`
	CreditsAvailable bool                    `json:"credits_available,omitempty"`
}

type CredentialQuotaWindow struct {
	Key           string   `json:"key"`
	UsedPercent   *float64 `json:"used_percent,omitempty"`
	Remaining     *float64 `json:"remaining,omitempty"`
	Limit         *float64 `json:"limit,omitempty"`
	WindowMinutes *int     `json:"window_minutes,omitempty"`
	ResetAt       int64    `json:"reset_at,omitempty"`
}

func (q CredentialQuota) BlockedUntil(now int64) int64 {
	if q.CreditsAvailable {
		return 0
	}
	var until int64
	for _, w := range q.Windows {
		exhausted := w.UsedPercent != nil && *w.UsedPercent >= 100 || w.Remaining != nil && *w.Remaining <= 0
		if exhausted && w.ResetAt > now && w.ResetAt > until {
			until = w.ResetAt
		}
	}
	return until
}
