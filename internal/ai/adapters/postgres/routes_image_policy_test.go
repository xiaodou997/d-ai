package postgres

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestImagePolicyFromConfigDefaultsToMultipartEditTransport(t *testing.T) {
	t.Parallel()

	policy := imagePolicyFromConfig(nil)
	if policy.StreamMode != domain.ImageStreamModeForceSync {
		t.Fatalf("default stream mode = %q, want %q", policy.StreamMode, domain.ImageStreamModeForceSync)
	}
	if policy.EditTransport != domain.ImageEditTransportMultipart {
		t.Fatalf("default edit transport = %q, want %q", policy.EditTransport, domain.ImageEditTransportMultipart)
	}
	if policy.UpstreamResponseFormat != "" {
		t.Fatalf("default upstream response format = %q, want empty", policy.UpstreamResponseFormat)
	}
}

func TestImagePolicyFromConfigReadsEditTransport(t *testing.T) {
	t.Parallel()

	policy := imagePolicyFromConfig([]byte(`{
		"image_generation": {"stream_mode": "auto", "edit_transport": "application/json", "upstream_response_format": "url"}
	}`))
	if policy.StreamMode != domain.ImageStreamModeAuto {
		t.Fatalf("stream mode = %q, want %q", policy.StreamMode, domain.ImageStreamModeAuto)
	}
	if policy.EditTransport != domain.ImageEditTransportJSON {
		t.Fatalf("edit transport = %q, want %q", policy.EditTransport, domain.ImageEditTransportJSON)
	}
	if policy.UpstreamResponseFormat != domain.ImageResponseFormatURL {
		t.Fatalf("upstream response format = %q, want %q", policy.UpstreamResponseFormat, domain.ImageResponseFormatURL)
	}
}
