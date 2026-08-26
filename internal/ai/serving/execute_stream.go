package serving

import (
	"bufio"
	"bytes"
	"errors"
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
)

// ============================================================================
// Non-streaming execution — strict 1:1 passthrough.
//
// The route candidate is guaranteed (by routes.go SQL filter) to share the
// client's protocol, so the upstream response body is already in the wire
// format the client expects. We extract usage as a side effect for billing
// and forward the bytes verbatim. The only mutation is unwrapping the
// CodeAssist {"response": {...}} envelope for gemini_cli / antigravity so
// the client receives a vanilla Gemini response.
// ============================================================================

func (s *ExecuteStep) executeSync(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter) error {
	bodyReader := &responseBodyReader{r: resp.Body, onFirstByte: dc.syncBodyPhase}
	bodyBytes, err := io.ReadAll(io.LimitReader(bodyReader, syncBodyLimit(req)))
	if err != nil {
		if cause := dc.cause(); cause != nil {
			err = cause
		}
		// Body read failed before anything was written downstream — retryable.
		return &precommitError{
			cause:      err,
			httpStatus: resp.StatusCode,
			message:    "read upstream body: " + err.Error(),
		}
	}
	req.UpstreamResponseBody = bodyBytes
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return &precommitError{
			cause:      errUpstreamEmpty,
			httpStatus: resp.StatusCode,
			message:    fmt.Sprintf("upstream returned %d with an empty response body", resp.StatusCode),
		}
	}

	policy := publicEgressPolicy(req, req.Candidate)
	bodyBytes = s.Bridge.NormalizeResponseBody(req, bodyBytes)
	bodyBytes = s.restorePII(req, bodyBytes)
	bodyBytes = egress.SanitizeJSON(bodyBytes, policy)

	// Extract the assistant reply for the audit log from the original body
	// (before model-name rewrite, but model identity is captured separately).
	req.AuditResponseMessage = audit.ExtractSyncResponseMessage(req.UpstreamResponseBody, req.Candidate.Protocol)
	req.AuditResponseMessage = s.restorePII(req, req.AuditResponseMessage)

	// A 200 whose body is actually an error object is a failed attempt — fail
	// over to another route instead of relaying a broken success.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && payloadIsError(bodyBytes) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: http.StatusOK,
			message:    "upstream returned 200 with an error body: " + truncateValidUTF8(string(bytes.TrimSpace(bodyBytes)), 256),
		}
	}

	// Side-channel: usage for billing. ExtractSyncUsage returns a zero value
	// when upstream did not report tokens; fillEstimatedUsage then falls back
	// to byte-length estimation.
	if u := s.extractSyncUsage(req, bodyBytes); u.PromptTokens != 0 || u.CompletionTokens != 0 {
		// Preserve any pre-populated image/video billing fields.
		u.ImageCount = req.TokenUsage.ImageCount
		u.ImageResolution = req.TokenUsage.ImageResolution
		u.VideoSeconds = req.TokenUsage.VideoSeconds
		u.VideoResolution = req.TokenUsage.VideoResolution
		req.TokenUsage = u
	}
	fillEstimatedUsage(req, len(bodyBytes))

	// Forward upstream Content-Type when present (Anthropic returns
	// `application/json`; OpenAI/Gemini do likewise). Defaults are safe.
	if ct := resp.Headers.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	req.ResponseCommitted = true
	req.MarkFirstResponseByte(time.Now())
	_, _ = w.Write(bodyBytes)

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode
	return nil
}

// responseBodyReader keeps the first-byte timer active until body data arrives.
// Streaming/aggregated callers can also reset an idle timer for later chunks.
type responseBodyReader struct {
	r           io.Reader
	onFirstByte func()
	onChunk     func()
	seen        bool
}

func (r *responseBodyReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		if !r.seen {
			r.seen = true
			r.onFirstByte()
		} else if r.onChunk != nil {
			r.onChunk()
		}
	}
	return n, err
}

