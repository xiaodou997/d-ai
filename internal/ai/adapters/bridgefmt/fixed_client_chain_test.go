package bridgefmt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/adapters/clienttransport"
	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

// Only the network origin is replaced. Request shaping, HTTP cancellation,
// protocol conversion, retry accounting and token metering use production code.
type nativeHTTP struct {
	target string
	client *http.Client
}

func (n nativeHTTP) Do(ctx context.Context, in *serving.UpstreamRequest) (*serving.UpstreamResponse, error) {
	u, err := url.Parse(in.URL)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, in.Method, n.target+u.RequestURI(), bytes.NewReader(in.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range in.Headers {
		r.Header.Set(k, v)
	}
	response, err := n.client.Do(r)
	if err != nil {
		return nil, err
	}
	return &serving.UpstreamResponse{StatusCode: response.StatusCode, Headers: response.Header, Body: response.Body}, nil
}

type nativePool struct {
	provider domain.FixedProviderType
	selected int
}

func (p *nativePool) SelectCredentialFromPool(_ context.Context, poolID, _ string) (*domain.OAuthCredential, error) {
	p.selected++
	if p.selected > 3 {
		return nil, errors.New("no credentials")
	}
	return &domain.OAuthCredential{ID: fmt.Sprintf("c%d", p.selected), PoolID: poolID, ProviderType: p.provider, AccessToken: fmt.Sprintf("access-%d", p.selected), Status: "active", TokenVersion: 1, AuthMetadata: map[string]any{"project_id": "project", "account_id": "workspace"}}, nil
}

func fixedProtocol(provider domain.FixedProviderType) domain.UpstreamProtocol {
	switch provider {
	case domain.FixedProviderCodex:
		return domain.ProtocolOpenAIResponses
	case domain.FixedProviderClaudeOAuth:
		return domain.ProtocolAnthropicMessages
	default:
		return domain.ProtocolGeminiGenerate
	}
}
func chainRequest(ctx context.Context, w http.ResponseWriter, provider domain.FixedProviderType, client domain.UpstreamProtocol, stream, followup bool) *serving.Request {
	var path, body string
	switch client {
	case domain.ProtocolOpenAIChat:
		path = "/v1/chat/completions"
		body = `{"model":"public","messages":[{"role":"user","content":"weather"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]}`
		if followup {
			body = `{"model":"public","messages":[{"role":"user","content":"weather"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":\"Paris\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"sunny"}]}`
		}
	case domain.ProtocolOpenAIResponses:
		path = "/v1/responses"
		body = `{"model":"public","input":"weather","tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}]}`
		if followup {
			body = `{"model":"public","input":[{"type":"message","role":"user","content":"weather"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"city\":\"Paris\"}"},{"type":"function_call_output","call_id":"call_1","output":"sunny"}]}`
		}
	case domain.ProtocolAnthropicMessages:
		path = "/v1/messages"
		body = `{"model":"public","max_tokens":128,"messages":[{"role":"user","content":"weather"}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]}`
		if followup {
			body = `{"model":"public","max_tokens":128,"messages":[{"role":"user","content":"weather"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"city":"Paris"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"sunny"}]}]}`
		}
	default:
		path = "/v1beta/models/public:generateContent"
		body = `{"contents":[{"role":"user","parts":[{"text":"weather"}]}],"tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}]}]}`
		if followup {
			body = `{"contents":[{"role":"user","parts":[{"text":"weather"}]},{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{"city":"Paris"}}}]},{"role":"user","parts":[{"functionResponse":{"id":"call_1","name":"lookup","response":{"result":"sunny"}}}]}]}`
		}
	}
	httpReq := httptest.NewRequest("POST", path, strings.NewReader(body)).WithContext(ctx)
	httpReq.Header.Set("Content-Type", "application/json")
	candidate := &domain.RouteCandidate{RouteID: "route", PoolID: "pool", ModelCode: "public", UpstreamModel: "native-model", PoolUpstreamModel: "native-model", Protocol: fixedProtocol(provider), FixedProviderType: provider, OAuthStrategy: "round_robin", Timeouts: domain.DefaultRouteTimeouts(domain.CapabilityChat)}
	return &serving.Request{RequestID: "chain", ModelCode: "public", RequestedModel: "public", CapabilityType: domain.CapabilityChat, ClientProtocol: client, ClientPath: path, IsStream: stream, Candidates: []*domain.RouteCandidate{candidate}, UsedCandidates: map[string]bool{}, Envelope: &serving.RequestEnvelope{R: httpReq, W: w, ClientBody: []byte(body)}}
}

func nativeToolStream(provider domain.FixedProviderType) string {
	switch provider {
	case domain.FixedProviderCodex:
		return "data: " + `{"type":"response.created","response":{"id":"resp_1","model":"native-model","status":"in_progress"}}` + "\n\ndata: " + `{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}` + "\n\ndata: " + `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"city\":\"Paris\"}"}` + "\n\ndata: " + `{"type":"response.completed","response":{"id":"resp_1","object":"response","model":"native-model","status":"completed","output":[{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"city\":\"Paris\"}","status":"completed"}],"usage":{"input_tokens":7,"output_tokens":3}}}` + "\n\n"
	case domain.FixedProviderClaudeOAuth:
		return "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"native-model","content":[],"usage":{"input_tokens":7,"output_tokens":0}}}` + "\n\nevent: content_block_start\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}` + "\n\nevent: content_block_delta\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Paris\"}"}}` + "\n\nevent: content_block_stop\ndata: " + `{"type":"content_block_stop","index":0}` + "\n\nevent: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}` + "\n\nevent: message_stop\ndata: " + `{"type":"message_stop"}` + "\n\n"
	default:
		return "data: " + `{"response":{"responseId":"resp_1","modelVersion":"native-model","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{"city":"Paris"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}}` + "\n\n"
	}
}

