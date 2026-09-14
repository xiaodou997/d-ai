package serving

import (
	"errors"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const maxUpstreamAttempts = 8

var ErrRetryDeadlineExceeded = errors.New("total upstream retry deadline exceeded")

// RetryBudget caps how aggressively the execute loop retries against alternate
// routes BEFORE the response is committed to the client. Once the response is
// committed (the streaming first byte is forwarded, or a sync body is written),
// the retry loop is over — per-attempt connect/first-byte/idle/max-duration
// timeouts are owned by deadlineController, not by this budget.
type RetryBudget struct {
	MaxAttempts int // total upstream call attempts including the first
	MaxElapsed  time.Duration
}

// DefaultRetryBudget returns the project-wide default for small route plans.
func DefaultRetryBudget() RetryBudget {
	return RetryBudget{
		MaxAttempts: 3,
	}
}

// Normalize applies the fixed send cap and one wall-clock deadline for all tries.
func (b RetryBudget) Normalize(req *Request) RetryBudget {
	defaults := DefaultRetryBudget()
	if b.MaxAttempts == 0 {
		b.MaxAttempts = defaults.MaxAttempts
	}
	if b.MaxElapsed <= 0 {
		b.MaxElapsed = defaultRetryMaxElapsed(req)
	}
	if b.MaxAttempts > maxUpstreamAttempts {
		b.MaxAttempts = maxUpstreamAttempts
	}
	return b
}

func defaultRetryMaxElapsed(req *Request) time.Duration {
	capability := domain.CapabilityChat
	if req != nil {
		capability = req.CapabilityType
	}
	return domain.DefaultRouteTimeouts(capability).MaxDuration
}

// RequestLeaseTTL bounds the lifetime of a Redis concurrency slot. It follows
// the request retry deadline and keeps a small recovery grace period so a
// crashed request does not hold the slot forever. This is unrelated to the
// removed billing authorization flow.
func RequestLeaseTTL(req *Request) time.Duration {
	ttl := defaultRetryMaxElapsed(req)
	if ttl < time.Minute {
		ttl = time.Minute
	}
	return ttl + 2*time.Minute
}
