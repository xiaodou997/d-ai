package serving

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

// activeBucketTier must expose only the lowest non-exhausted ConversionBucket so
// zero-conversion routes are strictly tried before lossy conversion ones, then
// fall through as each tier is marked used by failover.
func TestActiveBucketTier(t *testing.T) {
	cands := []*domain.RouteCandidate{
		{RouteID: "a", ConversionBucket: 0},
		{RouteID: "b", ConversionBucket: 3},
		{RouteID: "c", ConversionBucket: 0},
	}
	used := map[string]bool{}

	tier := activeBucketTier(cands, used)
	if len(tier) != 2 || tier[0].RouteID != "a" || tier[1].RouteID != "c" {
		t.Fatalf("first tier = %+v, want [a c] (bucket 0)", routeIDs(tier))
	}

	used["a"], used["c"] = true, true
	tier = activeBucketTier(cands, used)
	if len(tier) != 1 || tier[0].RouteID != "b" {
		t.Fatalf("fallback tier = %v, want [b] (bucket 3)", routeIDs(tier))
	}

	used["b"] = true
	if tier := activeBucketTier(cands, used); len(tier) != 0 {
		t.Fatalf("exhausted tier = %v, want empty", routeIDs(tier))
	}
}

func TestActivePriorityTier(t *testing.T) {
	cands := []*domain.RouteCandidate{
		{RouteID: "a", Priority: 20},
		{RouteID: "b", Priority: 10},
		{RouteID: "c", Priority: 10},
	}
	used := map[string]bool{}

	tier := activePriorityTier(cands, used)
	if len(tier) != 2 || tier[0].RouteID != "b" || tier[1].RouteID != "c" {
		t.Fatalf("first priority tier = %+v, want [b c] (priority 10)", routeIDs(tier))
	}

	used["b"], used["c"] = true, true
	tier = activePriorityTier(cands, used)
	if len(tier) != 1 || tier[0].RouteID != "a" {
		t.Fatalf("fallback priority tier = %v, want [a] (priority 20)", routeIDs(tier))
	}
}

func TestActiveGroupTierExhaustsHigherRankBeforeFallback(t *testing.T) {
	cands := []*domain.RouteCandidate{
		{RouteID: "primary-slow", GroupRank: 0, Priority: 20},
		{RouteID: "secondary-fast", GroupRank: 1, Priority: 1},
		{RouteID: "primary-next", GroupRank: 0, Priority: 30},
	}
	used := map[string]bool{}

	tier := activeGroupTier(cands, used)
	if got := routeIDs(tier); len(got) != 2 || got[0] != "primary-slow" || got[1] != "primary-next" {
		t.Fatalf("primary group tier = %v", got)
	}
	used["primary-slow"], used["primary-next"] = true, true
	if got := routeIDs(activeGroupTier(cands, used)); len(got) != 1 || got[0] != "secondary-fast" {
		t.Fatalf("fallback group tier = %v", got)
	}
}

func TestCandidateSplitExhaustsAllRoutesForOnePhysicalTarget(t *testing.T) {
	req := &Request{
		Candidates: []*domain.RouteCandidate{
			{RouteID: "route-a1", EndpointID: "account-a"},
			{RouteID: "route-a2", EndpointID: "account-a"},
			{RouteID: "route-b1", EndpointID: "account-b"},
		},
		UsedCandidates: map[string]bool{},
	}

	exhaustPhysicalTarget(req, req.Candidates[0])

	if !req.UsedCandidates["route-a1"] || !req.UsedCandidates["route-a2"] {
		t.Fatalf("routes sharing account-a were not exhausted: %#v", req.UsedCandidates)
	}
	if req.UsedCandidates["route-b1"] {
		t.Fatalf("unrelated account-b route was exhausted: %#v", req.UsedCandidates)
	}
}

func TestPickCandidateKeepsTargetPriorityAheadOfConversionPreference(t *testing.T) {
	req := &Request{
		Candidates: []*domain.RouteCandidate{
			{RouteID: "primary-converted", GroupRank: 0, Priority: 10, ConversionBucket: 3},
			{RouteID: "backup-native", GroupRank: 0, Priority: 20, ConversionBucket: 0},
		},
		UsedCandidates: map[string]bool{},
	}
	got, _ := (&ExecuteStep{}).pickCandidate(context.Background(), req)
	if got == nil || got.RouteID != "primary-converted" {
		t.Fatalf("picked %#v, want configured primary route", got)
	}
}

