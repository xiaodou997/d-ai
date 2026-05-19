package postgres

import (
	"testing"

	"uni-ai-api/backend/internal/domain"
)

// oauthFixedTypesForProtocol gates which OAuth credential pools are eligible
// for a given client_protocol. Adding a new fixed-provider type or protocol
// without updating both maps would silently break routing — this test makes
// the mapping the contract.
func TestOAuthFixedTypesForProtocol(t *testing.T) {
	cases := []struct {
		proto domain.UpstreamProtocol
		want  []string
	}{
		{domain.ProtocolOpenAIResponses, []string{"codex"}},
		{domain.ProtocolAnthropicMessages, []string{"claude_oauth"}},
		{domain.ProtocolGeminiGenerate, []string{"gemini_cli", "antigravity"}},
		// Protocols that have no OAuth pool today: chat, embeddings, images,
		// completions. Must return nil so the SQL ANY({}) clause filters them
		// out of the OAuth half of the WHERE.
		{domain.ProtocolOpenAIChat, nil},
		{domain.ProtocolOpenAIEmbeddings, nil},
		{domain.ProtocolOpenAIImages, nil},
		{domain.ProtocolOpenAICompletions, nil},
		{domain.ProtocolGeminiEmbeddings, nil},
	}
	for _, tc := range cases {
		got := oauthFixedTypesForProtocol(tc.proto)
		if !stringSliceEq(got, tc.want) {
			t.Errorf("proto=%q got=%v want=%v", tc.proto, got, tc.want)
		}
	}
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
