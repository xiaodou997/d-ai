package auth

import "testing"

func TestBanReconcilerLifecycleIsIdempotent(t *testing.T) {
	r := NewBanReconciler(nil, nil, nil, 0)
	r.Start()
	r.Start()
	r.Stop()
	r.Stop()
}

func TestBanReconcilerCannotStartAfterStop(t *testing.T) {
	r := NewBanReconciler(nil, nil, nil, 0)
	r.Stop()
	r.Start()
}
