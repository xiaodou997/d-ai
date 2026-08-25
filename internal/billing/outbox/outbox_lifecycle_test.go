package outbox

import (
	"context"
	"testing"
)

func TestConsumerLifecycleIsIdempotent(t *testing.T) {
	c := NewConsumer(nil, nil)
	c.Stop(context.Background())
	c.Run(context.Background())
	c.Stop(context.Background())
}
