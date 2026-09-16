package serving

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"xiaodou/dai/internal/ai/audit"
	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/egress"
	"xiaodou/dai/internal/ai/formats"
)

type upstreamSSEEvent struct {
	raw, data []byte
	event     string
}

// readUpstreamEvent respects SSE event boundaries (including multiline data,
// CRLF, optional spaces and an EOF-dispatched final event).
func readUpstreamEvent(r *bufio.Reader) (upstreamSSEEvent, error) {
	var ev upstreamSSEEvent
	var data [][]byte
	for {
		line, err := readSSELine(r, maxSSELineBytes)
		ev.raw = append(ev.raw, line...)
		if len(ev.raw) > maxSSELineBytes {
			return ev, errSSELineTooLong
		}
		trimmed := trimEOL(line)
		field, value, _ := bytes.Cut(trimmed, []byte(":"))
		value = bytes.TrimPrefix(value, []byte(" "))
		switch string(field) {
		case "data":
			data = append(data, append([]byte(nil), value...))
		case "event":
			ev.event = string(value)
		}
		if len(trimmed) == 0 || err != nil {
			ev.data = bytes.Join(data, []byte("\n"))
			return ev, err
		}
	}
}

func (ev upstreamSSEEvent) withData(data []byte) []byte {
	if bytes.Equal(ev.data, data) {
		return ev.raw
	}
	var out bytes.Buffer
	wrote := false
	for _, line := range bytes.Split(ev.raw, []byte("\n")) {
		trimmed := bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(trimmed, []byte("data:")) {
			if !wrote {
				for _, part := range bytes.Split(data, []byte("\n")) {
					out.WriteString("data: ")
					out.Write(part)
					out.WriteByte('\n')
				}
				wrote = true
			}
		} else if len(trimmed) > 0 {
			out.Write(line)
			out.WriteByte('\n')
		}
	}
	out.WriteByte('\n')
	return out.Bytes()
}

