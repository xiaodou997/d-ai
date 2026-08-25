package tokenrefresh

import (
	"context"
	"testing"
)

func TestRefresherLifecycleIsIdempotent(t *testing.T) {
	r := New(nil, nil)
	r.Stop(context.Background())
	r.Start(context.Background())
	r.Stop(context.Background())
}
