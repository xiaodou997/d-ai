package gateway

import (
	"net/http"
	"testing"
)

func TestExtractRuntimeAPIKeySupportsBearerAndAnthropicHeaders(t *testing.T) {
	const key = "sk-ai-test-key"
	tests := []struct {
		name    string
		headers http.Header
		want    string
		wantErr bool
	}{
		{
			name:    "bearer only",
			headers: http.Header{"Authorization": {"Bearer " + key}},
			want:    key,
		},
		{
			name:    "anthropic header only",
			headers: http.Header{"X-Api-Key": {key}},
			want:    key,
		},
		{
			name: "matching headers",
			headers: http.Header{
				"Authorization": {"Bearer " + key},
				"X-Api-Key":     {key},
			},
			want: key,
		},
		{
			name: "mismatched headers",
			headers: http.Header{
				"Authorization": {"Bearer " + key},
				"X-Api-Key":     {"sk-ai-another-key"},
			},
			wantErr: true,
		},
		{
			name: "malformed bearer does not downgrade",
			headers: http.Header{
				"Authorization": {"Basic " + key},
				"X-Api-Key":     {key},
			},
			wantErr: true,
		},
		{
			name:    "missing headers",
			headers: http.Header{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractRuntimeAPIKey(tt.headers)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("extractRuntimeAPIKey() = %q, want error", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("extractRuntimeAPIKey() = %q, %v; want %q, nil", got, err, tt.want)
			}
		})
	}
}
