package auth

import "testing"

func TestBanReconcilerLifecycleIsIdempotent(t *testing.T) {
	r := NewBanReconciler(nil, nil, nil, 0)
	r.Start()
	r.Start()
	if got := r.Health(); got.Started || got.Stopped {
		t.Fatalf("unconfigured reconciler health = %+v, want not started", got)
	}
	r.Stop()
	r.Stop()
	if got := r.Health(); !got.Stopped {
		t.Fatalf("stopped reconciler health = %+v, want stopped", got)
	}
}

func TestBanReconcilerCannotStartAfterStop(t *testing.T) {
	r := NewBanReconciler(nil, nil, nil, 0)
	r.Stop()
	r.Start()
}
