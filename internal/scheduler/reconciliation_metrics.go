package scheduler

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"xiaodou/dai/internal/billing/invariants"
)

func publishBillingReconciliationMetrics(report invariants.Report, now time.Time) {
	timestamp := float64(now.UTC().Unix())
	billingReconciliationLastRun.Set(timestamp)
	billingReconciliationViolations.Set(float64(len(report.Violations)))
	billingReconciliationViolationKinds.Reset()
	for _, violation := range report.Violations {
		billingReconciliationViolationKinds.WithLabelValues(violation.Invariant).Inc()
	}
	if report.Healthy() {
		billingReconciliationLastHealthy.Set(timestamp)
	}
}

var (
	billingReconciliationViolations = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_billing_reconciliation_violations",
		Help: "Current number of money invariant violations found by the latest billing reconciliation.",
	})
	billingReconciliationViolationKinds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dai_billing_reconciliation_violation_kinds",
		Help: "Current billing invariant violations grouped by bounded invariant name.",
	}, []string{"invariant"})
	billingReconciliationLastRun = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_billing_reconciliation_last_run_timestamp_seconds",
		Help: "Unix timestamp of the latest completed billing reconciliation snapshot, healthy or not.",
	})
	billingReconciliationLastHealthy = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_billing_reconciliation_last_healthy_timestamp_seconds",
		Help: "Unix timestamp of the latest billing reconciliation snapshot with no invariant violations.",
	})
)
