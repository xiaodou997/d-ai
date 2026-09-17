package upstreamcompat

import (
	"net/http"
	"testing"
)

func TestProbeBlockBoundsUntrustedTokenHeader(t *testing.T) {
	for _, value := range []string{"-1", "not-a-number", "999999999999999999999999", "12\r\nsecret"} {
		headers := http.Header{}
		headers.Set("X-Sub2api-Probe-Blocked", "true")
		headers.Set("X-Sub2api-Estimated-Input-Tokens", value)
		if count, blocked := ProbeBlock(headers); !blocked || count != 0 {
			t.Fatalf("count=%d blocked=%v", count, blocked)
		}
	}
}
