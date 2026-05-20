package blobstore

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const partitionAheadMonths = 2

// Partitioner ensures ai_request_payloads monthly partitions exist for the
// current month and the next partitionAheadMonths months.
// Call EnsurePartitions on startup and let Run tick it daily.
type Partitioner struct {
	pool *pgxpool.Pool
}

// NewPartitioner creates a Partitioner backed by pool.
func NewPartitioner(pool *pgxpool.Pool) *Partitioner {
	return &Partitioner{pool: pool}
}

// EnsurePartitions creates any missing monthly partitions within the window.
// Existing partitions are left untouched.
func (p *Partitioner) EnsurePartitions(ctx context.Context) error {
	now := time.Now().UTC()
	for i := 0; i <= partitionAheadMonths; i++ {
		m := addMonths(now, i)
		if err := p.ensureOne(ctx, m); err != nil {
			return fmt.Errorf("ensure partition for %s: %w", m.Format("2006-01"), err)
		}
	}
	return nil
}

// Run blocks and ticks EnsurePartitions once per day until ctx is cancelled.
// Call EnsurePartitions separately before Run if you want an immediate check
// on startup.
func (p *Partitioner) Run(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := p.EnsurePartitions(ctx); err != nil {
				zap.L().Error("blobstore: partition maintenance failed", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *Partitioner) ensureOne(ctx context.Context, month time.Time) error {
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := addMonths(start, 1)
	name := "ai_request_payloads_" + start.Format("2006_01")

	// Check existence first to avoid DDL lock when partition already exists.
	var exists bool
	if err := p.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`, name,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Table name and timestamps are derived from time.Time arithmetic (no user
	// input), so inline embedding is safe here.
	ddl := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS "%s" PARTITION OF ai_request_payloads `+
			`FOR VALUES FROM ('%s') TO ('%s')`,
		name,
		start.Format("2006-01-02 15:04:05 UTC"),
		end.Format("2006-01-02 15:04:05 UTC"),
	)
	_, err := p.pool.Exec(ctx, ddl)
	return err
}

// addMonths adds n calendar months to t, normalising to the first of the month.
func addMonths(t time.Time, n int) time.Time {
	return time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
}
