package domain

// CredentialQuotaInspection reports discovery health separately from quota
// amounts and from the account's authentication/management status.
type CredentialQuotaInspection struct {
	State      string `json:"state" enum:"live,cache,unknown,failed,unsupported"`
	ObservedAt int64  `json:"observed_at,omitempty" doc:"上次主动查询时间，Unix 毫秒"`
}