func TestPickCandidateRecordsPolicyReason(t *testing.T) {
	t.Parallel()

	req := &Request{
		Candidates: []*domain.RouteCandidate{
			{
				RouteID:       "weighted-route",
				GroupID:       "group-1",
				GroupRank:     0,
				Priority:      1,
				RouteStrategy: "weighted",
				RoutingWeight: 2,
			},
			{RouteID: "excluded-route", GroupID: "group-1", GroupRank: 0, Priority: 1, RouteStrategy: "weighted"},
		},
		UsedCandidates: map[string]bool{},
	}
	got, _ := (&ExecuteStep{Scorer: &MultiDimScorer{}}).pickCandidate(context.Background(), req)
	if got == nil || got.RouteID != "weighted-route" {
		t.Fatalf("picked %#v", got)
	}
	if req.SelectionReason != "weighted" {
		t.Fatalf("selection reason = %q, want weighted", req.SelectionReason)
	}
}

func TestPickCandidateExplainsSingleCandidate(t *testing.T) {
	t.Parallel()

	req := &Request{
		Candidates: []*domain.RouteCandidate{{
			RouteID:       "only-route",
			GroupRank:     0,
			Priority:      1,
			RouteStrategy: "adaptive",
		}},
		UsedCandidates: map[string]bool{},
	}
	if got, _ := (&ExecuteStep{Scorer: &MultiDimScorer{}}).pickCandidate(context.Background(), req); got == nil {
		t.Fatal("expected a candidate")
	}
	if req.SelectionReason != "single_candidate" {
		t.Fatalf("selection reason = %q, want single_candidate", req.SelectionReason)
	}
}

func TestPickCandidateKeepsWeightedSelectionInsidePriorityTier(t *testing.T) {
	t.Parallel()

	req := &Request{
		Candidates: []*domain.RouteCandidate{
			{RouteID: "primary", GroupRank: 0, Priority: 1, RouteStrategy: "weighted", RoutingWeight: 0},
			{RouteID: "backup", GroupRank: 0, Priority: 5, RouteStrategy: "weighted", RoutingWeight: 100},
		},
		UsedCandidates: map[string]bool{},
	}
	got, _ := (&ExecuteStep{Scorer: &MultiDimScorer{}}).pickCandidate(context.Background(), req)
	if got == nil || got.RouteID != "primary" {
		t.Fatalf("weighted policy crossed priority boundary and picked %#v", got)
	}
}

func TestPickCandidateExplainsAllZeroWeightedFallback(t *testing.T) {
	t.Parallel()

	req := &Request{
		Candidates: []*domain.RouteCandidate{
			{RouteID: "zero-a", GroupRank: 0, Priority: 1, RouteStrategy: "weighted"},
			{RouteID: "zero-b", GroupRank: 0, Priority: 1, RouteStrategy: "weighted"},
		},
		UsedCandidates: map[string]bool{},
	}
	got, _ := (&ExecuteStep{Scorer: &MultiDimScorer{}}).pickCandidate(context.Background(), req)
	if got == nil {
		t.Fatal("expected priority fallback candidate")
	}
	if req.SelectionReason != "weighted_fallback" {
		t.Fatalf("selection reason = %q, want weighted_fallback", req.SelectionReason)
	}
}

type imageResponseNormalizerStub struct {
	format string
}

func (s *imageResponseNormalizerStub) NormalizeImageResponse(_ context.Context, _ []byte, format string) ([]byte, error) {
	s.format = format
	return []byte(`{"data":[{"url":"/runtime/v1/images/assets/test?key=test"}]}`), nil
}

func TestExecuteImageRelayNormalizesToClientFormat(t *testing.T) {
	normalizer := &imageResponseNormalizerStub{}
	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := &Request{
		CapabilityType:            domain.CapabilityImage,
		ClientProtocol:            domain.ProtocolOpenAIImages,
		ImageClientResponseFormat: domain.ImageResponseFormatURL,
		Candidate:                 &domain.RouteCandidate{Protocol: domain.ProtocolOpenAIImages},
	}

	err := (&ExecuteStep{Bridge: testProtocolBridge{}, ImageNormalizer: normalizer}).executeImageRelay(
		dc,
		req,
		jsonResp(`{"data":[{"b64_json":"aGVsbG8="}]}`),
		w,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("executeImageRelay: %v", err)
	}
	if normalizer.format != domain.ImageResponseFormatURL {
		t.Fatalf("normalizer format = %q, want url", normalizer.format)
	}
	if !strings.Contains(w.Body.String(), "/runtime/v1/images/assets/test") {
		t.Fatalf("normalized response missing from client body: %s", w.Body.String())
	}
}

