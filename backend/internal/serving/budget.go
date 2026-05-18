package serving

import "time"

// RetryBudget caps how aggressively the execute loop retries against
// alternate routes. The defaults below match the design doc: at most 3
// attempts within a 90-second wall-clock window.
type RetryBudget struct {
	MaxAttempts  int           // total upstream call attempts including the first
	TotalTimeout time.Duration // wall-clock cap across all attempts
}

// DefaultRetryBudget returns the project-wide default budget.
func DefaultRetryBudget() RetryBudget {
	return RetryBudget{
		MaxAttempts:  3,
		TotalTimeout: 90 * time.Second,
	}
}

// PerAttemptTimeout returns how long the next upstream call may take. It is
// the smaller of (a) the route's own configured timeout and (b) the time
// remaining before deadline. Returns 0 if the budget is already exhausted.
func (b RetryBudget) PerAttemptTimeout(deadline time.Time, routeTimeoutMs int) time.Duration {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0
	}
	route := time.Duration(routeTimeoutMs) * time.Millisecond
	if route > 0 && route < remaining {
		return route
	}
	return remaining
}

// BackoffFor returns the sleep duration before a 429-driven retry. Uses
// 300ms × 2^(attempt-1), capped at 2s. Returns 0 for first attempt.
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