func TestFixedClientsStreamToolCallsAndToolResultsAcrossSurfaces(t *testing.T) {
	for _, provider := range []domain.FixedProviderType{domain.FixedProviderCodex, domain.FixedProviderClaudeOAuth, domain.FixedProviderGeminiCLI, domain.FixedProviderAntigravity} {
		for _, client := range []domain.UpstreamProtocol{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses, domain.ProtocolAnthropicMessages, domain.ProtocolGeminiGenerate} {
			t.Run(string(provider)+"/"+string(client), func(t *testing.T) {
				for _, followup := range []bool{false, true} {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ := io.ReadAll(r.Body)
						if r.Header.Get("Authorization") != "Bearer access-1" || !strings.Contains(string(body), `"model":"native-model"`) {
							t.Errorf("invalid native request: %s", body)
						}
						if followup && (!strings.Contains(string(body), "sunny") || !strings.Contains(string(body), "lookup")) {
							t.Errorf("tool result lost: %s", body)
						}
						if !followup && !strings.Contains(string(body), "lookup") {
							t.Errorf("tool schema lost: %s", body)
						}
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, nativeToolStream(provider))
					}))
					transport := nativeHTTP{target: upstream.URL, client: upstream.Client()}
					runtime := clientruntime.New(clienttransport.New(transport), nil)
					pool := &nativePool{provider: provider}
					w := httptest.NewRecorder()
					req := chainRequest(context.Background(), w, provider, client, true, followup)
					step := &serving.ExecuteStep{Transport: transport, ClientRuntime: runtime, Bridge: NewRuntime(), OAuthPool: pool, Budget: serving.RetryBudget{MaxAttempts: 1}}
					err := step.Execute(context.Background(), req)
					if err != nil || !strings.Contains(w.Body.String(), "lookup") || !strings.Contains(w.Body.String(), "Paris") || req.TokenUsage.PromptTokens != 7 || req.TokenUsage.CompletionTokens != 3 {
						t.Errorf("err=%v usage=%+v body=%s", err, req.TokenUsage, w.Body.String())
					}
					runtime.Stop(context.Background())
					upstream.Close()
				}
			})
		}
	}
}

func TestCodexNonStreamingClientsReceiveCompletedJSON(t *testing.T) {
	for _, client := range []domain.UpstreamProtocol{domain.ProtocolOpenAIResponses, domain.ProtocolOpenAIChat, domain.ProtocolAnthropicMessages, domain.ProtocolGeminiGenerate} {
		t.Run(string(client), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), `"stream":true`) || r.Header.Get("Accept") != "text/event-stream" {
					t.Errorf("Codex must use native SSE: %s", body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, nativeToolStream(domain.FixedProviderCodex))
			}))
			defer upstream.Close()
			transport := nativeHTTP{target: upstream.URL, client: upstream.Client()}
			runtime := clientruntime.New(clienttransport.New(transport), nil)
			defer runtime.Stop(context.Background())
			w := httptest.NewRecorder()
			req := chainRequest(context.Background(), w, domain.FixedProviderCodex, client, false, false)
			step := &serving.ExecuteStep{Transport: transport, ClientRuntime: runtime, Bridge: NewRuntime(), OAuthPool: &nativePool{provider: domain.FixedProviderCodex}, Budget: serving.RetryBudget{MaxAttempts: 1}}
			err := step.Execute(context.Background(), req)
			if err != nil || w.Header().Get("Content-Type") != "application/json" || !strings.Contains(w.Body.String(), "lookup") || req.TokenUsage.PromptTokens != 7 || strings.Contains(w.Body.String(), "data:") {
				t.Fatalf("err=%v usage=%+v response=%s", err, req.TokenUsage, w.Body.String())
			}
		})
	}
}

