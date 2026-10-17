package domain

import "strings"

// CredentialModelIdentity scopes model observations to one credential revision.
// It contains no tokens, ciphertexts, or account metadata.
type CredentialModelIdentity struct {
	ID           string
	PoolID       string
	ProviderType FixedProviderType
	TokenVersion int64
}

// CredentialModelCatalog is an account-specific observation. A built-in
// provider catalog is never evidence of an individual account's permissions.
type CredentialModelCatalog struct {
	CredentialID string   `json:"credential_id"`
	Models       []string `json:"models"`
	Source       string   `json:"source" enum:"live,cache,stale,unknown"`
	ObservedAt   int64    `json:"observed_at,omitempty" doc:"观测时间，Unix 毫秒"`
	ValidUntil   int64    `json:"valid_until,omitempty" doc:"有效截止时间，Unix 毫秒"`
}

// Excludes only trusts a fresh, successful account manifest. Discovery errors,
// expired snapshots and providers without inspection remain unknown and usable.
func (c CredentialModelCatalog) Excludes(model string, now int64) bool {
	if (c.Source != "live" && c.Source != "cache") || c.ValidUntil <= now || len(c.Models) == 0 {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, id := range c.Models {
		if id == model {
			return false
		}
	}
	return true
}
