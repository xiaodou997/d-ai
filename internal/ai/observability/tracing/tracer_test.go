package tracing

import (
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestValidateOptions(t *testing.T) {
	valid := []Options{
		{},
		{Sampler: "parentbased_traceidratio", SamplerArg: "0.25"},
		{Sampler: "always_on"},
		{Sampler: "always_off"},
	}
	for _, options := range valid {
		if err := ValidateOptions(options); err != nil {
			t.Errorf("ValidateOptions(%+v): %v", options, err)
		}
	}
	for _, options := range []Options{
		{Sampler: "unknown"},
		{Sampler: "parentbased_traceidratio", SamplerArg: "2"},
		{Sampler: "parentbased_traceidratio", SamplerArg: "not-a-number"},
	} {
		if err := ValidateOptions(options); err == nil {
			t.Errorf("ValidateOptions(%+v) succeeded, want error", options)
		}
	}
}

func TestSamplerDefaultsToParentBasedRatio(t *testing.T) {
	sampler := samplerFromOptions(Options{})
	if sampler == nil {
		t.Fatal("sampler is nil")
	}
	// The SDK sampler string includes the parent-based wrapper and ratio. This
	// protects the production default from regressing to AlwaysSample.
	if got := sampler.Description(); got == sdktrace.AlwaysSample().Description() {
		t.Fatalf("default sampler = %q, unexpectedly always-on", got)
	}
}

func TestOptionsFromEnvPrefersTraceSpecificEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "collector.example:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://traces.example/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE", "false")

	opts := optionsFromEnv()
	if opts.Endpoint != "https://traces.example/v1/traces" {
		t.Fatalf("endpoint = %q, want trace-specific endpoint", opts.Endpoint)
	}
	if opts.Insecure {
		t.Fatal("trace-specific insecure=false was not respected")
	}
}
