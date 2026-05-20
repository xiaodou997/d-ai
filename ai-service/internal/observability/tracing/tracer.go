// Package tracing initialises the OpenTelemetry TracerProvider.
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset the provider is a no-op and adds
// zero overhead. When the env var is set, traces are exported via OTLP/HTTP.
package tracing

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const ServiceName = "uni-ai-api"

// Init sets up the global TracerProvider and returns a shutdown function.
// If OTEL_EXPORTER_OTLP_ENDPOINT is empty, a no-op provider is installed.
func Init(ctx context.Context) (shutdown func(context.Context) error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }
	}

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		// Fall back to noop on exporter init failure rather than crashing.
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(ServiceName)),
		resource.WithProcess(),
		resource.WithOS(),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown
}

// Tracer returns a Tracer scoped to the given instrumentation library name.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
