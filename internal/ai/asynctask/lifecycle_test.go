package asynctask

import (
	"context"
	"testing"
)

func TestEngineHealthAndStopBeforeStart(t *testing.T) {
	engine := &Engine{}
	if got := engine.Health(); got != (HealthSnapshot{}) {
		t.Fatalf("initial health = %+v, want zero state", got)
	}
	if err := engine.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() before Start error = %v", err)
	}
	if got := engine.Health(); got != (HealthSnapshot{Stopped: true}) {
		t.Fatalf("stopped health = %+v, want stopped state", got)
	}

	engine.Start(context.Background())
	if got := engine.Health(); got != (HealthSnapshot{Stopped: true}) {
		t.Fatalf("health after Start following Stop = %+v, want unchanged stopped state", got)
	}
}
