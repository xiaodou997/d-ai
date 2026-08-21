package redis

import (
	"context"
	"errors"

	goredis "github.com/redis/go-redis/v9"
)

// HealthProbe checks the Redis connection owned by the infrastructure layer.
type HealthProbe struct {
	client *goredis.Client
}

func NewHealthProbe(client *goredis.Client) *HealthProbe {
	return &HealthProbe{client: client}
}

func (p *HealthProbe) Check(ctx context.Context) error {
	if p == nil || p.client == nil {
		return errors.New("redis health probe is not configured")
	}
	return p.client.Ping(ctx).Err()
}
