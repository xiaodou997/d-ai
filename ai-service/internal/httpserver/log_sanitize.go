package httpserver

import "strings"

const upstreamErrorBodyMaxBytes = 2048

func upstreamErrorBodyForLog(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) > upstreamErrorBodyMaxBytes {
		body = body[:upstreamErrorBodyMaxBytes]
	}
	return redactLogText(string(body))
}

func redactLogText(value string) string {
	replacements := []string{
		"authorization", "api_key", "apiKey", "provider_key", "providerKey",
		"appSecret", "password", "token", "secret",
	}
	redacted := value
	for _, key := range replacements {
		redacted = redactJSONLikeValue(redacted, key)
	}
	return redacted
}

func redactJSONLikeValue(value string, key string) string {
	for _, quote := range []string{`"`, `'`} {
		pattern := quote + key + quote
		idx := strings.Index(value, pattern)
		for idx >= 0 {
			colon := strings.Index(value[idx+len(pattern):], ":")
			if colon < 0 {
				break
			}
			start := idx + len(pattern) + colon + 1
			for start < len(value) && (value[start] == ' ' || value[start] == '\t') {
				start++
			}
			if start >= len(value) {
				break
			}
			end := start
			if value[start] == '"' || value[start] == '\'' {
				end++
				for end < len(value) && value[end] != value[start] {
					end++
				}
				if end < len(value) {
					end++
				}
			} else {
				for end < len(value) && value[end] != ',' && value[end] != '}' && value[end] != '\n' {
					end++
				}
			}
			value = value[:start] + `"[REDACTED]"` + value[end:]
			next := idx + len(pattern)
			if next >= len(value) {
				break
			}
			nextIdx := strings.Index(value[next:], pattern)
			if nextIdx < 0 {
				break
			}
			idx = next + nextIdx
		}
	}
	return value
}
