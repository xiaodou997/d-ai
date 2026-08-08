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

// ApplyRequestFloor expands the attempt budget to cover the concrete route
// plan while retaining one capability-level wall-clock deadline for all tries.
func (b RetryBudget) ApplyRequestFloor(req *Request) RetryBudget {
	defaults := DefaultRetryBudget()
	if b.MaxAttempts == 0 {
		b.MaxAttempts = defaults.MaxAttempts
	}
	if b.MaxElapsed <= 0 {
		b.MaxElapsed = defaultRetryMaxElapsed(req)
	}
	// Every structurally distinct route up to the safety cap gets a chance before the request is
	// declared exhausted. Pool routes get one additional attempt so a rejected
	// OAuth credential can be replaced without consuming the route's failover
	// opportunity. Cap request amplification for accidentally huge plans.
	if req != nil {
		required := len(req.Candidates)
		for _, candidate := range req.Candidates {
			if candidate != nil && candidate.IsPoolRoute() {
				required++
			}
		}
		if required > maxUpstreamAttempts {
			required = maxUpstreamAttempts
		}
		if b.MaxAttempts < required {
			b.MaxAttempts = required
		}
	}
	if req != nil && req.CapabilityType == domain.CapabilityImage && b.MaxAttempts < defaults.MaxAttempts {
		b.MaxAttempts = defaults.MaxAttempts
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

// BackoffFor returns the sleep duration before a 429-driven retry. Uses
// 300ms × 2^(attempt-1), capped at 2s. Returns 0 for the first attempt.
func (b RetryBudget) BackoffFor(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	const base = 300 * time.Millisecond
	const cap = 2 * time.Second
	delay := base
	for i := 1; i < attempt-1; i++ {
		delay *= 2
		if delay >= cap {
			return cap
		}
	}
	return delay
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
