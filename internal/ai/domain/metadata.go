package domain

// credentialSummaryMetadataKeys is an allowlist, not a secret-name denylist:
// imported provider metadata is arbitrary and a new credential-shaped key
// must never become public merely because its spelling was not anticipated.
var credentialSummaryMetadataKeys = [...]string{
	"account_id",
	"chatgpt_account_id",
	"accountId",
	"plan_type",
	"user_id",
	"account_user_id",
}

// CredentialSummaryMetadata returns the small, non-secret account identity
// subset allowed in management responses. Values must be strings; maps,
// arrays and other opaque provider payloads remain available only to runtime.
func CredentialSummaryMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(credentialSummaryMetadataKeys))
	for _, key := range credentialSummaryMetadataKeys {
		value, ok := in[key].(string)
		if ok {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