func routeIDs(cs []*domain.RouteCandidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.RouteID
	}
	return out
}

// Streaming Anthropic upstream → OpenAI chat client: the gateway translates each
// SSE frame two-sidedly, rewrites the model to the public code, maps usage, and
// terminates with [DONE].
func TestExecuteStreamConvertClaudeToOpenAI(t *testing.T) {
	body := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":10,"output_tokens":0}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		"",
	}, "\n") + "\n"

	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := &Request{
		IsStream:       true,
		ClientProtocol: domain.ProtocolOpenAIChat,
		ModelCode:      "public-model",
		Candidate:      &domain.RouteCandidate{Protocol: domain.ProtocolAnthropicMessages, RouteID: "r1", UpstreamModel: "claude-x"},
		Attempts:       []AttemptRecord{{RouteID: "r1"}},
	}

	if err := (&ExecuteStep{Bridge: testProtocolBridge{}}).executeStreamConvert(dc, req, sseResp(body), w, time.Now()); err != nil {
		t.Fatalf("executeStreamConvert err = %v, want nil", err)
	}
	got := w.Body.String()
	if !strings.Contains(got, "chat.completion.chunk") {
		t.Fatalf("client did not receive OpenAI chunks: %q", got)
	}
	if !strings.Contains(got, `"content":"hi"`) {
		t.Fatalf("text delta not translated: %q", got)
	}
	if !strings.Contains(got, "[DONE]") {
		t.Fatalf("stream not terminated with [DONE]: %q", got)
	}
	if !strings.Contains(got, "public-model") || strings.Contains(got, "claude-x") {
		t.Fatalf("model not rewritten to public code: %q", got)
	}
	if req.TokenUsage.PromptTokens != 10 || req.TokenUsage.CompletionTokens != 5 {
		t.Fatalf("usage = %d/%d, want 10/5", req.TokenUsage.PromptTokens, req.TokenUsage.CompletionTokens)
	}
	if req.RequestStatus != domain.RequestSuccess || req.HTTPStatus != http.StatusOK {
		t.Fatalf("status = %v/%d, want success/200", req.RequestStatus, req.HTTPStatus)
	}
}

func TestExecuteStreamConvertPreservesClaudeTerminalUsageSnapshot(t *testing.T) {
	body := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":0,"output_tokens":0}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":544,"output_tokens":55}}`,
		"",
	}, "\n") + "\n"

	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := &Request{
		IsStream:         true,
		ClientProtocol:   domain.ProtocolOpenAIChat,
		ModelCode:        "public-model",
		UpstreamBodySize: 162,
		Candidate:        &domain.RouteCandidate{Protocol: domain.ProtocolAnthropicMessages, RouteID: "r1", UpstreamModel: "claude-x"},
		Attempts:         []AttemptRecord{{RouteID: "r1"}},
	}

	if err := (&ExecuteStep{Bridge: testProtocolBridge{}}).executeStreamConvert(dc, req, sseResp(body), w, time.Now()); err != nil {
		t.Fatalf("executeStreamConvert err = %v, want nil", err)
	}
	if req.TokenUsage.PromptTokens != 544 || req.TokenUsage.CompletionTokens != 55 {
		t.Fatalf("usage = %+v, want 544 input / 55 output", req.TokenUsage)
	}
	if req.TokenCountSource != domain.TokenUsageSourceUpstream {
		t.Fatalf("token source = %q, want upstream", req.TokenCountSource)
	}
}

// Non-streaming Anthropic upstream → OpenAI chat client: response body is
// translated to OpenAI shape, model rewritten, usage extracted from the
// provider-format body.
func TestExecuteSyncConvertClaudeToOpenAI(t *testing.T) {
	body := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-x",` +
		`"content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn",` +
		`"usage":{"input_tokens":10,"output_tokens":3}}`

	w := httptest.NewRecorder()
	dc := genTestDC()
	defer dc.stop()
	req := &Request{
		ClientProtocol: domain.ProtocolOpenAIChat,
		ModelCode:      "public-model",
		Candidate:      &domain.RouteCandidate{Protocol: domain.ProtocolAnthropicMessages, RouteID: "r1", UpstreamModel: "claude-x"},
		Attempts:       []AttemptRecord{{RouteID: "r1"}},
	}

	if err := (&ExecuteStep{Bridge: testProtocolBridge{}}).executeSyncConvert(dc, req, jsonResp(body), w); err != nil {
		t.Fatalf("executeSyncConvert err = %v, want nil", err)
	}
	got := w.Body.String()
	if !strings.Contains(got, "chat.completion") {
		t.Fatalf("response not translated to OpenAI shape: %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Fatalf("text content lost in translation: %q", got)
	}
	if !strings.Contains(got, "public-model") || strings.Contains(got, "claude-x") {
		t.Fatalf("model not rewritten to public code: %q", got)
	}
	if req.TokenUsage.PromptTokens != 10 || req.TokenUsage.CompletionTokens != 3 {
		t.Fatalf("usage = %d/%d, want 10/3", req.TokenUsage.PromptTokens, req.TokenUsage.CompletionTokens)
	}
}

