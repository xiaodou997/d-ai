package tokenrefresh

import (
	"context"
	"testing"
)

func TestRefresherLifecycleIsIdempotent(t *testing.T) {
	r := New(nil, nil)
	r.Stop(context.Background())
	if got := r.Health(); !got.Stopped || got.Started {
		t.Fatalf("stopped refresher health = %+v", got)
	}
	r.Start(context.Background())
	r.Stop(context.Background())
	if got := r.Health(); !got.Stopped || got.Started {
		t.Fatalf("refresher health after stop-before-start = %+v", got)
	}
}
