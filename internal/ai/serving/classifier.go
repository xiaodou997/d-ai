package serving

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
	"xiaodou/dai/internal/ai/domain"
)

// ResultStatus is shared by retry policy, availability and attempt statistics.
type ResultStatus int

const (
	ResultUnknown ResultStatus = iota
	ResultSuccess
	ResultClientError  // other 4xx — request is malformed, don't retry
	ResultUnauthorized // 401/403 — credential is rejected, swap or fail over
	ResultRateLimited  // 429 — back off and retry on a different route
	ResultServerError  // 5xx
	ResultTimeout      // ctx deadline exceeded / explicit timeout
	ResultNetwork      // transport-level error (DNS / connection refused / TLS)
	ResultCanceled     // caller context ended; never retry or penalize upstream health
	ResultCircuitOpen  // candidate skipped: circuit breaker open, never reached transport
	ResultRejected     // candidate skipped by planner/binder verdict, never reached transport
	ResultModelError   // upstream model or operation is unavailable
)

// String returns a short human-readable label for the status.
func (s ResultStatus) String() string {
	switch s {
	case ResultModelError:
		return "model_error"
	case ResultSuccess:
		return "success"
	case ResultClientError:
		return "client_error"
	case ResultUnauthorized:
		return "unauthorized"
	case ResultRateLimited:
		return "rate_limited"
	case ResultServerError:
		return "server_error"
	case ResultTimeout:
		return "timeout"
	case ResultNetwork:
		return "network_error"
	case ResultCanceled:
		return "canceled"
	case ResultCircuitOpen:
		return "circuit_open"
	case ResultRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// Outcome is the structured result of one upstream attempt.
type AttemptOutcome struct {
	ErrorCode  string
	Status     ResultStatus
	RetryAt    int64
	HTTPStatus int // 0 if no response was received
	Err        error
}

// Outcome retains the concise name used by the executor.
type Outcome = AttemptOutcome

// Decision is what the retry loop should do next.
type Decision int

const (
	DecisionAccept       Decision = iota // success — relay to client and stop
	DecisionRetry                        // try another route from the candidate list
	DecisionRetryNewCred                 // OAuth 401/403 — same pool, new credential
	DecisionGiveUp                       // client error — return failure to client
)

// ClassifyOutcome translates a transport-level (status, err) pair into a
// structured Outcome. Callers then read Outcome.Decision() to drive the loop.
func ClassifyOutcome(httpStatus int, err error) Outcome {
	if err != nil {
		var timeout net.Error
		switch {
		case errors.Is(err, context.Canceled):
			return Outcome{Status: ResultCanceled, Err: err}
		case errors.Is(err, ErrResponseHeaderTimeout), errors.Is(err, ErrFirstByteTimeout),
			errors.Is(err, ErrIdleTimeout), errors.Is(err, ErrMaxDuration),
			errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout():
			return Outcome{Status: ResultTimeout, Err: err}
		}
		return Outcome{Status: ResultNetwork, Err: err}
	}
	switch {
	case httpStatus >= 200 && httpStatus < 300:
		return Outcome{Status: ResultSuccess, HTTPStatus: httpStatus}
	case httpStatus == http.StatusUnauthorized || httpStatus == http.StatusForbidden:
		return Outcome{Status: ResultUnauthorized, HTTPStatus: httpStatus}
	case httpStatus == http.StatusTooManyRequests:
		return Outcome{Status: ResultRateLimited, HTTPStatus: httpStatus}
	case httpStatus >= 500:
		return Outcome{Status: ResultServerError, HTTPStatus: httpStatus}
	case httpStatus >= 400:
		return Outcome{Status: ResultClientError, HTTPStatus: httpStatus}
	default:
		return Outcome{Status: ResultUnknown, HTTPStatus: httpStatus}
	}
}

// Decision returns what the retry loop should do given the outcome's
// classification. The mapping deliberately treats 429 as retryable-but-not-
// breakable and 5xx/timeout/network as both retryable and breakable.
//
// hasCredential indicates whether the current attempt used a Pool credential
// (relevant for the Unauthorized/Forbidden credential-swap decision).
func (o Outcome) Decision(hasCredential bool) Decision {
	switch o.Status {
	case ResultSuccess:
		return DecisionAccept
	case ResultUnauthorized:
		if hasCredential {
			return DecisionRetryNewCred
		}
		return DecisionRetry
	case ResultModelError, ResultRateLimited, ResultServerError, ResultTimeout, ResultNetwork:
		return DecisionRetry
	case ResultClientError:
		return DecisionGiveUp
	case ResultCanceled:
		return DecisionGiveUp
	default:
		// Unknown/no-response — be conservative and retry once.
		return DecisionRetry
	}
}

// CountsAsHealthFailure returns whether the outcome should bump the circuit
// breaker counter. 429 is excluded — rate limiting from a deployment is an
// orthogonal concern from health and would cause spurious breaker trips.
func (o Outcome) CountsAsHealthFailure() bool {
	switch o.Status {
	case ResultServerError, ResultTimeout, ResultNetwork:
		return true
	default:
		return false
	}
}

// AttemptRecord captures one upstream call inside the retry loop. Used both
// for X-Route-Trace observability (via observability.BuildTrace, which reads
// only route-policy metadata, score, outcome and timings — never upstream
// identity or raw error fields)
// and for the admin-only persisted retry trail (via BuildAttemptsDetail,
// serving/attempts_detail.go). ProviderCode/UpstreamModel/EndpointID/PoolID/
// CredentialID/ErrorMsg must never reach the client — they identify internal
// upstream accounts and may contain raw transport error text.
type AttemptRecord struct {
	OutcomeFinal        bool
	FirstOutputMs       int
	CandidateID         string
	AccountID           string
	ModelCode           string
	Operation           string
	Stream              bool
	AvailabilityOutcome string
	FailureScope        string
	CooldownEntered     bool
	RetryAt             int64
	UpstreamErrorCode   string

	UsageEvidence         domain.UsageEvidence         `json:"usage_evidence"`
	ProviderTerminalState domain.ProviderTerminalState `json:"provider_terminal_state"`
	PricingSnapshot       domain.BillingSnapshot       `json:"pricing_snapshot"`
	Sequence              int                          `json:"-"` // Request-local order shared by calls and execution skips; planning precedes both.
	RouteID               string
	GroupID               string
	RoutePolicy           string
	GroupRank             int
	TargetPriority        int // 分组内人工优先级（越小越优先，100 = 默认平级），用于 attempts_detail 展示
	SelectionReason       string
	TargetID              string // deployment_id or credential_id (legacy/opaque; kept for existing consumers)
	ProviderCode          string
	UpstreamModel         string
	EndpointID            string // ai_upstream_account_endpoints.id; empty for pool routes
	PoolID                string // ai_credential_pools.id; empty for account routes
	CredentialID          string // OAuth credential actually used this attempt; empty when not pool-based
	ProfileRevision       string // fixed-client profile frozen for this attempt
	UpstreamRequestID     string // admin-only correlation from the upstream response
	HTTPStatus            int
	Outcome               ResultStatus
	StartedAt             time.Time `json:"-"`
	TransportStartedAt    time.Time `json:"-"`
	CompletedAt           time.Time `json:"-"`
	LatencyMs             int       // connect phase: request sent → response headers
	FirstByteMs           int       // request sent → first committed byte (0 when not committed)
	TotalMs               int
	ErrorMsg              string
	Score                 float64 // policy utility (0 when no comparison was needed)
}

// ClassifyResponse uses provider error facts before applying retry policy.
func ClassifyResponse(status int, headers http.Header, body string, err error) Outcome {
	out := ClassifyOutcome(status, err)
	out.RetryAt = retryAfter(headers)
	if err != nil {
		return out
	}
	var envelope struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Status  string          `json:"status"`
			Type    string          `json:"type"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal([]byte(body), &envelope)
	out.ErrorCode = strings.Trim(string(envelope.Error.Code), "\"")
	if out.ErrorCode == "" || out.ErrorCode == "null" {
		out.ErrorCode = envelope.Error.Type
	}
	if out.ErrorCode == "" {
		out.ErrorCode = envelope.Error.Status
	}
	code := strings.ToLower(out.ErrorCode + " " + envelope.Error.Type + " " + envelope.Error.Status)
	if status == http.StatusTooManyRequests {
		return out
	}
	modelError := strings.Contains(code, "model_not_found") || strings.Contains(code, "model_not_available") || strings.Contains(code, "model_access") || strings.Contains(code, "unsupported_model") || strings.Contains(code, "insufficient_quota")
	authError := strings.Contains(code, "invalid_api_key") || strings.Contains(code, "invalid_token") || strings.Contains(code, "authentication_error") || strings.Contains(code, "invalid_grant") || strings.Contains(code, "unauthenticated")
	// A generic gateway/WAF denial does not prove a model or key is invalid.
	if status == http.StatusForbidden && !authError && !modelError {
		out.Status = ResultServerError
	}
	if status == http.StatusNotFound || modelError {
		out.Status = ResultModelError
	}
	if authError {
		out.Status = ResultUnauthorized
	}
	return out
}
