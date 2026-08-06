// Package metrics provides Prometheus instrumentation for the AI gateway.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"xiaodou/dai/internal/ai/billingledger"
	"xiaodou/dai/internal/ai/serving"
)

// Gateway holds all Prometheus metrics for the AI serving pipeline.
type Gateway struct {
	requestsTotal            *prometheus.CounterVec
	requestDuration          *prometheus.HistogramVec
	tokenUsage               *prometheus.CounterVec
	pipelineErrors           *prometheus.CounterVec
	circuitBreaker           *prometheus.GaugeVec
	billingAdmissionFailures *prometheus.CounterVec
	billingLeaseOperations   *prometheus.CounterVec
	billingSettlements       *prometheus.CounterVec
	billingWindows           *prometheus.GaugeVec
	billingOutboxPending     prometheus.Gauge
	billingOutboxOldest      prometheus.Gauge
	billingReconciliations   prometheus.Gauge
}

var buckets = []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

// NewGateway registers all metrics with the default Prometheus registry.
func NewGateway() *Gateway {
	return &Gateway{
		requestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dai_ai_requests_total",
			Help: "Total number of AI requests processed, by model, status, and capability.",
		}, []string{"model", "capability", "status", "provider"}),

		requestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dai_ai_request_duration_ms",
			Help:    "End-to-end request latency in milliseconds.",
			Buckets: buckets,
		}, []string{"model", "capability", "provider"}),

		tokenUsage: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dai_ai_tokens_total",
			Help: "Token counts by model, provider, and token type.",
		}, []string{"model", "provider", "token_type"}),

		pipelineErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dai_ai_pipeline_errors_total",
			Help: "Pipeline step errors by step name and error code.",
		}, []string{"step", "code"}),

		circuitBreaker: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "dai_ai_circuit_breaker_open",
			Help: "1 if the circuit breaker is open for a deployment, 0 otherwise.",
		}, []string{"deployment_id"}),
		billingAdmissionFailures: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_billing_admission_failures_total",
			Help: "Billing admissions rejected before any upstream attempt.",
		}, []string{"reason"}),
		billingLeaseOperations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_billing_lease_operations_total",
			Help: "Credit lease acquire and renew outcomes.",
		}, []string{"operation", "result"}),
		billingSettlements: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_billing_settlement_dispatch_total",
			Help: "Durable settlement outbox dispatch outcomes.",
		}, []string{"result"}),
		billingWindows: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ai_billing_windows",
			Help: "Current durable billing windows by state.",
		}, []string{"state"}),
		billingOutboxPending: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ai_billing_outbox_pending",
			Help: "Settlement outbox records pending delivery.",
		}),
		billingOutboxOldest: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ai_billing_outbox_oldest_age_seconds",
			Help: "Age in seconds of the oldest undelivered settlement.",
		}),
		billingReconciliations: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ai_billing_reconciling_admissions",
			Help: "Request admissions awaiting operator reconciliation.",
		}),
	}
}

func (g *Gateway) BillingAdmissionFailure(reason string) {
	g.billingAdmissionFailures.WithLabelValues(reason).Inc()
}

func (g *Gateway) BillingLeaseOperation(operation, result string) {
	g.billingLeaseOperations.WithLabelValues(operation, result).Inc()
}

func (g *Gateway) BillingSettlementDispatch(result string) {
	g.billingSettlements.WithLabelValues(result).Inc()
}

func (g *Gateway) SetBillingSnapshot(snapshot billingledger.BillingSnapshot) {
	g.billingWindows.WithLabelValues("opening").Set(float64(snapshot.OpeningWindows))
	g.billingWindows.WithLabelValues("active").Set(float64(snapshot.ActiveWindows))
	g.billingWindows.WithLabelValues("draining").Set(float64(snapshot.DrainingWindows))
	g.billingWindows.WithLabelValues("reconciling").Set(float64(snapshot.ReconcilingWindows))
	g.billingWindows.WithLabelValues("settlement_pending").Set(float64(snapshot.SettlementPending))
	g.billingOutboxPending.Set(float64(snapshot.PendingOutbox))
	g.billingOutboxOldest.Set(snapshot.OldestOutboxAgeSeconds)
	g.billingReconciliations.Set(float64(snapshot.ReconcilingAdmissions))
}

// Handler returns the Prometheus HTTP handler for /metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}

// RecordRequest records a completed pipeline request.
func (g *Gateway) RecordRequest(req *serving.Request) {
	if req == nil {
		return
	}

	model := req.ModelCode
	capability := string(req.CapabilityType)
	status := string(req.RequestStatus)
	provider := ""
	if req.Candidate != nil {
		provider = req.Candidate.ProviderCode
	}

	g.requestsTotal.WithLabelValues(model, capability, status, provider).Inc()

	if totalMs, ok := req.RequestTotalMs(); ok {
		g.requestDuration.WithLabelValues(model, capability, provider).Observe(float64(totalMs))
	}

	usage := req.TokenUsage
	if usage.PromptTokens > 0 {
		g.tokenUsage.WithLabelValues(model, provider, "prompt").Add(float64(usage.PromptTokens))
	}
	if usage.CompletionTokens > 0 {
		g.tokenUsage.WithLabelValues(model, provider, "completion").Add(float64(usage.CompletionTokens))
	}
	if usage.CacheWriteTokens > 0 {
		g.tokenUsage.WithLabelValues(model, provider, "cache_write").Add(float64(usage.CacheWriteTokens))
	}
	if usage.CacheReadTokens > 0 {
		g.tokenUsage.WithLabelValues(model, provider, "cache_read").Add(float64(usage.CacheReadTokens))
	}
	if usage.ReasoningTokens > 0 {
		g.tokenUsage.WithLabelValues(model, provider, "reasoning").Add(float64(usage.ReasoningTokens))
	}
}

// RecordPipelineError records a step failure with its error code.
func (g *Gateway) RecordPipelineError(step, code string) {
	g.pipelineErrors.WithLabelValues(step, code).Inc()
}

// SetCircuitBreakerOpen sets the circuit breaker gauge for a deployment.
func (g *Gateway) SetCircuitBreakerOpen(deploymentID string, open bool) {
	v := 0.0
	if open {
		v = 1.0
	}
	g.circuitBreaker.WithLabelValues(deploymentID).Set(v)
}

// ============================================================================
// HTTP middleware — instruments every HTTP request
// ============================================================================

// HTTPMiddleware wraps an http.Handler with basic request instrumentation.
func HTTPMiddleware(next http.Handler) http.Handler {
	inFlight := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_http_requests_in_flight",
		Help: "Current number of in-flight HTTP requests.",
	})
	total := promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dai_http_requests_total",
		Help: "Total HTTP requests by method, path pattern, and status.",
	}, []string{"method", "status"})
	duration := promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dai_http_request_duration_ms",
		Help:    "HTTP request latency in milliseconds.",
		Buckets: buckets,
	}, []string{"method", "status"})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inFlight.Inc()
		defer inFlight.Dec()

		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rw, r)
		elapsed := time.Since(start).Milliseconds()

		status := strconv.Itoa(rw.status)
		total.WithLabelValues(r.Method, status).Inc()
		duration.WithLabelValues(r.Method, status).Observe(float64(elapsed))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
