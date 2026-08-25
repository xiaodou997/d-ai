package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/payment"
)

// SweepRetryStats is the low-cardinality operational projection used by the
// payment sweep metrics. It deliberately contains no order IDs or provider
// error text, so it is safe to publish on the management metrics endpoint.
type SweepRetryStats struct {
	RetryOrders        int64
	DueRetryOrders     int64
	OldestRetrySeconds float64
}

// GetSweepRetryStats reports durable provider/settlement retry backlog. The
// query is scoped to the non-terminal order states; successful settlement and
// close transitions reset sweep_attempts, so terminal rows cannot inflate the
// alert signal.
func GetSweepRetryStats(ctx context.Context, pool *pgxpool.Pool, now time.Time) (SweepRetryStats, error) {
	var stats SweepRetryStats
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE sweep_attempts > 0),
			COUNT(*) FILTER (
				WHERE sweep_attempts > 0
				  AND (sweep_next_attempt_at IS NULL OR sweep_next_attempt_at <= $1)
			),
			COALESCE(EXTRACT(EPOCH FROM (
				$1 - MIN(sweep_last_attempt_at) FILTER (WHERE sweep_attempts > 0)
			)), 0)
		FROM pay_orders
		WHERE status IN ($2, $3, $4)
	`, now, payment.OrderStatusCreated, payment.OrderStatusPaying, payment.OrderStatusExpired).Scan(
		&stats.RetryOrders, &stats.DueRetryOrders, &stats.OldestRetrySeconds,
	); err != nil {
		return SweepRetryStats{}, fmt.Errorf("read payment sweep retry stats: %w", err)
	}
	if stats.OldestRetrySeconds < 0 {
		stats.OldestRetrySeconds = 0
	}
	return stats, nil
}
