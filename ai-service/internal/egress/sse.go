package egress

import "bytes"

// Sanitizer is the runtime public egress sanitizer for one selected route.
type Sanitizer struct {
	policy Policy
}

func NewSanitizer(policy Policy) *Sanitizer {
	return &Sanitizer{policy: policy}
}

func (s *Sanitizer) SanitizeJSON(body []byte) []byte {
	if s == nil {
		return body
	}
	return SanitizeJSON(body, s.policy)
}

func (s *Sanitizer) SanitizeSSEData(data []byte) []byte {
	if s == nil || bytes.Equal(data, []byte("[DONE]")) {
		return data
	}
	return SanitizeJSON(data, s.policy)
}

func (s *Sanitizer) SanitizeText(text string) string {
	if s == nil {
		return text
	}
	return SanitizeText(text, s.policy)
}
