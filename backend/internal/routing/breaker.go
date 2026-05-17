// Package routing implements circuit breaker and route selection logic.
package routing

import (
	"sync"
	"time"
)

const (
	defaultFailureThreshold = 5
	defaultCooldownSeconds  = 60
)

// BreakerConfig controls the circuit breaker thresholds.
type BreakerConfig struct {
	FailureThreshold int           // consecutive failures before marking unhealthy
	Cooldown         time.Duration // how long to stay unhealthy before retrying
}

func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		FailureThreshold: defaultFailureThreshold,
		Cooldown:         defaultCooldownSeconds * time.Second,
	}
}

// deploymentState tracks per-deployment circuit breaker state in memory.
type deploymentState struct {
	consecutiveFailures int
	unhealthyUntil      time.Time
}

// CircuitBreaker tracks consecutive failures per upstream deployment.
// It is safe for concurrent use. State is in-memory only; it resets on restart.
// For persistent health state, callers also write to the DB via UpdateDeploymentHealth.
type CircuitBreaker struct {
	mu     sync.Mutex
	states map[string]*deploymentState
	cfg    BreakerConfig
}

func NewCircuitBreaker(cfg BreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = defaultFailureThreshold
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = defaultCooldownSeconds * time.Second
	}
	return &CircuitBreaker{
		states: make(map[string]*deploymentState),
		cfg:    cfg,
	}
}

// IsOpen returns true if the deployment circuit is open (unhealthy).
func (cb *CircuitBreaker) IsOpen(deploymentID string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	st := cb.states[deploymentID]
	if st == nil {
		return false
	}
	if !st.unhealthyUntil.IsZero() && time.Now().Before(st.unhealthyUntil) {
		return true
	}
	// Cooldown expired — reset to allow retry
	if !st.unhealthyUntil.IsZero() {
		st.unhealthyUntil = time.Time{}
		st.consecutiveFailures = 0
	}
	return false
}

// RecordSuccess resets the failure counter for a deployment.
func (cb *CircuitBreaker) RecordSuccess(deploymentID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.states, deploymentID)
}

// BreakerState is a snapshot of a single deployment's circuit breaker state.
type BreakerState struct {
	DeploymentID        string     `json:"deployment_id"`
	Open                bool       `json:"open"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	UnhealthyUntil      *time.Time `json:"unhealthy_until,omitempty"`
}

// ListStates returns a snapshot of all tracked deployments. Deployments with no
// recorded failures are not included (circuit is implicitly closed/healthy).
func (cb *CircuitBreaker) ListStates() []BreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	now := time.Now()
	out := make([]BreakerState, 0, len(cb.states))
	for id, st := range cb.states {
		open := !st.unhealthyUntil.IsZero() && now.Before(st.unhealthyUntil)
		var until *time.Time
		if !st.unhealthyUntil.IsZero() {
			t := st.unhealthyUntil
			until = &t
		}
		out = append(out, BreakerState{
			DeploymentID:        id,
			Open:                open,
			ConsecutiveFailures: st.consecutiveFailures,
			UnhealthyUntil:      until,
		})
	}
	return out
}

// RecordFailure increments the failure counter. Returns true when the threshold
// is crossed for the first time (caller should persist the health change to DB).
func (cb *CircuitBreaker) RecordFailure(deploymentID string) (tripped bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	st := cb.states[deploymentID]
	if st == nil {
		st = &deploymentState{}
		cb.states[deploymentID] = st
	}
	if !st.unhealthyUntil.IsZero() {
		// Cooldown still active — already tripped.
		if time.Now().Before(st.unhealthyUntil) {
			return false
		}
		// Cooldown expired without a successful probe: reset and start fresh so
		// a new failure burst doesn't continue accumulating from the old count.
		st.unhealthyUntil = time.Time{}
		st.consecutiveFailures = 0
	}
	st.consecutiveFailures++
	if st.consecutiveFailures >= cb.cfg.FailureThreshold {
		st.unhealthyUntil = time.Now().Add(cb.cfg.Cooldown)
		return true
	}
	return false
}
