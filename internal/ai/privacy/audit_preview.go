package privacy

import (
	"regexp"
	"strings"
)

var (
	auditBearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+\-/]+=*`)
	auditSecretPattern = regexp.MustCompile(`(?i)\b(sk|rk|pk|api[_-]?key|token|secret|password)[-_:=\s]+[A-Za-z0-9._~+\-/]{8,}`)
	auditEmailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	auditPhonePattern  = regexp.MustCompile(`(?:\+?\d[\d\s().-]{8,}\d)`)
)

// AuditPreview creates a deliberately non-recoverable preview for persistent
// security metadata. It never returns a short prompt verbatim.
func AuditPreview(value string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 96
	}
	value = auditBearerPattern.ReplaceAllString(value, "Bearer ***")
	value = auditSecretPattern.ReplaceAllString(value, "***")
	value = auditEmailPattern.ReplaceAllString(value, "***@***")
	value = auditPhonePattern.ReplaceAllString(value, "***PHONE***")
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	if len(runes) == 0 {
		return ""
	}
	if len(runes) < 32 {
		return "***"
	}
	keep := len(runes) / 4
	if keep > 24 {
		keep = 24
	}
	return string(runes[:keep]) + "***…"
}
