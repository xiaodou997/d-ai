package imageassets

import (
	"strings"

	"xiaodou/dai/internal/ai/imagepayload"
)

// DecodeInlineImageValue decodes image payloads that were mistakenly placed in
// URL fields instead of b64_json.
func DecodeInlineImageValue(value string) ([]byte, string, bool, error) {
	return imagepayload.DecodeInlineImageValue(value)
}

// LooksLikeInlineImageValue returns true when the string appears to contain
// inline image data rather than a real URL/path.
func LooksLikeInlineImageValue(value string) bool {
	return imagepayload.LooksLikeInlineImageValue(value)
}

func SanitizeAssetURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if LooksLikeInlineImageValue(value) {
		return ""
	}
	return value
}

func NormalizeAssetURLs(previewURL, displayURL, originalURL string) (string, string, string) {
	previewURL = SanitizeAssetURL(previewURL)
	displayURL = SanitizeAssetURL(displayURL)
	originalURL = SanitizeAssetURL(originalURL)

	if previewURL == "" {
		previewURL = displayURL
	}
	if previewURL == "" {
		previewURL = originalURL
	}
	if displayURL == "" {
		displayURL = previewURL
	}
	if displayURL == "" {
		displayURL = originalURL
	}
	return previewURL, displayURL, originalURL
}

func decodeBase64ImageBytes(value, contentType string) ([]byte, string, error) {
	return imagepayload.DecodeBase64ImageBytes(value, contentType)
}