// ============================================================================
// Streaming execution — delayed-commit, line-level 1:1 passthrough.
//
// Phase 3 (确认): buffer leading event:/comment lines and read until the first
// usable `data:` frame. If the upstream produced nothing usable — empty
// stream, 200-with-error-body, or an error as the first frame — the relay
// returns a *precommitError and the execute loop fails over to another route
// WITHOUT having written anything to the client.
//
// Phase 4 (传输): once the first frame is confirmed the gateway commits 200 OK
// and forwards every subsequent SSE line verbatim (data: lines for
// gemini_cli / antigravity have the CodeAssist envelope stripped; usage frames
// are inspected for billing). A mid-stream failure after this point can no
// longer fail over, so the client is sent a protocol-level error frame.
// ============================================================================

// maxSSELineBytes is a defensive OOM ceiling, NOT a functional cap: a single
// SSE data frame may legitimately inline a multi-megabyte base64 image (4K
// images in particular). It only guards against a pathological upstream that
// never emits a newline.
const maxSSELineBytes = 256 * 1024 * 1024

// maxPreambleBytes caps the leading non-data lines buffered before the first
// data frame — a well-behaved upstream sends only a few small event: lines.
const maxPreambleBytes = 1 << 20

var (
	errStreamNoFlusher   = errors.New("response writer does not support streaming")
	errUpstreamErrorBody = errors.New("upstream returned an error body")
	errUpstreamEmpty     = errors.New("upstream returned an empty stream")
	errSSELineTooLong    = errors.New("sse frame exceeds the 256MiB safety ceiling")
)

const (
	defaultSyncBodyLimitBytes = 32 * 1024 * 1024
	imageSyncBodyLimitBytes   = 256 * 1024 * 1024
)

