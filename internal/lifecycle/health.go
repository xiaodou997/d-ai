// Package lifecycle contains small process-lifecycle contracts shared by
// background workers. It deliberately has no infrastructure dependencies.
package lifecycle

// HealthSnapshot is the common lifecycle projection exposed by worker-owned
// Health methods. Readiness and domain-specific failure details belong to the
// owning component rather than this package.
type HealthSnapshot struct {
	Started bool `json:"started"`
	Stopped bool `json:"stopped"`
}

// Component is implemented by process-owned workers that expose lifecycle
// state to a composition root or management probe.
type Component interface {
	Health() HealthSnapshot
}
