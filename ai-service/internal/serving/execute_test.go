package serving

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaodou/uni-ai-api/internal/domain"
)

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name   string
		cand   *domain.RouteCandidate
		stream bool
		want   string
	}{
		{
			name: "openai chat default path",
			cand: &domain.RouteCandidate{
				BaseURL:  "https://api.openai.com",
				Protocol: domain.ProtocolOpenAIChat,
			},
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "trailing slash trimmed and custom path",
			cand: &domain.RouteCandidate{
				BaseURL:     "https://example.com/api/",
				RequestPath: "/foo/bar",
				Protocol:    domain.ProtocolOpenAIChat,
			},
			want: "https://example.com/api/foo/bar",
		},
		{
			name: "path without leading slash gets one",
			cand: &domain.RouteCandidate{
				BaseURL:     "https://example.com",
				RequestPath: "rel/path",
				Protocol:    domain.ProtocolOpenAIChat,
			},
			want: "https://example.com/rel/path",
		},
		{
			name: "gemini model placeholder substitution",
			cand: &domain.RouteCandidate{
				BaseURL:       "https://generativelanguage.googleapis.com",
				UpstreamModel: "gemini-2.0-flash",
				Protocol:      domain.ProtocolGeminiGenerate,
			},
			want: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
		},
		{
			name: "gemini_cli uses CodeAssist URL regardless of RequestPath",
			cand: &domain.RouteCandidate{
				BaseURL:           "https://ignored.example",
				RequestPath:       "/should/be/ignored",
				FixedProviderType: domain.FixedProviderGeminiCLI,
				Protocol:          domain.ProtocolGeminiGenerate,
			},
			stream: false,
			// We can't hardcode the exact URL without importing the gemini pkg;
			// just sanity-check it does NOT fall back to the RequestPath.
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildURL(tc.cand, tc.stream)
			if tc.want != "" && got != tc.want {
				t.Fatalf("buildURL = %q, want %q", got, tc.want)
			}
			if tc.cand.FixedProviderType == domain.FixedProviderGeminiCLI {
				if got == "" || got == tc.cand.BaseURL+tc.cand.RequestPath {
					t.Fatalf("gemini_cli URL should bypass RequestPath, got %q", got)
				}
			}
		})
	}
}

func TestBuildHeadersAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		cand     *domain.RouteCandidate
		req      *Request
		wantAuth string
		wantKey  string // header name expected to carry the key
	}{
		{
			name: "openai uses Bearer",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolOpenAIChat,
				APIKeyCiphertext: "sk-abc",
			},
			req:      &Request{},
			wantAuth: "Bearer sk-abc",
			wantKey:  "Authorization",
		},
		{
			name: "anthropic uses x-api-key + version",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolAnthropicMessages,
				APIKeyCiphertext: "sk-ant",
			},
			req:     &Request{},
			wantKey: "x-api-key",
		},
		{
			name: "gemini exposes x-gemini-api-key for transport to consume",
			cand: &domain.RouteCandidate{
				Protocol:         domain.ProtocolGeminiGenerate,
				APIKeyCiphertext: "AIza-test",
			},
			req:     &Request{},
			wantKey: "x-gemini-api-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := buildHeaders(tc.cand, tc.req)
			if h["Content-Type"] != "application/json" {
				t.Fatalf("missing Content-Type")
			}
			switch tc.wantKey {
			case "Authorization":
				if h["Authorization"] != tc.wantAuth {
					t.Fatalf("Authorization = %q, want %q", h["Authorization"], tc.wantAuth)
				}
			case "x-api-key":
				if h["x-api-key"] != tc.cand.APIKeyCiphertext {
					t.Fatalf("x-api-key not set to API key")
				}
				if h["anthropic-version"] == "" {
					t.Fatalf("anthropic-version missing")
				}
			case "x-gemini-api-key":
				if h["x-gemini-api-key"] != tc.cand.APIKeyCiphertext {
					t.Fatalf("x-gemini-api-key = %q, want %q", h["x-gemini-api-key"], tc.cand.APIKeyCiphertext)
				}
				if _, ok := h["Authorization"]; ok {
					t.Fatalf("Gemini API-key path must not set Authorization header")
				}
			}
		})
	}
}

