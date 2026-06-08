// Package metrics provides Prometheus instrumentation for the AI gateway.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"xiaodou/unihub/ai-service/internal/serving"
)

// Gateway holds all Prometheus metrics for the AI serving pipeline.
type Gateway struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	tokenUsage      *prometheus.CounterVec
	pipelineErrors  *prometheus.CounterVec
	circuitBreaker  *prometheus.GaugeVec
}

var buckets = []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

// NewGateway registers all metrics with the default Prometheus registry.
func NewGateway() *Gateway {
	return &Gateway{
		requestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_gateway_requests_total",
			Help: "Total number of AI requests processed, by model, status, and capability.",
		}, []string{"model", "capability", "status", "provider"}),

		requestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ai_gateway_request_duration_ms",
			Help:    "End-to-end request latency in milliseconds.",
			Buckets: buckets,
		}, []string{"model", "capability", "provider"}),

		tokenUsage: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_gateway_tokens_total",
			Help: "Token counts by model, provider, and token type.",
		}, []string{"model", "provider", "token_type"}),

		pipelineErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_gateway_pipeline_errors_total",
			Help: "Pipeline step errors by step name and error code.",
		}, []string{"step", "code"}),

		circuitBreaker: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ai_gateway_circuit_breaker_open",
			Help: "1 if the circuit breaker is open for a deployment, 0 otherwise.",
		}, []string{"deployment_id"}),
	}
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

	if req.LatencyMs > 0 {
		g.requestDuration.WithLabelValues(model, capability, provider).Observe(float64(req.LatencyMs))
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
		Name: "ai_gateway_http_requests_in_flight",
		Help: "Current number of in-flight HTTP requests.",
	})
	total := promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_gateway_http_requests_total",
		Help: "Total HTTP requests by method, path pattern, and status.",
	}, []string{"method", "status"})
	duration := promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_gateway_http_request_duration_ms",
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
