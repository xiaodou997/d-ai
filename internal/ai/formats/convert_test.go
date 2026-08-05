package formats

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestDetectClientProtocolImagesEdits(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	if got := DetectClientProtocol(req); got != domain.ProtocolOpenAIImages {
		t.Fatalf("DetectClientProtocol = %v, want %v", got, domain.ProtocolOpenAIImages)
	}
}