func TestBuildHeadersStreamAccept(t *testing.T) {
	cand := &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat, APIKeyCiphertext: "k"}
	h := buildHeaders(cand, &Request{IsStream: true})
	if h["Accept"] != "text/event-stream" {
		t.Fatalf("stream request must set Accept text/event-stream, got %q", h["Accept"])
	}
}

func TestUpstreamStatusToGateway(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{429, http.StatusTooManyRequests},
		{500, http.StatusBadGateway},
		{502, http.StatusBadGateway},
		{503, http.StatusBadGateway},
		{401, http.StatusBadGateway},
		{403, http.StatusBadGateway},
		{400, 400},
		{404, 404},
		{409, 409},
	}
	for _, tc := range tests {
		if got := upstreamStatusToGateway(tc.in); got != tc.want {
			t.Errorf("upstreamStatusToGateway(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// ============================================================================
// Streaming execution — delayed commit / precommit failover / postcommit frame
// ============================================================================

func TestPayloadIsError(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`{"error":{"message":"x"}}`, true},
		{`{"type":"error","error":{}}`, true},
		{`  {"error":{"code":1}}`, true},
		{`{"choices":[{"delta":{}}]}`, false},
		{`{"type":"message","role":"assistant"}`, false},
		{`{"id":"x","object":"response"}`, false},
		{`[DONE]`, false},
		{``, false},
		{`not json`, false},
	}
	for _, c := range cases {
		if got := payloadIsError([]byte(c.in)); got != c.want {
			t.Errorf("payloadIsError(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestReadSSELine(t *testing.T) {
	r := bufio.NewReaderSize(strings.NewReader("a\nbb\n\nccc"), 8)
	want := []string{"a\n", "bb\n", "\n", "ccc"}
	for i, w := range want {
		line, err := readSSELine(r, maxSSELineBytes)
		if string(line) != w {
			t.Fatalf("line %d = %q, want %q", i, line, w)
		}
		if i < len(want)-1 && err != nil {
			t.Fatalf("line %d unexpected err %v", i, err)
		}
		if i == len(want)-1 && err != io.EOF {
			t.Fatalf("last line err = %v, want io.EOF", err)
		}
	}
}

// errAfterReader yields data then fails — simulates a mid-stream upstream drop.
type errAfterReader struct {
	data []byte
	pos  int
}

func (e *errAfterReader) Read(p []byte) (int, error) {
	if e.pos >= len(e.data) {
		return 0, errors.New("simulated upstream connection drop")
	}
	n := copy(p, e.data[e.pos:])
	e.pos += n
	return n, nil
}

func (e *errAfterReader) Close() error { return nil }

type delayedFirstByteReader struct {
	ctx   context.Context
	delay time.Duration
	data  []byte
	sent  bool
}

func (r *delayedFirstByteReader) Read(p []byte) (int, error) {
	if !r.sent {
		select {
		case <-time.After(r.delay):
			r.sent = true
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *delayedFirstByteReader) Close() error { return nil }

func genTestDC() *deadlineController {
	return newDeadlineController(context.Background(), domain.RouteTimeouts{
		Connect: time.Hour, FirstByte: time.Hour, Idle: time.Hour, MaxDuration: time.Hour,
	})
}

func newStreamReq() *Request {
	return &Request{
		IsStream:       true,
		ClientProtocol: domain.ProtocolOpenAIChat,
		Candidate:      &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat, RouteID: "r1"},
		Attempts:       []AttemptRecord{{RouteID: "r1"}},
	}
}

func sseResp(body string) *UpstreamResponse {
	return &UpstreamResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestExecuteStreamSuccess(t *testing.T) {
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	if err := (&ExecuteStep{}).executeStream(dc, req, sseResp(body), w, time.Now()); err != nil {
		t.Fatalf("executeStream err = %v, want nil", err)
	}
	if req.HTTPStatus != http.StatusOK {
		t.Fatalf("HTTPStatus = %d, want 200 (committed)", req.HTTPStatus)
	}
	if req.RequestStatus != domain.RequestSuccess {
		t.Fatalf("RequestStatus = %v, want success", req.RequestStatus)
	}
	if !strings.Contains(w.Body.String(), "hi") || !strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("stream body not forwarded verbatim: %q", w.Body.String())
	}
}

func TestExecuteStreamPrecommitEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	err := (&ExecuteStep{}).executeStream(dc, req, sseResp(""), w, time.Now())
	if !isPrecommitError(err) {
		t.Fatalf("empty stream: err = %v, want *precommitError", err)
	}
	if req.HTTPStatus != 0 {
		t.Fatalf("HTTPStatus must stay 0 (uncommitted), got %d", req.HTTPStatus)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("nothing must be written to the client before commit, got %q", w.Body.String())
	}
}

func TestExecuteStreamPrecommitErrorBody(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	err := (&ExecuteStep{}).executeStream(dc, req, sseResp(`{"error":{"message":"bad gateway"}}`), w, time.Now())
	if !isPrecommitError(err) {
		t.Fatalf("200+error body: err = %v, want *precommitError", err)
	}
	if req.HTTPStatus != 0 {
		t.Fatalf("HTTPStatus must stay 0, got %d", req.HTTPStatus)
	}
}

func TestExecuteStreamPrecommitFirstFrameError(t *testing.T) {
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	body := "data: {\"error\":{\"message\":\"upstream boom\"}}\n\n"
	err := (&ExecuteStep{}).executeStream(dc, req, sseResp(body), w, time.Now())
	if !isPrecommitError(err) {
		t.Fatalf("first-frame error: err = %v, want *precommitError", err)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("an error first frame must not be forwarded, got %q", w.Body.String())
	}
}

func TestExecuteStreamPostcommitErrorFrame(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
	resp := &UpstreamResponse{StatusCode: http.StatusOK, Headers: http.Header{}, Body: &errAfterReader{data: body}}
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := newStreamReq()

	err := (&ExecuteStep{}).executeStream(dc, req, resp, w, time.Now())
	var pc *postcommitError
	if !errors.As(err, &pc) {
		t.Fatalf("mid-stream drop: err = %v, want *postcommitError", err)
	}
	if req.HTTPStatus != http.StatusOK {
		t.Fatalf("stream should have committed before the drop, HTTPStatus = %d", req.HTTPStatus)
	}
	if req.RequestStatus != domain.RequestFailed {
		t.Fatalf("RequestStatus = %v, want failed", req.RequestStatus)
	}
	if !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("client must receive a protocol error frame, got %q", w.Body.String())
	}
}

func TestExecuteSyncFirstByteTimeout(t *testing.T) {
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		Connect: time.Hour, FirstByte: 20 * time.Millisecond, Idle: time.Hour, MaxDuration: time.Hour,
	})
	defer dc.stop()
	dc.headersReceived()

	resp := &UpstreamResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{},
		Body: &delayedFirstByteReader{
			ctx:   dc.ctx,
			delay: 200 * time.Millisecond,
			data:  []byte(`{"ok":true}`),
		},
	}
	req := &Request{Candidate: &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIChat}}
	w := httptest.NewRecorder()

	err := (&ExecuteStep{}).executeSync(dc, req, resp, w)
	var pre *precommitError
	if !errors.As(err, &pre) {
		t.Fatalf("executeSync err = %v, want *precommitError", err)
	}
	if !errors.Is(pre.Unwrap(), ErrFirstByteTimeout) {
		t.Fatalf("precommit cause = %v, want ErrFirstByteTimeout", pre.Unwrap())
	}
	if req.HTTPStatus != 0 {
		t.Fatalf("sync response must stay uncommitted on first-byte timeout, got HTTPStatus=%d", req.HTTPStatus)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("nothing must be written before sync first-byte commit, got %q", w.Body.String())
	}
}
