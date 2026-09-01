package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"xiaodou/dai/internal/ai/serving"
)

func TestHTTPMiddlewareUsesRouteTemplate(t *testing.T) {
	router := chi.NewRouter()
	router.Use(HTTPMiddleware)
	router.Get("/users/{userID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	metricsResponse := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(metricsResponse.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	want := `dai_http_requests_total{method="GET",route="/users/{userID}",status="204"}`
	if !strings.Contains(string(body), want) {
		t.Fatalf("metrics did not contain route template %q:\n%s", want, body)
	}
}

func TestGatewayRecordsRoutePolicyMetrics(t *testing.T) {
	gateway := NewGateway()
	gateway.RecordRequest(&serving.Request{
		ModelCode:      "model-1",
		CapabilityType: "chat",
		RequestStatus:  "success",
		Attempts: []serving.AttemptRecord{{
			RouteID:         "route-1",
			GroupID:         "group-1",
			RoutePolicy:     "latency",
			SelectionReason: "latency",
			Outcome:         serving.ResultSuccess,
			TotalMs:         25,
		}},
	})

	metricsResponse := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(metricsResponse.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `dai_ai_route_attempts_total{group_id="group-1",outcome="success",policy="latency",reason="latency",route_id="route-1"}`) {
		t.Fatalf("route attempt metric missing:\n%s", text)
	}
	if !strings.Contains(text, `dai_ai_route_attempt_duration_ms_count{group_id="group-1",outcome="success",policy="latency"} 1`) {
		t.Fatalf("route latency metric missing:\n%s", text)
	}
}

func TestDBPoolCollectorCanBeRegisteredWithoutPools(t *testing.T) {
	// Registering the collector is intentionally safe before infrastructure is
	// ready. The process will start publishing pool samples once the composition
	// root supplies live pools.
	RegisterDBPools(nil, nil)
	if _, err := prometheus.DefaultGatherer.Gather(); err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
}
