package upstreamcompat

import (
	"net/http"
	"strconv"
	"strings"
)

const ProbeBlockedCode = "upstream_probe_blocked"
const ProbeBlockedMessage = "the upstream rejected this request as a probe; check the upstream anti-probe policy or use a representative request"

// ProbeBlock reads an explicit upstream rejection signal, not a heuristic
// based on a short prompt, HTTP 200, or a plain-text greeting.
func ProbeBlock(headers http.Header) (estimatedTokens int64, blocked bool) {
	blocked, _ = strconv.ParseBool(strings.TrimSpace(headers.Get("X-Sub2api-Probe-Blocked")))
	if !blocked {
		return 0, false
	}
	estimatedTokens, err := strconv.ParseInt(strings.TrimSpace(headers.Get("X-Sub2api-Estimated-Input-Tokens")), 10, 64)
	if err != nil || estimatedTokens < 0 {
		estimatedTokens = 0
	}
	return estimatedTokens, true
}
