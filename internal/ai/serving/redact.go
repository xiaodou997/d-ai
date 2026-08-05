package serving

import "regexp"

// redactSecretPatterns is a best-effort last line of defense before
// persisting InternalErrorDetail. That field is admin-only and intentionally
// unsanitized (unlike ErrorMessage, which goes through egress.SanitizeText
// for vendor/model-identity hiding), but it must never contain a literal
// upstream API key, bearer token, or password lifted verbatim from an error
// string or raw upstream response body.
var redactSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`AIza[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`(?i)("?(?:api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|secret|password)"?\s*[:=]\s*"?)[A-Za-z0-9._\-]{6,}`),
}

// internalErrorDetailMaxLen caps InternalErrorDetail well above the old
// 1024-byte client-facing summary so admins can see full upstream bodies /
// error chains, without letting a pathological error balloon the audit
// worker's byte budget or the DB row.
const internalErrorDetailMaxLen = 32 * 1024

// RedactInternalErrorDetail masks likely credentials in s and caps its length.
// Exported so other packages that populate admin-only diagnostics outside the
// main runtime pipeline (e.g. console image async tasks) can reuse the same
// last-line-of-defense redaction instead of duplicating the pattern list.
func RedactInternalErrorDetail(s string) string {
	if s == "" {
		return s
	}
	for _, re := range redactSecretPatterns {
		s = re.ReplaceAllString(s, "${1}[REDACTED]")
	}
	if len(s) > internalErrorDetailMaxLen {
		s = s[:internalErrorDetailMaxLen] + "...[truncated]"
	}
	return s
}