func jsonResp(body string) *UpstreamResponse {
	r := sseResp(body)
	r.Headers.Set("Content-Type", "application/json")
	return r
}

type testProtocolBridge struct{}

func (testProtocolBridge) PrepareRequest(req *Request, body []byte) (corebridge.PreparedRequest, error) {
	if req.ClientProtocol != req.Candidate.Protocol {
		rewritten, err := testProtocolBridge{}.BridgeRequest(req, body)
		return corebridge.PreparedRequest{Body: rewritten}, err
	}
	return corebridge.PreparedRequest{Body: body}, nil
}

func (testProtocolBridge) BridgeRequest(req *Request, body []byte) ([]byte, error) {
	src, _ := formats.FormatIDForProtocol(req.ClientProtocol)
	dst, _ := formats.FormatIDForProtocol(req.Candidate.Protocol)
	return formats.ConvertRequest(src, dst, body, req.Candidate.UpstreamModel, req.IsStream)
}

func (testProtocolBridge) BridgeResponse(req *Request, body []byte) ([]byte, error) {
	src, _ := formats.FormatIDForProtocol(req.Candidate.Protocol)
	dst, _ := formats.FormatIDForProtocol(req.ClientProtocol)
	return formats.ConvertResponse(src, dst, body)
}

func (testProtocolBridge) BridgeImageStream(req *Request, rawBody []byte) (corebridge.ImageStreamResult, error) {
	return corebridge.ImageStreamResult{
		ClientStream: rawBody,
		ProviderBody: rawBody,
	}, nil
}

func (testProtocolBridge) AggregateImageProviderBody(req *Request, rawBody []byte) ([]byte, error) {
	return rawBody, nil
}

func (testProtocolBridge) BuildImageClientStream(req *Request, clientBody []byte) ([]byte, error) {
	return append([]byte("data: "), append(clientBody, []byte("\n\n")...)...), nil
}

func (testProtocolBridge) BuildUpstreamRequest(req *Request, prepared corebridge.PreparedRequest) (*UpstreamRequest, error) {
	return &UpstreamRequest{
		Method:   "POST",
		URL:      "https://example.test/upstream",
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     prepared.Body,
		Protocol: req.Candidate.Protocol,
	}, nil
}

func (testProtocolBridge) NewProvider(req *Request) (corebridge.StreamProvider, error) {
	src, _ := formats.FormatIDForProtocol(req.Candidate.Protocol)
	return &testStreamProvider{inner: mustStreamProvider(src, req.Candidate.UpstreamModel)}, nil
}

func (testProtocolBridge) NewEmitter(req *Request) (corebridge.StreamEmitter, error) {
	dst, _ := formats.FormatIDForProtocol(req.ClientProtocol)
	return &testStreamEmitter{inner: mustStreamEmitter(dst)}, nil
}

func (testProtocolBridge) ExtractSyncUsage(req *Request, body []byte) domain.TokenUsage {
	return formats.ExtractSyncUsage(body, req.Candidate.Protocol)
}

func (testProtocolBridge) ExtractStreamUsage(req *Request, prev domain.TokenUsage, data []byte, eventType string) (domain.TokenUsage, bool) {
	return formats.ExtractStreamUsage(prev, data, eventType, req.Candidate.Protocol)
}

func (testProtocolBridge) NormalizeResponseBody(req *Request, body []byte) []byte {
	return body
}

func (testProtocolBridge) StreamErrorFrame(req *Request, code, msg string) []byte {
	return formats.StreamErrorFrame(req.ClientProtocol, code, msg)
}

type testStreamProvider struct{ inner formats.StreamProvider }

