package transport

import (
	"context"
	"errors"
	"testing"
)

func TestComponentProbeStatus(t *testing.T) {
	tests := []struct {
		name       string
		probe      ComponentHealthProbe
		wantStatus string
		wantError  string
	}{
		{name: "disabled", wantStatus: "disabled"},
		{name: "healthy", probe: componentHealthProbeStub{}, wantStatus: "ok"},
		{name: "unhealthy", probe: componentHealthProbeStub{err: errors.New("redis unavailable")}, wantStatus: "error", wantError: "redis unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := componentProbeStatus(t.Context(), tt.probe)
			if got.Status != tt.wantStatus || got.Error != tt.wantError {
				t.Fatalf("status = %#v, want status %q error %q", got, tt.wantStatus, tt.wantError)
			}
		})
	}
}

type componentHealthProbeStub struct {
	err error
}

func (s componentHealthProbeStub) Check(context.Context) error { return s.err }
