package formats

import (
	"strings"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

func TestStreamErrorFrame(t *testing.T) {
	tests := []struct {
		proto      domain.UpstreamProtocol
		wantPrefix string
		wantInside string
	}{
		{domain.ProtocolOpenAIChat, "data: ", `"error"`},
		{domain.ProtocolOpenAIResponses, "data: ", `"error"`},
		{domain.ProtocolAnthropicMessages, "event: error\ndata: ", `"type":"error"`},
		{domain.ProtocolGeminiGenerate, "data: ", `"error"`},
	}
	for _, tc := range tests {
		frame := string(StreamErrorFrame(tc.proto, "stream_idle_timeout", "boom message"))
		if !strings.HasPrefix(frame, tc.wantPrefix) {
			t.Errorf("%s: frame = %q, want prefix %q", tc.proto, frame, tc.wantPrefix)
		}
		if !strings.Contains(frame, tc.wantInside) {
			t.Errorf("%s: frame %q missing %q", tc.proto, frame, tc.wantInside)
		}
		if !strings.Contains(frame, "boom message") {
			t.Errorf("%s: frame must carry the error message", tc.proto)
		}
		if !strings.HasSuffix(frame, "\n\n") {
			t.Errorf("%s: frame must end with a blank line, got %q", tc.proto, frame)
		}
	}
}