func (s *ExecuteStep) executeStream(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	dataPrefix := []byte("data: ")
	eventPrefix := []byte("event: ")
	donePayload := []byte("[DONE]")

	// ---- Phase 3 确认：缓冲前导行，读到首个有效 data 帧才提交 ----
	var (
		preamble  []byte
		eventType string
		firstData []byte
		awaitErr  error
	)
awaitLoop:
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		trimmed := trimEOL(line)
		switch {
		case bytes.HasPrefix(trimmed, dataPrefix):
			firstData = line
			awaitErr = readErr
			break awaitLoop
		case bytes.HasPrefix(trimmed, eventPrefix):
			eventType = string(bytes.TrimPrefix(trimmed, eventPrefix))
			preamble = append(preamble, line...)
		default:
			// blank line / comment / non-SSE bytes — buffer it
			preamble = append(preamble, line...)
		}
		if len(preamble) > maxPreambleBytes {
			return &precommitError{
				cause:      errUpstreamErrorBody,
				httpStatus: resp.StatusCode,
				message:    "upstream sent an oversized preamble without a data frame",
			}
		}
		if readErr != nil {
			awaitErr = readErr
			break awaitLoop
		}
	}

	// No data frame at all — the upstream produced nothing usable. Fail over.
	if firstData == nil {
		if cause := dc.cause(); cause != nil {
			awaitErr = cause
		}
		return precommitFromNoFrame(resp.StatusCode, preamble, awaitErr)
	}

	// The first data frame itself is an error event. Fail over.
	firstPayload := trimEOL(firstData)[len(dataPrefix):]
	if !bytes.Equal(firstPayload, donePayload) && (eventType == "error" || payloadIsError(firstPayload)) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: resp.StatusCode,
			message:    "upstream streamed an error before any content: " + truncateValidUTF8(string(firstPayload), 256),
		}
	}

	// ---- commit：从这里起响应不可逆 ----
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	req.ResponseCommitted = true
	req.HTTPStatus = http.StatusOK
	dc.firstByte()
	req.MarkFirstResponseByte(time.Now())
	req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
	zap.L().Info("stream started",
		requestLogFields(req,
			zap.String("model_code", req.ModelCode),
			zap.String("upstream_model", req.Candidate.UpstreamModel),
			zap.String("provider_code", req.Candidate.ProviderCode),
			zap.String("route_id", req.Candidate.RouteID),
			zap.Int("first_byte_ms", req.FirstTokenMs),
		)...,
	)

	accumulatedOutputBytes := 0
	publicSanitizer := egress.NewSanitizer(publicEgressPolicy(req, req.Candidate))
	auditAcc := audit.NewResponseAccumulator(req.Candidate.Protocol)

	// forward writes one already-framed SSE line to the client, stripping the
	// CodeAssist envelope on data lines, rewriting the model name to the
	// public-facing model code, and side-extracting usage for billing.
	forward := func(line []byte, evt string) error {
		trimmed := trimEOL(line)
		if !bytes.HasPrefix(trimmed, dataPrefix) {
			_, err := w.Write(line) // blank line / comment — verbatim
			return err
		}
		data := trimmed[len(dataPrefix):]
		if bytes.Equal(data, donePayload) {
			_, err := w.Write(line)
			return err
		}
		unwrapped := s.Bridge.NormalizeResponseBody(req, data)
		unwrapped = s.restorePII(req, unwrapped)
		finalData := publicSanitizer.SanitizeSSEData(unwrapped)
		auditAcc.AddChunk(finalData)
		if bytes.Equal(finalData, data) {
			if _, err := w.Write(line); err != nil {
				return err
			}
		} else {
			out := append(append([]byte(nil), dataPrefix...), finalData...)
			out = append(out, '\n')
			if _, err := w.Write(out); err != nil {
				return err
			}
		}
		if u, ok := s.extractStreamUsage(req, req.TokenUsage, finalData, evt); ok {
			u.ImageCount = req.TokenUsage.ImageCount
			u.ImageResolution = req.TokenUsage.ImageResolution
			u.VideoSeconds = req.TokenUsage.VideoSeconds
			u.VideoResolution = req.TokenUsage.VideoResolution
			req.TokenUsage = u
			accumulatedOutputBytes = 0
		} else {
			accumulatedOutputBytes += len(finalData)
		}
		return nil
	}

	// Forward the buffered preamble + the confirmed first data frame.
	if len(preamble) > 0 {
		if _, err := w.Write(preamble); err != nil {
			return streamClientWriteError(req, err)
		}
	}
	if err := forward(firstData, eventType); err != nil {
		return streamClientWriteError(req, err)
	}
	flusher.Flush()
	eventType = ""

	if awaitErr != nil {
		req.AuditResponseMessage = auditAcc.Build()
		return finishStream(s, req, startTime, accumulatedOutputBytes, awaitErr, dc, w, flusher)
	}

	// ---- Phase 4 传输 ----
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		if len(line) > 0 {
			dc.chunkReceived()
			trimmed := trimEOL(line)
			if bytes.HasPrefix(trimmed, eventPrefix) {
				eventType = string(bytes.TrimPrefix(trimmed, eventPrefix))
				if _, err := w.Write(line); err != nil {
					return streamClientWriteError(req, err)
				}
			} else {
				if err := forward(line, eventType); err != nil {
					return streamClientWriteError(req, err)
				}
				flusher.Flush()
				eventType = ""
			}
		}
		if readErr != nil {
			req.AuditResponseMessage = auditAcc.Build()
			return finishStream(s, req, startTime, accumulatedOutputBytes, readErr, dc, w, flusher)
		}
	}
}

