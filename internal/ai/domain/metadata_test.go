package domain

import "testing"

func TestCredentialSummaryMetadataAllowsOnlyKnownStringIdentityFields(t *testing.T) {
	in := map[string]any{
		"account_id":          "account-1",
		"chatgpt_account_id":  "chatgpt-1",
		"accountId":           "legacy-account-1",
		"plan_type":           "team",
		"user_id":             "user-1",
		"account_user_id":     "account-user-1",
		"private_key":         "must-not-leak",
		"authorization":       "must-not-leak",
		"cookie":              "must-not-leak",
		"bearer":              "must-not-leak",
		"credential":          "must-not-leak",
		"access_token":        "must-not-leak",
		"provider_project_id": "not-part-of-the-public-contract",
		"nested": map[string]any{
			"account_id": "nested-values-are-not-public",
		},
	}

	got := CredentialSummaryMetadata(in)
	want := map[string]string{
		"account_id":         "account-1",
		"chatgpt_account_id": "chatgpt-1",
		"accountId":          "legacy-account-1",
		"plan_type":          "team",
		"user_id":            "user-1",
		"account_user_id":    "account-user-1",
	}
	if len(got) != len(want) {
		t.Fatalf("summary metadata = %#v, want exactly %d allowed fields", got, len(want))
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("summary metadata[%q] = %#v, want %q", key, got[key], value)
		}
	}
}

func TestCredentialSummaryMetadataRejectsNonStringAllowedValues(t *testing.T) {
	got := CredentialSummaryMetadata(map[string]any{
		"account_id": map[string]any{"private_key": "must-not-leak"},
		"user_id":    []any{"must-not-leak"},
		"plan_type":  1,
	})
	if got != nil {
		t.Fatalf("summary metadata = %#v, want nil", got)
	}
}