func (p *testStreamProvider) PushLine(line []byte) ([]corebridge.StreamFrame, error) {
	frames, err := p.inner.PushLine(line)
	if err != nil {
		return nil, err
	}
	out := make([]corebridge.StreamFrame, 0, len(frames))
	for _, frame := range frames {
		out = append(out, corebridge.StreamFrame{
			ID:           frame.ID,
			Model:        frame.Model,
			Event:        corebridge.StreamEvent(frame.Event),
			Text:         frame.Text,
			ToolIndex:    frame.ToolIndex,
			CallID:       frame.CallID,
			Name:         frame.Name,
			Arguments:    frame.Arguments,
			ToolUseID:    frame.ToolUseID,
			Content:      frame.Content,
			FinishReason: frame.FinishReason,
			HasFinish:    frame.HasFinish,
			Usage:        mapTestUsage(frame.Usage),
			Unknown:      append([]byte(nil), frame.Unknown...),
		})
	}
	return out, nil
}

func (p *testStreamProvider) Finish() ([]corebridge.StreamFrame, error) {
	frames, err := p.inner.Finish()
	if err != nil {
		return nil, err
	}
	out := make([]corebridge.StreamFrame, 0, len(frames))
	for _, frame := range frames {
		out = append(out, corebridge.StreamFrame{
			ID:           frame.ID,
			Model:        frame.Model,
			Event:        corebridge.StreamEvent(frame.Event),
			Text:         frame.Text,
			ToolIndex:    frame.ToolIndex,
			CallID:       frame.CallID,
			Name:         frame.Name,
			Arguments:    frame.Arguments,
			ToolUseID:    frame.ToolUseID,
			Content:      frame.Content,
			FinishReason: frame.FinishReason,
			HasFinish:    frame.HasFinish,
			Usage:        mapTestUsage(frame.Usage),
			Unknown:      append([]byte(nil), frame.Unknown...),
		})
	}
	return out, nil
}

type testStreamEmitter struct{ inner formats.StreamEmitter }

func (e *testStreamEmitter) Emit(frame corebridge.StreamFrame) ([]byte, error) {
	return e.inner.Emit(formats.StreamFrame{
		ID:           frame.ID,
		Model:        frame.Model,
		Event:        formats.StreamEvent(frame.Event),
		Text:         frame.Text,
		ToolIndex:    frame.ToolIndex,
		CallID:       frame.CallID,
		Name:         frame.Name,
		Arguments:    frame.Arguments,
		ToolUseID:    frame.ToolUseID,
		Content:      frame.Content,
		FinishReason: frame.FinishReason,
		HasFinish:    frame.HasFinish,
		Usage:        unmapTestUsage(frame.Usage),
		Unknown:      append([]byte(nil), frame.Unknown...),
	})
}

func (e *testStreamEmitter) Finish() ([]byte, error) {
	return e.inner.Finish()
}

func mustStreamProvider(id formats.FormatID, model string) formats.StreamProvider {
	provider, err := formats.NewStreamProvider(id, model)
	if err != nil {
		panic(err)
	}
	return provider
}

func mustStreamEmitter(id formats.FormatID) formats.StreamEmitter {
	emitter, err := formats.NewStreamEmitter(id)
	if err != nil {
		panic(err)
	}
	return emitter
}

func mapTestUsage(in *formats.Usage) *corebridge.Usage {
	if in == nil {
		return nil
	}
	return &corebridge.Usage{
		InputTokens:                 in.InputTokens,
		OutputTokens:                in.OutputTokens,
		TotalTokens:                 in.TotalTokens,
		CacheReadTokens:             in.CacheReadTokens,
		CacheWriteTokens:            in.CacheWriteTokens,
		CacheCreationEphemeral5mTok: in.CacheCreationEphemeral5mTok,
		CacheCreationEphemeral1hTok: in.CacheCreationEphemeral1hTok,
		ReasoningTokens:             in.ReasoningTokens,
	}
}

func unmapTestUsage(in *corebridge.Usage) *formats.Usage {
	if in == nil {
		return nil
	}
	return &formats.Usage{
		InputTokens:                 in.InputTokens,
		OutputTokens:                in.OutputTokens,
		TotalTokens:                 in.TotalTokens,
		CacheReadTokens:             in.CacheReadTokens,
		CacheWriteTokens:            in.CacheWriteTokens,
		CacheCreationEphemeral5mTok: in.CacheCreationEphemeral5mTok,
		CacheCreationEphemeral1hTok: in.CacheCreationEphemeral1hTok,
		ReasoningTokens:             in.ReasoningTokens,
	}
}
