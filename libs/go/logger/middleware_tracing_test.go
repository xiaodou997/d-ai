package logger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestChiRequestLoggerExtractsTraceContextAndLogsTraceID(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	parentCtx, parent := provider.Tracer("test").Start(context.Background(), "parent")
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil).WithContext(parentCtx)
	propagation.TraceContext{}.Inject(parentCtx, propagation.HeaderCarrier(req.Header))

	core, observed := observer.New(zapcore.InfoLevel)
	router := chi.NewRouter()
	router.Get("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := ChiRequestLogger(zap.New(core))(router)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	parent.End()

	var accessLog observer.LoggedEntry
	found := false
	for _, entry := range observed.AllUntimed() {
		if entry.Message == "HTTP Request" {
			accessLog = entry
			found = true
		}
	}
	if !found {
		t.Fatal("missing HTTP Request log")
	}
	traceID, ok := accessLog.ContextMap()["trace_id"].(string)
	if !ok || traceID == "" {
		t.Fatalf("trace_id missing from request log: %#v", accessLog.ContextMap())
	}
	if got, want := traceID, parent.SpanContext().TraceID().String(); got != want {
		t.Fatalf("trace_id = %q, want parent trace %q", got, want)
	}

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("exported spans = %d, want parent and request span", len(spans))
	}
	foundServerSpan := false
	for _, span := range spans {
		if span.SpanKind == trace.SpanKindServer {
			foundServerSpan = true
			break
		}
	}
	if !foundServerSpan {
		t.Fatalf("exported spans did not contain a server span: %#v", spans)
	}
}
