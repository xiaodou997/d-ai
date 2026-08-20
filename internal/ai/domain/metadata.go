package domain

import "strings"

// RedactSensitiveMetadata returns a copy of account metadata without values
// that could contain credentials or key material. It is shared by adapters
// and transport so a non-secret read model remains safe at every boundary.
func RedactSensitiveMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if sensitiveMetadataKey(key) {
			continue
		}
		out[key] = redactSensitiveMetadataValue(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func redactSensitiveMetadataValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return RedactSensitiveMetadata(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, redactSensitiveMetadataValue(item))
		}
		return out
	default:
		return value
	}
}

func sensitiveMetadataKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, part := range []string{"token", "secret", "password", "cipher", "api_key", "apikey", "key_hash"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}
