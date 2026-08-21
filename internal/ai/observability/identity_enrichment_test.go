package observability

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestIdentityEnrichmentLoggerRecordsFailure(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	recorder := NewIdentityEnrichmentLogger(zap.New(core))

	recorder.ObserveFailure("users", errors.New("identity unavailable"))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].Message != "identity enrichment failed open" {
		t.Fatalf("message = %q", entries[0].Message)
	}
	fields := entries[0].ContextMap()
	if fields["kind"] != "users" || fields["error"] != "identity unavailable" {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestIdentityEnrichmentLoggerAllowsMissingLogger(t *testing.T) {
	NewIdentityEnrichmentLogger(nil).ObserveFailure("users", errors.New("ignored"))
}
