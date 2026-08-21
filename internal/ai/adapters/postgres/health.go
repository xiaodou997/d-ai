package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthProbe checks the PostgreSQL connection owned by the infrastructure
// layer without exposing pgx types to transport.
type HealthProbe struct {
	pool *pgxpool.Pool
}

func NewHealthProbe(pool *pgxpool.Pool) *HealthProbe {
	return &HealthProbe{pool: pool}
}

func (p *HealthProbe) Check(ctx context.Context) error {
	if p == nil || p.pool == nil {
		return errors.New("postgres health probe is not configured")
	}
	return p.pool.Ping(ctx)
}
