package serving

import (
	"testing"
	"xiaodou/dai/internal/ai/domain"
)

func TestCompletionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name            string
		req             Request
		charge, failure bool
	}{
		{"reported", Request{UsageEvidence: domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10}}}, true, false},
		{"missing", Request{}, false, false},
		{"zero", Request{UsageEvidence: domain.UsageEvidence{Fields: map[string]int{"input_tokens": 0}}}, true, false},
		{"disconnect", Request{ErrorCode: "client_disconnected", UsageEvidence: domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10}}}, true, false},
		{"error_reported", Request{ProviderTerminalState: domain.ProviderTerminalFailed, UsageEvidence: domain.UsageEvidence{Fields: map[string]int{"input_tokens": 10}}}, false, true},
		{"error_then_disconnect", Request{ProviderTerminalState: domain.ProviderTerminalFailed, ErrorCode: "client_disconnected", CancellationOrigin: domain.CancellationClient}, false, true},
		{"timeout", Request{ProviderTerminalState: domain.ProviderTerminalIncomplete, ErrorCode: "stream_idle_timeout"}, false, true},
		{"auth", Request{HTTPStatus: 401, ErrorCode: "invalid_api_key"}, false, true},
		{"legacy_failed_is_not_evidence", Request{RequestStatus: domain.RequestFailed}, false, false},
		{"requested_image_is_not_evidence", Request{CapabilityType: domain.CapabilityImage, TokenUsage: domain.TokenUsage{ImageCount: 2}}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := DecideCompletion(&tc.req)
			if d.Billable != tc.charge || (d.Error != nil) != tc.failure {
				t.Fatalf("decision=%+v", d)
			}
		})
	}
}
