package gateway

import (
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

func parseServiceTier(body []byte, contentType string) (domain.ServiceTier, error) {
	raw, err := requestServiceTier(body, contentType)
	if err != nil {
		return "", err
	}
	return normalizeServiceTier(raw)
}

func validateServiceTierParseableBody(body []byte, contentType string) error {
	if len(body) == 0 {
		return nil
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType == "multipart/form-data" {
		_, err := formats.MultipartScalarFields(body, contentType, 1<<20)
		return err
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	return nil
}

func requestServiceTier(body []byte, contentType string) (string, error) {
	if len(body) == 0 {
		return "", nil
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType == "multipart/form-data" {
		fields, err := formats.MultipartScalarFields(body, contentType, 1<<20)
		if err != nil {
			return "", fmt.Errorf("parse service_tier: %w", err)
		}
		return fields["service_tier"], nil
	}
	var meta struct {
		ServiceTier string `json:"service_tier"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", fmt.Errorf("parse service_tier: %w", err)
	}
	return meta.ServiceTier, nil
}

func normalizeServiceTier(raw string) (domain.ServiceTier, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto", "default", "standard":
		return domain.ServiceTierStandard, nil
	case "priority", "fast":
		return domain.ServiceTierFast, nil
	default:
		return "", fmt.Errorf("invalid service_tier %q: expected standard, fast, priority, auto, or default", raw)
	}
}
