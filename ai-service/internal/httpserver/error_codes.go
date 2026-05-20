package httpserver

const (
	errorCodeAuthInvalidToken        = "AUTH_INVALID_TOKEN"
	errorCodeAuthModelNotGranted     = "AUTH_MODEL_NOT_GRANTED"
	errorCodeModelNotFound           = "MODEL_NOT_FOUND"
	errorCodeRouteNotFound           = "ROUTE_NOT_FOUND"
	errorCodeUpstreamTimeout         = "UPSTREAM_TIMEOUT"
	errorCodeUpstreamBadStatus       = "UPSTREAM_BAD_STATUS"
	errorCodeUpstreamRequestFailed   = "UPSTREAM_REQUEST_FAILED"
	errorCodeUpstreamProtocolError   = "UPSTREAM_PROTOCOL_ERROR"
	errorCodeQuotaInsufficient       = "QUOTA_INSUFFICIENT"
	errorCodeQuotaReservationFailed  = "QUOTA_RESERVATION_FAILED"
	errorCodeSettlementFailed        = "SETTLEMENT_FAILED"
	errorCodeSettlementRejected      = "SETTLEMENT_REJECTED"
	errorCodeRateLimited             = "RATE_LIMITED"
	errorCodeProviderCredentialError = "PROVIDER_CREDENTIAL_ERROR"
	errorCodeInvalidRequest          = "INVALID_REQUEST"
	errorCodeConfigInvalid           = "CONFIG_INVALID"
	errorCodeInternal                = "INTERNAL_ERROR"
)
