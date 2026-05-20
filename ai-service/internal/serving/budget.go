package serving

import "time"

// RetryBudget caps how aggressively the execute loop retries against alternate
// routes BEFORE the response is committed to the client. Once the response is
// committed (the streaming first byte is forwarded, or a sync body is written),
// the retry loop is over — per-attempt connect/first-byte/idle/max-duration
// timeouts are owned by deadlineController, not by this budget.
type RetryBudget struct {
	MaxAttempts int           // total upstream call attempts including the first
	RetryWindow time.Duration // wall-clock cap for the pre-commit retry loop
}

// DefaultRetryBudget returns the project-wide default: at most 3 pre-commit
// attempts within a 90-second window.
func DefaultRetryBudget() RetryBudget {
	return RetryBudget{
		MaxAttempts: 3,
		RetryWindow: 90 * time.Second,
	}
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
