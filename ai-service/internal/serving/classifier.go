package serving

import (
	"context"
	"errors"
	"net/http"
)

// ResultStatus is the coarse category of an upstream call outcome. Used by
// both the retry classifier and the HealthTracker (P2) to decide whether the
// failure should count against a circuit breaker.
type ResultStatus int

const (
	ResultUnknown ResultStatus = iota
	ResultSuccess
	ResultClientError  // 4xx (excluding 401/429) — request is malformed, don't retry
	ResultUnauthorized // 401 — credential is stale, swap and retry
	ResultRateLimited  // 429 — back off and retry on a different route
	ResultServerError  // 5xx
	ResultTimeout      // ctx deadline exceeded / explicit timeout
	ResultNetwork      // transport-level error (DNS / connection refused / TLS)
)

// String returns a short human-readable label for the status.
func (s ResultStatus) String() string {
	switch s {
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
	default:
		return "unknown"
	}
}

// Outcome is the structured result of one upstream attempt.
type Outcome struct {
	Status     ResultStatus
	HTTPStatus int // 0 if no response was received
	Err        error
}

// Decision is what the retry loop should do next.
type Decision int

const (
	DecisionAccept       Decision = iota // success — relay to client and stop
	DecisionRetry                        // try another route from the candidate list
	DecisionRetryNewCred                 // OAuth 401 — same pool, new credential
	DecisionGiveUp                       // client error — return failure to client
)

// ClassifyOutcome translates a transport-level (status, err) pair into a
// structured Outcome. Callers then read Outcome.Decision() to drive the loop.
func ClassifyOutcome(httpStatus int, err error) Outcome {
	if err != nil {
		switch {
		case errors.Is(err, ErrConnectTimeout), errors.Is(err, ErrFirstByteTimeout),
			errors.Is(err, ErrIdleTimeout), errors.Is(err, ErrMaxDuration),
			errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return Outcome{Status: ResultTimeout, Err: err}
		}
		return Outcome{Status: ResultNetwork, Err: err}
	}
	switch {
	case httpStatus >= 200 && httpStatus < 300:
		return Outcome{Status: ResultSuccess, HTTPStatus: httpStatus}
	case httpStatus == http.StatusUnauthorized:
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
// (relevant for the Unauthorized → swap-credential vs swap-route decision).
func (o Outcome) Decision(hasCredential bool) Decision {
	switch o.Status {
	case ResultSuccess:
		return DecisionAccept
	case ResultUnauthorized:
		if hasCredential {
			return DecisionRetryNewCred
		}
		return DecisionGiveUp
	case ResultRateLimited, ResultServerError, ResultTimeout, ResultNetwork:
		return DecisionRetry
	case ResultClientError:
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
// for X-Route-Trace observability and pipeline-internal bookkeeping.
type AttemptRecord struct {
	RouteID     string
	TargetID    string // deployment_id or credential_id
	HTTPStatus  int
	Outcome     ResultStatus
	LatencyMs   int    // connect phase: request sent → response headers
	FirstByteMs int    // headers → first committed byte (0 when not committed)
	ErrorMsg    string
	Score       float64 // scorer probability from softmax (0 when scorer unavailable)
}
