package clientruntime

import "strings"

func firstMetadataString(metadata map[string]any, paths ...[]string) string {
	for _, path := range paths {
		var value any = metadata
		matched := true
		for _, segment := range path {
			current, ok := value.(map[string]any)
			if !ok {
				matched = false
				break
			}
			value, ok = current[segment]
			if !ok {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		switch current := value.(type) {
		case string:
			if current = strings.TrimSpace(current); current != "" {
				return current
			}
		case map[string]any:
			if nested := firstString(current, "id", "project_id", "projectId"); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}
