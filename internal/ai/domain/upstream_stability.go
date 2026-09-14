package domain

// UpstreamStability counts provider attempts, never final gateway requests.
// Counters cover only instrumented records, beginning at CoverageStartedAt.
type UpstreamRuntimePath struct {
	EndpointID   string `json:"endpoint_id"`
	CredentialID string `json:"credential_id"`
	Model        string `json:"model"`
	Operation    string `json:"operation"`
}
type UpstreamStability struct {
	Paths              []UpstreamRuntimePath    `json:"paths"`
	EndpointCount      int                      `json:"endpoint_count"`
	CredentialCount    int                      `json:"credential_count"`
	ModelCount         int                      `json:"model_count"`
	ResourceKind       string                   `json:"resource_kind"`
	ConfigStatus       string                   `json:"config_status"`
	ResourceID         string                   `json:"resource_id"`
	Window             string                   `json:"window"`
	CoverageStartedAt  int64                    `json:"coverage_started_at"`
	Successes          int64                    `json:"successes"`
	Failures           int64                    `json:"failures"`
	Excluded           int64                    `json:"excluded"`
	Samples            int64                    `json:"samples"`
	SuccessRate        *float64                 `json:"success_rate"`
	CooldownsLastHour  int64                    `json:"cooldowns_last_hour"`
	RepeatedFailure    bool                     `json:"repeated_failure"`
	StabilityDeclining bool                     `json:"stability_declining"`
	Models             []UpstreamModelStability `json:"models"`
}
type UpstreamModelStability struct {
	EndpointID   string  `json:"endpoint_id"`
	CredentialID string  `json:"credential_id"`
	Model        string  `json:"model"`
	Operation    string  `json:"operation"`
	Stream       bool    `json:"stream"`
	Outcome      string  `json:"outcome"`
	Count        int64   `json:"count"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	LastError    string  `json:"last_error"`
}
