package serving

import (
	"encoding/json"
	"strings"
	"xiaodou/dai/internal/ai/domain"
)

// Only output-bearing protocol fields establish media quantity. Echoed request
// parameters and unrelated URLs (documentation, polling, errors) are excluded.
func observeMediaOutput(req *Request, body []byte) {
	if req.CapabilityType != domain.CapabilityImage && req.CapabilityType != domain.CapabilityVideo {
		return
	}
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return
	}
	images := 0
	seconds := 0.0
	text := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	var walk func(any, bool)
	walk = func(v any, output bool) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				walk(item, output)
			}
		case map[string]any:
			if req.CapabilityType == domain.CapabilityImage {
				if text(x, "type") == "image_generation_call" && text(x, "result") != "" {
					images++
					return
				}
				if output && (text(x, "b64_json") != "" || text(x, "url") != "") {
					images++
					return
				}
				for _, k := range []string{"inlineData", "inline_data"} {
					if m, ok := x[k].(map[string]any); ok && text(m, "data") != "" && (strings.HasPrefix(text(m, "mimeType"), "image/") || strings.HasPrefix(text(m, "mime_type"), "image/")) {
						images++
						return
					}
				}
			} else if output && (text(x, "url") != "" || text(x, "uri") != "" || text(x, "video_url") != "") {
				for _, k := range []string{"duration_seconds", "duration", "seconds"} {
					if n, ok := x[k].(float64); ok && n > 0 {
						seconds += n
						return
					}
				}
			}
			for k, item := range x {
				switch k {
				case "data", "images", "output", "videos", "generatedSamples", "generated_samples", "candidates", "content", "parts", "video":
					walk(item, true)
				}
			}
		}
	}
	walk(root, false)
	if req.CapabilityType == domain.CapabilityImage {
		req.TokenUsage.ImageCount = images
		req.MediaUsageConfirmed = images > 0
	} else {
		req.TokenUsage.VideoSeconds = seconds
		req.MediaUsageConfirmed = seconds > 0
	}
}
