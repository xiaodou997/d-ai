package service

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	billingdomain "xiaodou/dai/internal/billing"
	paymentpg "xiaodou/dai/internal/payment/pg"
)

var (
	paymentSweepRetryOrders = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_payment_sweep_retry_orders",
		Help: "Payment orders with a durable provider or settlement retry pending.",
	})
	paymentSweepDueRetryOrders = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_payment_sweep_due_retry_orders",
		Help: "Payment retry orders whose next attempt is due now.",
	})
	paymentSweepOldestRetrySeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_payment_sweep_oldest_retry_seconds",
		Help: "Age of the oldest payment sweep failure awaiting recovery.",
	})
	paymentSweepStatsErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dai_payment_sweep_stats_errors_total",
		Help: "Failures while reading durable payment sweep retry health.",
	})
)

func (s *PaymentService) publishSweepRetryHealth(ctx context.Context) error {
	stats, err := paymentpg.GetSweepRetryStats(ctx, s.pool, billingdomain.NowUTC())
	if err != nil {
		paymentSweepStatsErrors.Inc()
		return err
	}
	paymentSweepRetryOrders.Set(float64(stats.RetryOrders))
	paymentSweepDueRetryOrders.Set(float64(stats.DueRetryOrders))
	paymentSweepOldestRetrySeconds.Set(stats.OldestRetrySeconds)
	return nil
}