func TestCodexSyncStreamRequiresCompletionAndPreservesAttemptUsageAcrossRetries(t *testing.T) {
	for _, tc := range []struct {
		name, first string
		calls       int
		success     bool
		input       int
	}{
		{"truncated retries before delivery", "data: " + `{"type":"response.created","response":{"status":"in_progress"}}` + "\n\n", 2, true, 7},
		{"unmetered failure retries", "data: " + `{"type":"response.failed","response":{"status":"failed","error":{"message":"unavailable"}}}` + "\n\n", 2, true, 7},
		{"metered failure preserves its own usage", "data: " + `{"type":"response.failed","response":{"status":"failed","error":{"message":"unavailable"},"usage":{"input_tokens":9,"output_tokens":1}}}` + "\n\n", 2, true, 7},
		{"bare error preserves later failed usage", "event: error\ndata: " + `{"error":{"message":"failed"}}` + "\n\ndata: " + `{"type":"response.in_progress","response":{"status":"in_progress"}}` + "\n\ndata: " + `{"type":"response.failed","response":{"status":"failed","usage":{"input_tokens":9,"output_tokens":1}}}` + "\n\n", 2, true, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if calls.Add(1) == 1 {
					io.WriteString(w, tc.first)
				} else {
					io.WriteString(w, nativeToolStream(domain.FixedProviderCodex))
				}
			}))
			defer upstream.Close()
			transport := nativeHTTP{target: upstream.URL, client: upstream.Client()}
			runtime := clientruntime.New(clienttransport.New(transport), nil)
			defer runtime.Stop(context.Background())
			w := httptest.NewRecorder()
			req := chainRequest(context.Background(), w, domain.FixedProviderCodex, domain.ProtocolOpenAIChat, false, false)
			backup := *req.Candidates[0]
			backup.RouteID, backup.PoolID = "backup-route", "backup-pool"
			req.Candidates = append(req.Candidates, &backup)
			step := &serving.ExecuteStep{Transport: transport, ClientRuntime: runtime, Bridge: NewRuntime(), OAuthPool: &nativePool{provider: domain.FixedProviderCodex}, Budget: serving.RetryBudget{MaxAttempts: 2}}
			err := step.Execute(context.Background(), req)
			if calls.Load() != int32(tc.calls) || (err == nil) != tc.success || req.TokenUsage.PromptTokens != tc.input {
				t.Fatalf("calls=%d error=%v usage=%+v attempts=%+v", calls.Load(), err, req.TokenUsage, req.Attempts)
			}
			if strings.Contains(tc.first, `"input_tokens":9`) && (req.Attempts[0].UsageEvidence.Fields["input_tokens"] != 9 || !req.Attempts[0].UsageEvidence.Terminal) {
				t.Fatal("prior failed attempt usage was lost or attributed to the successful attempt")
			}
		})
	}
}

func TestNativeRateLimitSwapsAccountsWithinTheSendBudget(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"code":"rate_limit_exceeded","message":"rate limit"}}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer access-2" {
			t.Error("rate limited credential was reused")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, nativeToolStream(domain.FixedProviderCodex))
	}))
	defer upstream.Close()
	transport := nativeHTTP{target: upstream.URL, client: upstream.Client()}
	runtime := clientruntime.New(clienttransport.New(transport), nil)
	defer runtime.Stop(context.Background())
	w := httptest.NewRecorder()
	req := chainRequest(context.Background(), w, domain.FixedProviderCodex, domain.ProtocolOpenAIChat, true, false)
	step := &serving.ExecuteStep{Transport: transport, ClientRuntime: runtime, Bridge: NewRuntime(), OAuthPool: &nativePool{provider: domain.FixedProviderCodex}, Budget: serving.RetryBudget{MaxAttempts: 2}}
	if err := step.Execute(context.Background(), req); err != nil || calls.Load() != 2 || len(req.Attempts) != 2 {
		t.Fatalf("calls=%d attempts=%+v err=%v", calls.Load(), req.Attempts, err)
	}
}

type cancelWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w cancelWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	if bytes.Contains(p, []byte("partial")) {
		w.cancel()
	}
	return n, err
}
func TestNativeStreamCancellationClosesHTTPAndNeverReplays(t *testing.T) {
	var calls atomic.Int32
	closed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: "+`{"type":"response.output_text.delta","delta":"partial"}`+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	w := cancelWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	transport := nativeHTTP{target: upstream.URL, client: upstream.Client()}
	runtime := clientruntime.New(clienttransport.New(transport), nil)
	defer runtime.Stop(context.Background())
	req := chainRequest(ctx, w, domain.FixedProviderCodex, domain.ProtocolOpenAIChat, true, false)
	step := &serving.ExecuteStep{Transport: transport, ClientRuntime: runtime, Bridge: NewRuntime(), OAuthPool: &nativePool{provider: domain.FixedProviderCodex}, Budget: serving.RetryBudget{MaxAttempts: 3}}
	step.Execute(ctx, req)
	if !strings.Contains(w.Body.String(), "partial") {
		t.Fatalf("no output reached the client before cancellation: %s", w.Body.String())
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("native HTTP did not cancel")
	}
	if calls.Load() != 1 || len(req.Attempts) != 1 || strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("calls=%d attempts=%d response=%s", calls.Load(), len(req.Attempts), w.Body.String())
	}
}