// finishStream handles the end of a committed stream. io.EOF is a clean
// finish; anything else is a post-commit failure that earns a protocol-shaped
// error frame so the client is not left with a silent truncation.
func finishStream(s *ExecuteStep, req *Request, startTime time.Time, accumulatedOutputBytes int, readErr error, dc *deadlineController, w http.ResponseWriter, flusher http.Flusher) error {
	fillEstimatedUsage(req, accumulatedOutputBytes)

	if readErr == io.EOF {
		req.RequestStatus = domain.RequestSuccess
		req.HTTPStatus = http.StatusOK
		zap.L().Info("stream finished",
			requestLogFields(req,
				zap.String("model_code", req.ModelCode),
				zap.Int("first_token_ms", req.FirstTokenMs),
				zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
				zap.Int("prompt_tokens", req.TokenUsage.PromptTokens),
				zap.Int("completion_tokens", req.TokenUsage.CompletionTokens),
			)...,
		)
		return nil
	}

	// Post-commit failure — 200 OK is already sent and the body is partial.
	code, msg := streamFailureReason(dc, readErr)
	req.FailedStep = "execute"
	req.InternalErrorDetail = RedactInternalErrorDetail(msg)
	msg = egress.SanitizeText(msg, PublicEgressPolicy(req))
	req.RequestStatus = domain.RequestFailed
	req.HTTPStatus = http.StatusOK
	req.ErrorCode = code
	req.ErrorMessage = msg
	if _, werr := w.Write(s.streamErrorFrame(req, code, msg)); werr == nil {
		flusher.Flush()
	}
	zap.L().Warn("stream aborted after commit",
		requestLogFields(req,
			zap.String("error_code", code),
			zap.String("error", msg),
			zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
		)...,
	)
	return &postcommitError{code: code, message: msg}
}

// streamFailureReason maps a post-commit read failure to an error code/message.
func streamFailureReason(dc *deadlineController, readErr error) (string, string) {
	switch dc.cause() {
	case ErrIdleTimeout:
		return "stream_idle_timeout", "upstream stream stalled and was terminated by the gateway"
	case ErrMaxDuration:
		return "stream_max_duration", "upstream stream exceeded the maximum allowed duration"
	}
	if errors.Is(readErr, errSSELineTooLong) {
		return "stream_frame_too_large", "an upstream stream frame exceeded the gateway size limit"
	}
	return "stream_read_error", "the gateway lost the connection to the upstream mid-stream: " + readErr.Error()
}

// precommitFromNoFrame builds a precommitError for an upstream stream that
// ended before producing a single data frame.
func precommitFromNoFrame(status int, preamble []byte, readErr error) *precommitError {
	if payloadIsError(preamble) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: status,
			message:    "upstream returned a non-SSE error body: " + truncateValidUTF8(string(bytes.TrimSpace(preamble)), 256),
		}
	}
	cause := errUpstreamEmpty
	if readErr != nil && readErr != io.EOF {
		cause = readErr
	}
	return &precommitError{
		cause:      cause,
		httpStatus: status,
		message:    "upstream produced no stream content: " + cause.Error(),
	}
}

// ============================================================================
// Cross-protocol execution (P-C) — client≠provider 时经 internal/formats 翻译。
//
// 非流：读 provider-format 响应 → 提 usage/audit（provider 口径）→ ConvertResponse
// 翻成 client 格式 → 按 client 协议 egress sanitize → 写。
// 流式：两侧式翻译——NewStreamProvider(provider 格式) 把上游 SSE 解析成中性帧，
// NewStreamEmitter(client 格式) 重放成 client SSE；复用 passthrough 路径的 precommit
// 探错/commit/postcommit 错误帧语义。帧的 Model 改写为对外 model_code（替代 egress
// 的模型名重写，emit 产物本就是结构化干净输出）。
// ============================================================================