// executeReportedStream owns the text relay lifecycle in both passthrough and
// conversion. Metering always observes provider bytes before downstream writes.
func (s *ExecuteStep) executeReportedStream(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, start time.Time, convert bool) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}
	if s.Bridge == nil {
		return &precommitError{cause: errUpstreamErrorBody, message: "runtime bridge is not configured"}
	}
	var provider corebridge.StreamProvider
	var emitter corebridge.StreamEmitter
	if convert {
		var err error
		provider, err = s.Bridge.NewProvider(req)
		if err != nil {
			return err
		}
		emitter, err = s.Bridge.NewEmitter(req)
		if err != nil {
			return err
		}
	}
	initializeSettlementState(req)
	acc := audit.NewResponseAccumulator(req.Candidate.Protocol)
	defer func() {
		ApplyReportedUsage(req)
		req.AuditResponseMessage = acc.Build()
		updateResponseSummaryState(req, req.AuditResponseMessage, req.ProviderTerminalState == domain.ProviderTerminalCompleted)
	}()
	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	sanitizer := egress.NewSanitizer(publicEgressPolicy(req, req.Candidate))
	var pending []upstreamSSEEvent
	pendingSize := 0
	commit := func() {
		if req.ResponseCommitted {
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		req.ResponseCommitted = true
		req.HTTPStatus = http.StatusOK
		dc.firstByte()
		req.MarkFirstResponseByte(time.Now())
		ms, _ := req.FirstResponseByteDurationMs()
		zap.L().Info("stream started", requestLogFields(req, zap.Bool("converted", convert), zap.Int("first_byte_ms", ms))...)
	}
	var pendingFinish *corebridge.StreamFrame
	completionSealed := false
	completionPersisted := false
	emit := func(frames []corebridge.StreamFrame) error {
		for _, frame := range frames {
			if frame.Event == corebridge.EvFinish && !completionSealed {
				copy := frame
				pendingFinish = &copy
				continue
			}
			frame.Text = string(s.restorePII(req, []byte(frame.Text)))
			if frame.Model != "" {
				frame.Model = req.PublicModel()
			}
			out, err := emitter.Emit(frame)
			if err != nil {
				return err
			}
			if _, err = w.Write(out); err != nil {
				return err
			}
		}
		return nil
	}
	forward := func(ev upstreamSSEEvent) error {
		if !convert {
			out := ev.raw
			if len(ev.data) > 0 && !bytes.Equal(ev.data, []byte("[DONE]")) {
				out = ev.withData(sanitizer.SanitizeSSEData(s.restorePII(req, ev.data)))
			}
			_, err := w.Write(out)
			return err
		}
		if len(ev.data) == 0 {
			return nil
		}
		data := ev.data
		// Bridge providers consume JSON types rather than SSE event names.
		// Carry an event-only type into conversion without changing passthrough
		// bytes or the upstream usage evidence already collected above.
		if ev.event != "" {
			var body map[string]json.RawMessage
			if json.Unmarshal(data, &body) == nil && body != nil {
				var typ string
				_ = json.Unmarshal(body["type"], &typ)
				if typ == "" {
					body["type"], _ = json.Marshal(ev.event)
					data, _ = json.Marshal(body)
				}
			}
		}
		frames, err := provider.PushLine(append(append([]byte("data: "), data...), '\n'))
		if err != nil {
			return err
		}
		return emit(frames)
	}
	choices := newStreamChoiceTracker(req)
	var readErr error
	var terminalFailure *upstreamSSEEvent
	for {
		if err := dc.ctx.Err(); err != nil {
			readErr = err
			break
		}
		ev, err := readUpstreamEvent(reader)
		if cancelErr := dc.ctx.Err(); cancelErr != nil {
			readErr = cancelErr
			break
		}
		if len(ev.raw) > 0 {
			dc.chunkReceived()
		}
		if len(ev.data) > 0 {
			normalized := s.Bridge.NormalizeResponseBody(req, ev.data)
			if !bytes.Equal(normalized, ev.data) {
				ev.raw = ev.withData(normalized)
				ev.data = normalized
			}
			observeReportedUsage(req, ev.data, ev.event)
			observeProviderStreamFrame(req, ev.data, ev.event)
			outcome := formats.InspectStreamOutcome(ev.data, ev.event)
			choices.observe(ev.data, req.Candidate.Protocol)
			finalEvent := providerFinalEvent(req.Candidate.Protocol, ev.data, ev.event)
			if req.Candidate.Protocol == domain.ProtocolGeminiGenerate {
				finalEvent = choices.complete()
			}
			if finalEvent && req.Candidate.Protocol == domain.ProtocolGeminiGenerate {
				markProviderTerminal(req, domain.ProviderTerminalCompleted)
			}
			if req.Candidate.Protocol == domain.ProtocolOpenAIChat && choices.complete() {
				markProviderTerminal(req, domain.ProviderTerminalCompleted)
			}
			if finalEvent && choices.incomplete(req.Candidate.Protocol) {
				markProviderTerminal(req, domain.ProviderTerminalIncomplete)
			}
			if finalEvent && req.ProviderTerminalState == domain.ProviderTerminalCompleted {
				req.HTTPStatus = http.StatusOK
				if err := s.sealCompletion(req); err != nil {
					return err
				}
				completionPersisted = true
			}
			if outcome.Event == "[DONE]" && req.Candidate.Protocol == domain.ProtocolOpenAIResponses && req.ProviderTerminalState != domain.ProviderTerminalCompleted {
				if err != nil {
					readErr = err
					break
				}
				continue
			}
			// Preserve usage in a following response.failed after a bare error.
			// No error event is converted into a successful bridge finish.
			if req.ProviderTerminalState == domain.ProviderTerminalFailed || req.ProviderTerminalState == domain.ProviderTerminalIncomplete || req.ProviderTerminalState == domain.ProviderTerminalCancelled {
				failureTerminal := !outcome.BareError && (outcome.State == domain.ProviderTerminalFailed || outcome.State == domain.ProviderTerminalIncomplete || outcome.State == domain.ProviderTerminalCancelled)
				if !failureTerminal && err == nil {
					// Metadata or a contradictory success event may intervene before
					// the failure snapshot. Keep collecting usage under the existing
					// deadline, but never forward these frames after an error.
					continue
				}
				if failureTerminal {
					terminalFailure = &ev
				}
				readErr = err
				break
			}
			if req.CaptureBody {
				acc.AddChunk(s.restorePII(req, ev.data))
			}
			semantic := streamChunkStartsToken(string(ev.data), ev.event, req.Candidate.Protocol)
			if semantic && req.FirstTokenMs == 0 {
				req.MarkFirstOutput(time.Now(), start)
			}
			ready := req.Candidate.Protocol != domain.ProtocolOpenAIResponses || semantic || req.ProviderTerminalState == domain.ProviderTerminalCompleted
			if !req.ResponseCommitted && ready {
				commit()
				for _, pre := range pending {
					if err := forward(pre); err != nil {
						return streamClientWriteError(req, err)
					}
				}
				pending = nil
			}
		}
		if req.ResponseCommitted {
			if err := forward(ev); err != nil {
				return streamClientWriteError(req, err)
			}
			flusher.Flush()
		} else {
			pending = append(pending, ev)
			pendingSize += len(ev.raw)
			if pendingSize > maxPreambleBytes {
				readErr = fmt.Errorf("upstream preamble exceeds 1 MiB without semantic output")
				break
			}
		}
		if (providerFinalEvent(req.Candidate.Protocol, ev.data, ev.event) || (req.Candidate.Protocol == domain.ProtocolGeminiGenerate && choices.complete())) && req.ProviderTerminalState == domain.ProviderTerminalCompleted {
			readErr = io.EOF
			break
		}
		if err != nil {
			readErr = err
			break
		}
	}
	if readErr != nil && readErr != io.EOF {
		if streamClientCancelled(dc, readErr) {
			markClientCancellation(req)
			req.ErrorCode, req.ErrorMessage = "client_disconnected", "the client disconnected before the upstream stream completed"
		} else {
			markProviderTerminal(req, domain.ProviderTerminalIncomplete)
			req.RequestStatus = domain.RequestFailed
			req.ErrorCode, req.ErrorMessage = streamFailureReason(dc, readErr)
			req.InternalErrorDetail = RedactInternalErrorDetail(readErr.Error())
		}
	}
	if req.ProviderTerminalState == domain.ProviderTerminalUnknown || (readErr == io.EOF && choices.incomplete(req.Candidate.Protocol)) {
		markProviderTerminal(req, domain.ProviderTerminalIncomplete)
	}
	if req.ProviderTerminalState != domain.ProviderTerminalCompleted || req.CancellationOrigin == domain.CancellationClient {
		if req.RequestStatus != domain.RequestCancelled {
			req.RequestStatus = domain.RequestFailed
		}
		req.FailedStep = "execute"
		if req.ErrorCode == "" {
			req.ErrorCode = "provider_terminal_error"
			if req.ProviderTerminalState == domain.ProviderTerminalIncomplete {
				req.ErrorCode = "upstream_stream_incomplete"
			}
		}
		if req.ErrorMessage == "" {
			req.ErrorMessage = "the upstream stream did not complete successfully"
		}
		if req.InternalErrorDetail != "" {
			req.ErrorMessage = egress.SanitizeText(req.InternalErrorDetail, PublicEgressPolicy(req))
		}
		if req.CancellationOrigin != domain.CancellationClient {
			req.BillingReason = "provider_terminal_failure"
		}
		if req.ProviderTerminalState == domain.ProviderTerminalCancelled {
			req.RequestStatus = domain.RequestCancelled
			req.CancellationOrigin = domain.CancellationProvider
			req.BillingReason = "provider_cancelled"
		}
		if !req.ResponseCommitted {
			return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode, message: req.ErrorMessage}
		}
		if req.CancellationOrigin != domain.CancellationClient {
			if err := s.sealCompletion(req); err != nil {
				return err
			}
			commit()
			frame := s.reportedStreamErrorFrame(req)
			if !convert && terminalFailure != nil {
				frame = terminalFailure.withData(sanitizer.SanitizeSSEData(s.restorePII(req, terminalFailure.data)))
			}
			if _, err := w.Write(frame); err != nil {
				return streamClientWriteError(req, err)
			}
			flusher.Flush()
			markClientDelivery(req, domain.ClientDeliveryComplete)
		}
		if readErr != nil && readErr != io.EOF && req.ResponseCommitted {
			return &postcommitError{code: req.ErrorCode, message: req.ErrorMessage}
		}
		return nil
	}
	if !req.ResponseCommitted {
		return precommitFromNoFrame(resp.StatusCode, nil, readErr)
	}
	// EOF with all choices finished is also a valid Chat terminal. Seal it
	// before releasing any bridge-generated terminal, even without [DONE].
	if !completionPersisted {
		if err := s.sealCompletion(req); err != nil {
			return err
		}
	}
	completionSealed = true
	if convert {
		if pendingFinish != nil {
			if err := emit([]corebridge.StreamFrame{*pendingFinish}); err != nil {
				return streamClientWriteError(req, err)
			}
		}
		frames, err := provider.Finish()
		if err != nil {
			return err
		}
		if err := emit(frames); err != nil {
			return streamClientWriteError(req, err)
		}
		tail, err := emitter.Finish()
		if err != nil {
			return err
		}
		if _, err := w.Write(tail); err != nil {
			return streamClientWriteError(req, err)
		}
		flusher.Flush()
	}
	req.RequestStatus = domain.RequestSuccess
	markClientDelivery(req, domain.ClientDeliveryComplete)
	zap.L().Info("stream finished", requestLogFields(req, zap.String("usage_source", req.TokenCountSource), zap.Int("prompt_tokens", req.TokenUsage.PromptTokens), zap.Int("completion_tokens", req.TokenUsage.CompletionTokens))...)
	return nil
}

func (s *ExecuteStep) reportedStreamErrorFrame(req *Request) []byte {
	if req.ClientProtocol != domain.ProtocolOpenAIResponses {
		return s.streamErrorFrame(req, req.ErrorCode, req.ErrorMessage)
	}
	code := req.UpstreamErrorCode
	if code == "" {
		code = req.ErrorCode
	}
	response := map[string]any{"id": "resp_" + req.RequestID, "object": "response", "status": "failed", "output": []any{}, "model": req.PublicModel(),
		"error": map[string]string{"code": code, "message": req.ErrorMessage}}
	if req.UsageEvidence.Reported() {
		u := req.UsageEvidence.Tokens()
		response["usage"] = map[string]any{"input_tokens": u.PromptTokens, "output_tokens": u.CompletionTokens,
			"input_tokens_details": map[string]int{"cached_tokens": u.CacheReadTokens}, "output_tokens_details": map[string]int{"reasoning_tokens": u.ReasoningTokens}}
	}
	payload, _ := json.Marshal(map[string]any{"type": "response.failed", "response": response})
	return []byte("event: response.failed\ndata: " + strings.TrimSpace(string(payload)) + "\n\n")
}
