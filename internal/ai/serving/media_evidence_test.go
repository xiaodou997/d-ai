package serving

import (
	"testing"
	"xiaodou/dai/internal/ai/domain"
)

func TestMediaEvidenceRequiresActualOutput(t *testing.T) {
	for _, tc := range []struct {
		body   string
		images int
	}{
		{`{"n":8,"url":"polling","parameters":{"n":8}}`, 0},
		{`{"data":[{"url":"image-one"},{"b64_json":"image-two"}]}`, 2},
		{`{"output":[{"type":"image_generation_call","result":"image-three"}]}`, 1},
		{`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"image-four"}},{"inlineData":{"mimeType":"audio/wav","data":"sound"}}]}}]}`, 1},
	} {
		req := &Request{CapabilityType: domain.CapabilityImage, TokenUsage: domain.TokenUsage{ImageCount: 8}}
		observeMediaOutput(req, []byte(tc.body))
		if req.TokenUsage.ImageCount != tc.images || req.MediaUsageConfirmed != (tc.images > 0) {
			t.Fatalf("%s: %+v", tc.body, req.TokenUsage)
		}
	}
	req := &Request{CapabilityType: domain.CapabilityVideo}
	observeMediaOutput(req, []byte(`{"duration":30,"videos":[{"url":"first","duration":2},{"url":"second","duration":3}]}`))
	if req.TokenUsage.VideoSeconds != 5 {
		t.Fatalf("duration=%v", req.TokenUsage.VideoSeconds)
	}
}
