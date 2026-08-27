package outbox

import (
	"context"
	"testing"
)

func TestConsumerLifecycleIsIdempotent(t *testing.T) {
	c := NewConsumer(nil, nil)
	c.Stop(context.Background())
	if got := c.Health(); !got.Stopped || got.Started {
		t.Fatalf("stopped consumer health = %+v", got)
	}
	c.Run(context.Background())
	c.Stop(context.Background())
	if got := c.Health(); !got.Stopped || got.Started {
		t.Fatalf("consumer health after stop-before-start = %+v", got)
	}
}
