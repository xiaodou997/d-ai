package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/upstreamcontrol"
)

func TestEnsureDirectUpstreamExistsUsesAccountReader(t *testing.T) {
	reader := &recordingUpstreamAccountReader{
		account: upstreamcontrol.AccountSecret{DefaultProtocol: "anthropic"},
	}
	const accountID = "6f8f5771-b98c-44d4-997d-fd4848ce5d2d"

	got, err := ensureDirectUpstreamExists(t.Context(), reader, accountID)
	if err != nil {
		t.Fatalf("ensure direct upstream: %v", err)
	}
	if reader.accountID != accountID {
		t.Fatalf("reader account ID = %q, want %q", reader.accountID, accountID)
	}
	if got.DefaultProtocol != "anthropic" {
		t.Fatalf("default protocol = %q, want anthropic", got.DefaultProtocol)
	}
}

func TestEnsureDirectUpstreamExistsRejectsInvalidIDBeforeReader(t *testing.T) {
	reader := &recordingUpstreamAccountReader{}
	if _, err := ensureDirectUpstreamExists(t.Context(), reader, "not-a-uuid"); err == nil {
		t.Fatal("expected invalid account ID error")
	}
	if reader.accountID != "" {
		t.Fatalf("reader called with invalid account ID %q", reader.accountID)
	}
}

type recordingUpstreamAccountReader struct {
	accountID string
	account   upstreamcontrol.AccountSecret
}

func (r *recordingUpstreamAccountReader) GetAccountSecret(_ context.Context, id string) (upstreamcontrol.AccountSecret, error) {
	r.accountID = id
	return r.account, nil
}
