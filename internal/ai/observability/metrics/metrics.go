// Package metrics provides Prometheus instrumentation for the AI gateway.
package metrics

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"xiaodou/dai/internal/ai/serving"
)

// Gateway holds all Prometheus metrics for the AI serving pipeline.
type Gateway struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	tokenUsage      *prometheus.CounterVec
	pipelineErrors  *prometheus.CounterVec
	circuitBreaker  *prometheus.GaugeVec
	routeAttempts   *prometheus.CounterVec
	routeLatency    *prometheus.HistogramVec
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

		routeAttempts: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dai_ai_route_attempts_total",
			Help: "Upstream route attempts by group policy, route binding, and outcome.",
		}, []string{"group_id", "route_id", "policy", "reason", "outcome"}),

		routeLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dai_ai_route_attempt_duration_ms",
			Help:    "Upstream route attempt latency by group policy and outcome.",
			Buckets: buckets,
		}, []string{"group_id", "policy", "outcome"}),
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

	for _, attempt := range req.Attempts {
		groupID := metricLabel(attempt.GroupID, "unknown")
		routeID := metricLabel(attempt.RouteID, "unknown")
		policy := metricLabel(attempt.RoutePolicy, "balanced")
		reason := metricLabel(attempt.SelectionReason, "unknown")
		outcome := metricLabel(attempt.Outcome.String(), "unknown")
		g.routeAttempts.WithLabelValues(groupID, routeID, policy, reason, outcome).Inc()
		if attempt.TotalMs > 0 {
			g.routeLatency.WithLabelValues(groupID, policy, outcome).Observe(float64(attempt.TotalMs))
		}
	}
}

func metricLabel(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
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

var (
	httpMetricsOnce     sync.Once
	httpInFlight        prometheus.Gauge
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
)

func initHTTPMetrics() {
	httpMetricsOnce.Do(func() {
		httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "dai_http_requests_in_flight",
			Help: "Current number of in-flight HTTP requests.",
		})
		httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dai_http_requests_total",
			Help: "Total HTTP requests by method, route template, and status.",
		}, []string{"method", "route", "status"})
		httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dai_http_request_duration_ms",
			Help:    "HTTP request latency in milliseconds by method and route template.",
			Buckets: buckets,
		}, []string{"method", "route", "status"})
	})
}

// HTTPMiddleware wraps an http.Handler with basic request instrumentation.
// The route label is resolved after the handler runs so chi has populated the
// route context. Raw URLs are never used as metric labels.
func HTTPMiddleware(next http.Handler) http.Handler {
	initHTTPMetrics()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rw, r)
		elapsed := time.Since(start).Milliseconds()

		status := strconv.Itoa(rw.status)
		route := routeTemplate(r)
		httpRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, route, status).Observe(float64(elapsed))
	})
}

func routeTemplate(r *http.Request) string {
	if r != nil {
		if ctx := chi.RouteContext(r.Context()); ctx != nil {
			if pattern := ctx.RoutePattern(); pattern != "" {
				return pattern
			}
		}
		switch r.URL.Path {
		case "/metrics", "/health", "/healthz", "/ready":
			// The management mux has no chi route context, but these are fixed
			// operational endpoints and therefore safe, low-cardinality labels.
			return r.URL.Path
		}
	}
	return "unmatched"
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if sw.wrote {
		return
	}
	sw.status = code
	sw.wrote = true
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(p []byte) (int, error) {
	if !sw.wrote {
		sw.WriteHeader(http.StatusOK)
	}
	return sw.ResponseWriter.Write(p)
}

func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

func (sw *statusWriter) Flush() {
	if !sw.wrote {
		sw.WriteHeader(http.StatusOK)
	}
	if flusher, ok := sw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (sw *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := sw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (sw *statusWriter) ReadFrom(src io.Reader) (int64, error) {
	if !sw.wrote {
		sw.WriteHeader(http.StatusOK)
	}
	if readerFrom, ok := sw.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(src)
	}
	return io.Copy(sw.ResponseWriter, src)
}

func (sw *statusWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := sw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
