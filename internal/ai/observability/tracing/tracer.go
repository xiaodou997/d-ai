// Package tracing initialises the OpenTelemetry TracerProvider.
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset the provider is a no-op and adds
// zero overhead. When the env var is set, traces are exported via OTLP/HTTP.
package tracing

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
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

const ServiceName = "dai"

const defaultSamplingRatio = 0.1

// Options controls the OTLP exporter and trace sampling policy. Zero values
// are filled from the standard OTEL_TRACES_* environment variables.
type Options struct {
	Endpoint   string
	Insecure   bool
	Sampler    string
	SamplerArg string
}

// Init sets up the global TracerProvider and returns a shutdown function.
// If OTEL_EXPORTER_OTLP_ENDPOINT is empty, a no-op provider is installed.

func Init(ctx context.Context, options ...Options) (shutdown func(context.Context) error) {
	opts := optionsFromEnv()
	if len(options) > 0 {
		if options[0].Endpoint != "" {
			opts.Endpoint = options[0].Endpoint
		}
		if options[0].Sampler != "" {
			opts.Sampler = options[0].Sampler
		}
		if options[0].SamplerArg != "" {
			opts.SamplerArg = options[0].SamplerArg
		}
		opts.Insecure = options[0].Insecure
	}

	// Propagation is useful even when exporting is disabled: an incoming trace
	// ID must still be available to request spans and structured logs.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }
	}

	exporterOptions := []otlptracehttp.Option{endpointOption(endpoint)}
	if opts.Insecure {
		exporterOptions = append(exporterOptions, otlptracehttp.WithInsecure())
	}
	exp, err := otlptracehttp.New(ctx, exporterOptions...)
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
		sdktrace.WithSampler(samplerFromOptions(opts)),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown
}

func optionsFromEnv() Options {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	insecureValue := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE"))
	if insecureValue == "" {
		insecureValue = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"))
	}
	insecure, _ := strconv.ParseBool(insecureValue)
	return Options{
		Endpoint:   endpoint,
		Insecure:   insecure,
		Sampler:    os.Getenv("OTEL_TRACES_SAMPLER"),
		SamplerArg: os.Getenv("OTEL_TRACES_SAMPLER_ARG"),
	}
}

func endpointOption(endpoint string) otlptracehttp.Option {
	if strings.Contains(endpoint, "://") {
		return otlptracehttp.WithEndpointURL(endpoint)
	}
	return otlptracehttp.WithEndpoint(endpoint)
}

func samplerFromOptions(opts Options) sdktrace.Sampler {
	name := strings.ToLower(strings.TrimSpace(opts.Sampler))
	arg := defaultSamplingRatio
	if value := strings.TrimSpace(opts.SamplerArg); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 && parsed <= 1 {
			arg = parsed
		}
	}

	ratio := sdktrace.TraceIDRatioBased(arg)
	switch name {
	case "always_on", "alwayson":
		return sdktrace.AlwaysSample()
	case "always_off", "alwaysoff":
		return sdktrace.NeverSample()
	case "traceidratio", "trace_id_ratio":
		return ratio
	case "parentbased_always_on", "parentbased_alwayson":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off", "parentbased_alwaysoff":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio", "parentbased_trace_id_ratio", "":
		return sdktrace.ParentBased(ratio)
	default:
		// Invalid deployment values must not silently turn tracing off. Keep the
		// safe, bounded parent-based default and let the operator see the value in
		// the startup configuration validation/logging layer.
		return sdktrace.ParentBased(ratio)
	}
}

// ValidateOptions is used by configuration/tests to reject malformed sampling
// settings before a production process starts exporting traces.
func ValidateOptions(options Options) error {
	name := strings.ToLower(strings.TrimSpace(options.Sampler))
	valid := map[string]bool{
		"": true, "always_on": true, "alwayson": true, "always_off": true,
		"alwaysoff": true, "traceidratio": true, "trace_id_ratio": true,
		"parentbased_always_on": true, "parentbased_alwayson": true,
		"parentbased_always_off": true, "parentbased_alwaysoff": true,
		"parentbased_traceidratio": true, "parentbased_trace_id_ratio": true,
	}
	if !valid[name] {
		return fmt.Errorf("unsupported OTEL_TRACES_SAMPLER %q", options.Sampler)
	}
	if value := strings.TrimSpace(options.SamplerArg); value != "" {
		ratio, err := strconv.ParseFloat(value, 64)
		if err != nil || ratio < 0 || ratio > 1 {
			return fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be a number between 0 and 1")
		}
	}
	return nil
}

// Tracer returns a Tracer scoped to the given instrumentation library name.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