// applyCanonicalUsage maps canonical stream/response usage into req.TokenUsage,
// preserving any pre-populated image/video billing fields. No-op when usage
// carries no prompt/completion tokens (caller falls back to byte estimation).
func applyCanonicalUsage(req *Request, u *corebridge.Usage) {
	if u == nil || (u.InputTokens == 0 && u.OutputTokens == 0) {
		return
	}
	tu := domain.TokenUsage{
		PromptTokens:     int(u.InputTokens),
		CompletionTokens: int(u.OutputTokens),
		CacheWriteTokens: int(u.CacheWriteTokens),
		CacheReadTokens:  int(u.CacheReadTokens),
		ReasoningTokens:  int(u.ReasoningTokens),
	}
	tu.ImageCount = req.TokenUsage.ImageCount
	tu.ImageResolution = req.TokenUsage.ImageResolution
	tu.VideoSeconds = req.TokenUsage.VideoSeconds
	tu.VideoResolution = req.TokenUsage.VideoResolution
	req.TokenUsage = tu
}

func (s *ExecuteStep) executeSyncConvert(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter) error {
	bodyReader := &responseBodyReader{r: resp.Body, onFirstByte: dc.syncBodyPhase}
	bodyBytes, err := io.ReadAll(io.LimitReader(bodyReader, syncBodyLimit(req)))
	if err != nil {
		if cause := dc.cause(); cause != nil {
			err = cause
		}
		return &precommitError{cause: err, httpStatus: resp.StatusCode, message: "read upstream body: " + err.Error()}
	}
	req.UpstreamResponseBody = bodyBytes
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return &precommitError{
			cause:      errUpstreamEmpty,
			httpStatus: resp.StatusCode,
			message:    fmt.Sprintf("upstream returned %d with an empty response body", resp.StatusCode),
		}
	}

	// provider-format body (strip CodeAssist envelope for gemini pools).
	provBody := s.Bridge.NormalizeResponseBody(req, bodyBytes)

	// A 200 whose body is an error object is a failed attempt — fail over.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && payloadIsError(provBody) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: http.StatusOK,
			message:    "upstream returned 200 with an error body: " + truncateValidUTF8(string(bytes.TrimSpace(provBody)), 256),
		}
	}

	// Usage + audit from the provider-format body, keyed on the provider protocol.
	if u := s.Bridge.ExtractSyncUsage(req, provBody); u.PromptTokens != 0 || u.CompletionTokens != 0 {
		u.ImageCount = req.TokenUsage.ImageCount
		u.ImageResolution = req.TokenUsage.ImageResolution
		u.VideoSeconds = req.TokenUsage.VideoSeconds
		u.VideoResolution = req.TokenUsage.VideoResolution
		req.TokenUsage = u
	}
	req.AuditResponseMessage = audit.ExtractSyncResponseMessage(req.UpstreamResponseBody, req.Candidate.Protocol)
	req.AuditResponseMessage = s.restorePII(req, req.AuditResponseMessage)

	// Translate provider → client.
	if s.Bridge == nil {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "runtime bridge is not configured for sync response conversion"}
	}
	converted, cerr := s.Bridge.BridgeResponse(req, provBody)
	if cerr != nil {
		// 2xx but untranslatable — nothing written downstream, so fail over.
		return &precommitError{cause: cerr, httpStatus: resp.StatusCode, message: "convert response: " + cerr.Error()}
	}

	fillEstimatedUsage(req, len(provBody))

	// Sanitize with the CLIENT protocol policy — the bytes are now client-format.
	policy := publicEgressPolicy(req, req.Candidate)
	policy.Protocol = req.ClientProtocol
	out := s.restorePII(req, []byte(converted))
	out = egress.SanitizeJSON(out, policy)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	req.ResponseCommitted = true
	req.MarkFirstResponseByte(time.Now())
	_, _ = w.Write(out)

	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = resp.StatusCode
	return nil
}

