package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newConvRequest(t *testing.T, body string, headers map[string]string) (*http.Request, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, []byte(body)
}

func TestExtractConversationIDSources(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		headers map[string]string
		want    string
	}{
		{
			name:    "explicit header wins",
			body:    `{"conversation_id":"from-body"}`,
			headers: map[string]string{"X-Conversation-Id": "from-header"},
			want:    "from-header",
		},
		{
			name:    "codex session header",
			body:    `{"model":"gpt-5.6","input":[{"role":"user","content":"hi"}]}`,
			headers: map[string]string{"session_id": "sess-123"},
			want:    "sess-123",
		},
		{
			name: "codex prompt cache key",
			body: `{"model":"gpt-5.6","prompt_cache_key":"pck-42","input":[{"role":"user","content":"hi"}]}`,
			want: "pck-42",
		},
		{
			name: "responses conversation object",
			body: `{"conversation":{"id":"conv_abc"},"input":[]}`,
			want: "conv_abc",
		},
		{
			name: "responses conversation string",
			body: `{"conversation":"conv_str"}`,
			want: "conv_str",
		},
		{
			name: "anthropic metadata user id",
			body: `{"metadata":{"user_id":"user_x_session_y"},"messages":[{"role":"user","content":"hi"}]}`,
			want: "user_x_session_y",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, body := newConvRequest(t, tc.body, tc.headers)
			if got := extractConversationID(body, r); got != tc.want {
				t.Fatalf("extractConversationID = %q, want %q", got, tc.want)
			}
		})
	}
}

// A client that sends no conversation identity at all must still resolve to a
// stable key across the turns of one conversation — that is the whole point of
// the fingerprint fallback, and what keeps Codex/Claude Code pinned to a single
// upstream account.
func TestExtractConversationIDFingerprintIsStableAcrossTurns(t *testing.T) {
	turn1 := `{"model":"gpt-5.6","instructions":"you are codex","input":[{"role":"user","content":"write a parser"}]}`
	turn2 := `{"model":"gpt-5.6","instructions":"you are codex","input":[{"role":"user","content":"write a parser"},{"role":"assistant","content":"ok"},{"role":"user","content":"now add tests"}]}`

	r1, b1 := newConvRequest(t, turn1, nil)
	r2, b2 := newConvRequest(t, turn2, nil)

	got1 := extractConversationID(b1, r1)
	got2 := extractConversationID(b2, r2)
	if got1 == "" {
		t.Fatal("fingerprint is empty for the first turn")
	}
	if got1 != got2 {
		t.Fatalf("fingerprint changed between turns: %q vs %q", got1, got2)
	}

	other := `{"model":"gpt-5.6","instructions":"you are codex","input":[{"role":"user","content":"unrelated question"}]}`
	r3, b3 := newConvRequest(t, other, nil)
	if got3 := extractConversationID(b3, r3); got3 == got1 {
		t.Fatal("different conversations produced the same fingerprint")
	}
}

// Volatile system preambles (dates, cwd) must not leak into the fingerprint.
func TestExtractConversationIDFingerprintIgnoresSystemPreamble(t *testing.T) {
	day1 := `{"messages":[{"role":"system","content":"today is 2026-07-28"},{"role":"user","content":"hello"}]}`
	day2 := `{"messages":[{"role":"system","content":"today is 2026-07-29"},{"role":"user","content":"hello"}]}`

	r1, b1 := newConvRequest(t, day1, nil)
	r2, b2 := newConvRequest(t, day2, nil)
	if extractConversationID(b1, r1) != extractConversationID(b2, r2) {
		t.Fatal("fingerprint changed with the system preamble")
	}
}

func TestExtractConversationIDEmptyWithoutAnchor(t *testing.T) {
	r, b := newConvRequest(t, `{"model":"gpt-5.6"}`, nil)
	if got := extractConversationID(b, r); got != "" {
		t.Fatalf("expected empty conversation id, got %q", got)
	}
}
