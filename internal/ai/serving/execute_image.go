package serving

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"

	"xiaodou/dai/internal/ai/audit"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/egress"
)

func (s *ExecuteStep) executeImageRelay(dc *deadlineController, req *Request, resp *UpstreamResponse, w http.ResponseWriter, startTime time.Time) error {
	if s.Bridge == nil {
		return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode,
			message: "runtime bridge is not configured for image relay"}
	}

	bodyReader := &responseBodyReader{
		r:           resp.Body,
		onFirstByte: dc.firstByte,
		onChunk:     dc.chunkReceived,
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(bodyReader, syncBodyLimit(req)))
	if err != nil {
		if cause := dc.cause(); cause != nil {
			err = cause
		}
		return &precommitError{cause: err, httpStatus: resp.StatusCode, message: "read upstream body: " + err.Error()}
	}
	req.UpstreamResponseBody = bodyBytes

	// Collapse the upstream response into a single provider-format JSON body.
	// If the upstream streamed (SSE) we aggregate its frames; otherwise we only
	// strip provider envelopes (e.g. CodeAssist wrapper for Gemini pools).
	var providerBody []byte
	if looksLikeSSEBody(bodyBytes) {
		providerBody, err = s.Bridge.AggregateImageProviderBody(req, bodyBytes)
		if err != nil {
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				return apiErr
			}
			return &precommitError{cause: err, httpStatus: resp.StatusCode, message: "aggregate image stream: " + err.Error()}
		}
	} else {
		providerBody = s.Bridge.NormalizeResponseBody(req, bodyBytes)
	}

	// A 200 whose body is actually an error object is a failed attempt — fail over.
	if resp.StatusCode == http.StatusOK && payloadIsError(providerBody) {
		return &precommitError{
			cause:      errUpstreamErrorBody,
			httpStatus: http.StatusOK,
			message:    "upstream returned 200 with an error body: " + truncateValidUTF8(string(bytes.TrimSpace(providerBody)), 256),
		}
	}

	// Usage + audit from the provider-format body, keyed on the provider protocol.
	if u := s.Bridge.ExtractSyncUsage(req, providerBody); u.PromptTokens != 0 || u.CompletionTokens != 0 {
		u.ImageCount = req.TokenUsage.ImageCount
		u.ImageResolution = req.TokenUsage.ImageResolution
		u.VideoSeconds = req.TokenUsage.VideoSeconds
		u.VideoResolution = req.TokenUsage.VideoResolution
		req.TokenUsage = u
	}
	req.AuditResponseMessage = audit.ExtractSyncResponseMessage(providerBody, req.Candidate.Protocol)
	fillEstimatedUsage(req, len(providerBody))

	// Translate provider → client (passthrough when the protocols match, since
	// the image response bridge only defines cross-surface conversions).
	clientBody := providerBody
	if req.ClientProtocol != req.Candidate.Protocol {
		converted, cerr := s.Bridge.BridgeResponse(req, providerBody)
		if cerr != nil {
			return &precommitError{cause: cerr, httpStatus: resp.StatusCode, message: "convert image response: " + cerr.Error()}
		}
		clientBody = converted
	}
	if req.ClientProtocol == domain.ProtocolOpenAIImages && s.ImageNormalizer != nil {
		responseFormat := req.ImageClientResponseFormat
		if responseFormat == "" {
			responseFormat = domain.ImageResponseFormatB64
		}
		normalized, nerr := s.ImageNormalizer.NormalizeImageResponse(dc.ctx, clientBody, responseFormat)
		if nerr != nil {
			return &precommitError{cause: nerr, httpStatus: http.StatusBadGateway, message: "normalize image response: " + nerr.Error()}
		}
		clientBody = normalized
	}

	// Sanitize with the CLIENT protocol policy — the bytes are now client-format.
	policy := publicEgressPolicy(req, req.Candidate)
	policy.Protocol = req.ClientProtocol
	clientBody = egress.SanitizeJSON(clientBody, policy)

	if req.IsStream {
		return s.commitImageClientStream(dc, req, w, clientBody, resp.StatusCode, startTime)
	}
	return s.commitImageClientSync(req, w, clientBody, resp.StatusCode)
}

func (s *ExecuteStep) commitImageClientSync(req *Request, w http.ResponseWriter, clientBody []byte, statusCode int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	req.ResponseCommitted = true
	req.MarkFirstResponseByte(time.Now())
	_, _ = w.Write(clientBody)
	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = statusCode
	return nil
}

func (s *ExecuteStep) commitImageClientStream(dc *deadlineController, req *Request, w http.ResponseWriter, clientBody []byte, statusCode int, startTime time.Time) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &precommitError{cause: errStreamNoFlusher, message: errStreamNoFlusher.Error()}
	}
	clientStream, serr := s.Bridge.BuildImageClientStream(req, clientBody)
	if serr != nil {
		var apiErr *APIError
		if errors.As(serr, &apiErr) {
			return apiErr
		}
		return &precommitError{cause: serr, httpStatus: statusCode, message: "build image client stream: " + serr.Error()}
	}
	if len(clientStream) == 0 {
		return &precommitError{cause: errUpstreamEmpty, httpStatus: statusCode, message: "image client stream is empty"}
	}

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
	if _, err := w.Write(clientStream); err != nil {
		return streamClientWriteError(req, err)
	}
	flusher.Flush()
	req.RequestStatus = domain.RequestSuccess
	req.HTTPStatus = http.StatusOK
	return nil
}