func (s *ExecuteStep) executeStreamConvert(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}
	if s.Bridge == nil {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "runtime bridge is not configured for stream conversion"}
	}
	provider, perr := s.Bridge.NewProvider(req)
	if perr != nil {
		return &precommitError{cause: perr, httpStatus: resp.StatusCode, message: perr.Error()}
	}
	emitter, eerr := s.Bridge.NewEmitter(req)
	if eerr != nil {
		return &precommitError{cause: eerr, httpStatus: resp.StatusCode, message: eerr.Error()}
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	dataPrefix := []byte("data: ")
	eventPrefix := []byte("event: ")
	donePayload := []byte("[DONE]")

	// ---- Phase 3 确认：读上游直到首个 data 帧（探错/空流 → failover）----
	var (
		consumed     [][]byte
		consumedLen  int
		firstData    []byte
		firstPayload []byte
		eventType    string
		awaitErr     error
	)
awaitLoop:
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		if len(line) > 0 {
			trimmed := trimEOL(line)
			switch {
			case bytes.HasPrefix(trimmed, dataPrefix):
				firstData = line
				firstPayload = trimmed[len(dataPrefix):]
				consumed = append(consumed, line)
				consumedLen += len(line)
				awaitErr = readErr
				break awaitLoop
			case bytes.HasPrefix(trimmed, eventPrefix):
				eventType = string(bytes.TrimPrefix(trimmed, eventPrefix))
				consumed = append(consumed, line)
				consumedLen += len(line)
			default:
				consumed = append(consumed, line)
				consumedLen += len(line)
			}
			if consumedLen > maxPreambleBytes {
				return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
					message: "upstream sent an oversized preamble without a data frame"}
			}
		}
		if readErr != nil {
			awaitErr = readErr
			break awaitLoop
		}
	}

	if firstData == nil {
		if cause := dc.cause(); cause != nil {
			awaitErr = cause
		}
		return precommitFromNoFrame(resp.StatusCode, joinSSELines(consumed), awaitErr)
	}
	if !bytes.Equal(firstPayload, donePayload) && (eventType == "error" || payloadIsError(firstPayload)) {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "upstream streamed an error before any content: " + truncateValidUTF8(string(firstPayload), 256)}
	}

	// ---- commit：响应不可逆 ----
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	req.ResponseCommitted = true
	req.HTTPStatus = http.StatusOK
	dc.firstByte()
	req.MarkFirstResponseByte(time.Now())
	req.FirstTokenMs = int(time.Since(startTime).Milliseconds())
	zap.L().Info("stream started (convert)",
		requestLogFields(req,
			zap.String("model_code", req.ModelCode),
			zap.String("client_protocol", string(req.ClientProtocol)),
			zap.String("provider_protocol", string(req.Candidate.Protocol)),
			zap.String("route_id", req.Candidate.RouteID),
			zap.Int("first_byte_ms", req.FirstTokenMs),
		)...,
	)

	var auditBuf strings.Builder
	accumulatedOutputBytes := 0

	emitFrames := func(frames []corebridge.StreamFrame) error {
		for _, fr := range frames {
			fr.Text = string(s.restorePII(req, []byte(fr.Text)))
			if fr.Model != "" {
				fr.Model = req.PublicModel() // 对外模型名；替代 egress 模型重写
			}
			switch fr.Event {
			case corebridge.EvTextDelta, corebridge.EvReasoningDelta:
				auditBuf.WriteString(fr.Text)
				accumulatedOutputBytes += len(fr.Text)
			}
			if fr.HasFinish && fr.Usage != nil {
				applyCanonicalUsage(req, fr.Usage)
			}
			out, eerr := emitter.Emit(fr)
			if eerr != nil || len(out) == 0 {
				continue
			}
			if _, werr := w.Write(out); werr != nil {
				return werr
			}
		}
		return nil
	}
	pushLine := func(line []byte) error {
		frames, perr := provider.PushLine(line)
		if perr != nil {
			return nil // 解析瑕疵不致命，继续
		}
		return emitFrames(frames)
	}

	// 回放已消费的前导行 + 首帧。
	for _, line := range consumed {
		if err := pushLine(line); err != nil {
			return streamClientWriteError(req, err)
		}
	}
	flusher.Flush()

	if awaitErr != nil {
		return s.finishStreamConvert(req, startTime, &auditBuf, accumulatedOutputBytes, awaitErr, dc, w, flusher, provider, emitFrames, emitter)
	}

	// ---- Phase 4 传输 ----
	for {
		line, readErr := readSSELine(reader, maxSSELineBytes)
		if len(line) > 0 {
			dc.chunkReceived()
			if err := pushLine(line); err != nil {
				return streamClientWriteError(req, err)
			}
			flusher.Flush()
		}
		if readErr != nil {
			return s.finishStreamConvert(req, startTime, &auditBuf, accumulatedOutputBytes, readErr, dc, w, flusher, provider, emitFrames, emitter)
		}
	}
}

func (s *ExecuteStep) restorePII(req *Request, body []byte) []byte {
	if s == nil || s.Privacy == nil || req == nil || req.PIIMap == nil {
		return body
	}
	return s.Privacy.RestoreJSON(body, req.PIIMap)
}

// finishStreamConvert ends a committed converted stream: on clean EOF it flushes
// the provider's trailing finish frame and the emitter's terminator (e.g. OpenAI
// [DONE] / Claude message_stop); on any other error it emits a client-protocol
// error frame (post-commit failure, no failover).
func (s *ExecuteStep) finishStreamConvert(
	req *Request, startTime time.Time, auditBuf *strings.Builder, accumulatedOutputBytes int,
	readErr error, dc *deadlineController, w http.ResponseWriter, flusher http.Flusher,
	provider corebridge.StreamProvider, emitFrames func([]corebridge.StreamFrame) error, emitter corebridge.StreamEmitter,
) error {
	if readErr == io.EOF {
		if frames, ferr := provider.Finish(); ferr == nil && len(frames) > 0 {
			_ = emitFrames(frames)
		}
		if tail, ferr := emitter.Finish(); ferr == nil && len(tail) > 0 {
			_, _ = w.Write(tail)
		}
		flusher.Flush()
		req.AuditResponseMessage = []byte(auditBuf.String())
		fillEstimatedUsage(req, accumulatedOutputBytes)
		req.RequestStatus = domain.RequestSuccess
		req.HTTPStatus = http.StatusOK
		zap.L().Info("stream finished (convert)",
			requestLogFields(req,
				zap.String("model_code", req.ModelCode),
				zap.Int("first_token_ms", req.FirstTokenMs),
				zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
				zap.Int("prompt_tokens", req.TokenUsage.PromptTokens),
				zap.Int("completion_tokens", req.TokenUsage.CompletionTokens),
			)...,
		)
		return nil
	}

	req.AuditResponseMessage = []byte(auditBuf.String())
	fillEstimatedUsage(req, accumulatedOutputBytes)
	code, msg := streamFailureReason(dc, readErr)
	req.FailedStep = "execute"
	req.InternalErrorDetail = RedactInternalErrorDetail(msg)
	msg = egress.SanitizeText(msg, PublicEgressPolicy(req))
	req.RequestStatus = domain.RequestFailed
	req.HTTPStatus = http.StatusOK
	req.ErrorCode = code
	req.ErrorMessage = msg
	if _, werr := w.Write(s.Bridge.StreamErrorFrame(req, code, msg)); werr == nil {
		flusher.Flush()
	}
	zap.L().Warn("stream aborted after commit (convert)",
		requestLogFields(req,
			zap.String("error_code", code),
			zap.String("error", msg),
			zap.Int("total_ms", int(time.Since(startTime).Milliseconds())),
		)...,
	)
	return &postcommitError{code: code, message: msg}
}

// executeImageRelay is the unified image-generation relay. It reads the whole
// upstream response (image bodies are small and aggregatable), collapses any
// SSE into a single provider-format body, converts provider→client when the
// protocols differ (passthrough otherwise), and re-emits in the client's shape:
// a JSON object when the client asked for sync, or a single-frame SSE stream
// when the client asked to stream. This is what decouples the upstream
// transport (binding ImageStreamMode) from the client transport (req.IsStream).
