package bridgefmt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/runtimecompat"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/upstreamcompat"
)

const (
	outboundUserAgentMaxBytes = 512
	serviceVersion            = "1.0.0"
	webOutboundUserAgent      = "doustack-web/" + serviceVersion
)

var _ serving.ProtocolBridge = (*Runtime)(nil)

func (r *Runtime) BridgeRequest(req *serving.Request, body []byte) ([]byte, error) {
	return r.ConvertRequest(requestEnvelope(req), body)
}

func (r *Runtime) PrepareRequest(req *serving.Request, body []byte) (corebridge.PreparedRequest, error) {
	var (
		plan corebridge.PreparedRequest
		err  error
	)
	captureImageClientResponseFormat(req, body)
	if imagePrepareRequest(req) {
		plan, err = r.prepareImageRequest(req, body)
	} else if req.ClientProtocol != req.Candidate.Protocol {
		plan.Body, err = r.BridgeRequest(req, body)
	} else {
		contentType := ""
		if req.Envelope != nil && req.Envelope.R != nil {
			contentType = req.Envelope.R.Header.Get("Content-Type")
		}
		plan.Body, err = r.RewritePassthroughBody(body, req.Candidate.UpstreamModel, contentType, req.Candidate.FixedProviderType)
	}
	if err != nil {
		return corebridge.PreparedRequest{}, err
	}
	if plan.ContentType == "" && req.Envelope != nil && req.Envelope.R != nil && isMultipartContentType(req.Envelope.R.Header.Get("Content-Type")) {
		plan.ContentType = req.Envelope.R.Header.Get("Content-Type")
	}
	// OpenAI image upstreams read the transport mode from the request body's
	// `stream` field rather than the Accept header. Force it to the
	// upstream-facing decision so binding ImageStreamMode (force_sync/stream)
	// actually governs how we call the provider, independent of the client.
	// The caller response preference is handled after the upstream response. Any
	// client response_format is replaced here by the binding-owned upstream
	// preference, or removed when the binding leaves it unset.
	if isImageCapability(runtimecompat.CapabilityToCore(req.CapabilityType)) &&
		req.Candidate != nil && req.Candidate.Protocol == domain.ProtocolOpenAIImages {
		responseFormat := imageUpstreamResponseFormat(req.Candidate)
		if !isMultipartContentType(plan.ContentType) {
			plan.Body = rewriteOpenAIImageJSONFields(plan.Body, req.UpstreamStream(), responseFormat)
		} else {
			plan.Body, plan.ContentType, err = rewriteOpenAIImageMultipartFields(plan.Body, plan.ContentType, map[string]string{
				"stream":          strconv.FormatBool(req.UpstreamStream()),
				"response_format": responseFormat,
			})
			if err != nil {
				return corebridge.PreparedRequest{}, err
			}
		}
	}
	return plan, nil
}

func imageUpstreamResponseFormat(candidate *domain.RouteCandidate) string {
	if candidate == nil {
		return ""
	}
	switch strings.TrimSpace(candidate.ImageUpstreamResponseFormat) {
	case domain.ImageResponseFormatURL:
		return domain.ImageResponseFormatURL
	case domain.ImageResponseFormatB64:
		return domain.ImageResponseFormatB64
	default:
		return ""
	}
}

func rewriteOpenAIImageMultipartFields(body []byte, contentType string, values map[string]string) ([]byte, string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return body, contentType, nil
	}
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, readErr := reader.NextPart()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = writer.Close()
			return nil, "", readErr
		}
		name := part.FormName()
		if _, replace := values[name]; replace {
			_ = part.Close()
			continue
		}
		headers := cloneMIMEHeader(part.Header)
		copyPart, createErr := writer.CreatePart(headers)
		if createErr != nil {
			_ = part.Close()
			_ = writer.Close()
			return nil, "", createErr
		}
		if _, copyErr := io.Copy(copyPart, part); copyErr != nil {
			_ = part.Close()
			_ = writer.Close()
			return nil, "", copyErr
		}
		_ = part.Close()
	}
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			_ = writer.Close()
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), writer.FormDataContentType(), nil
}

func cloneMIMEHeader(headers textproto.MIMEHeader) textproto.MIMEHeader {
	clone := make(textproto.MIMEHeader, len(headers))
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

func captureImageClientResponseFormat(req *serving.Request, body []byte) {
	if req == nil || req.CapabilityType != domain.CapabilityImage || req.ClientProtocol != domain.ProtocolOpenAIImages {
		return
	}
	req.ImageClientResponseFormat = domain.ImageResponseFormatB64
	contentType := ""
	if req.Envelope != nil && req.Envelope.R != nil {
		contentType = req.Envelope.R.Header.Get("Content-Type")
	}
	if format := imageResponseFormatFromMultipart(body, contentType); format != "" {
		req.ImageClientResponseFormat = format
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil || doc == nil {
		return
	}
	if format, _ := doc["response_format"].(string); format == domain.ImageResponseFormatURL || format == domain.ImageResponseFormatB64 {
		req.ImageClientResponseFormat = format
	}
}

func imageResponseFormatFromMultipart(body []byte, contentType string) string {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, readErr := reader.NextPart()
		if readErr != nil {
			return ""
		}
		if part.FormName() != "response_format" {
			_ = part.Close()
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, 1024))
		_ = part.Close()
		if err != nil {
			return ""
		}
		format := strings.TrimSpace(string(value))
		if format == domain.ImageResponseFormatURL || format == domain.ImageResponseFormatB64 {
			return format
		}
		return ""
	}
}

func (r *Runtime) BridgeResponse(req *serving.Request, body []byte) ([]byte, error) {
	return r.ConvertResponse(responseEnvelope(req), body)
}

func (r *Runtime) BuildUpstreamRequest(req *serving.Request, prepared corebridge.PreparedRequest) (*serving.UpstreamRequest, error) {
	if req == nil || req.Candidate == nil {
		return nil, errors.New("bridgefmt: request and route candidate are required")
	}
	meta := buildUpstreamCompatMeta(req)
	candidate := req.Candidate
	if prepared.RequestPath != "" {
		cloned := *req.Candidate
		cloned.RequestPath = prepared.RequestPath
		candidate = &cloned
	}
	body, err := upstreamcompat.ApplyRequestBodyTransform(candidate, meta, prepared.Body)
	if err != nil {
		return nil, err
	}
	headers := upstreamcompat.BuildHeaders(candidate, meta)
	if prepared.ContentType != "" {
		headers["Content-Type"] = prepared.ContentType
	}
	url, err := upstreamcompat.BuildURL(candidate, meta)
	if err != nil {
		return nil, err
	}
	return &serving.UpstreamRequest{
		Method:   "POST",
		URL:      url,
		Headers:  headers,
		Body:     body,
		Protocol: candidate.Protocol,
	}, nil
}

func (r *Runtime) AggregateImageProviderBody(req *serving.Request, rawBody []byte) ([]byte, error) {
	return aggregateImageProviderBody(req, rawBody)
}

func (r *Runtime) BuildImageClientStream(req *serving.Request, clientBody []byte) ([]byte, error) {
	return buildImageClientStream(req, clientBody)
}

func (r *Runtime) NewProvider(req *serving.Request) (corebridge.StreamProvider, error) {
	return r.NewStreamProvider(responseEnvelope(req), req.Candidate.UpstreamModel)
}

func (r *Runtime) NewEmitter(req *serving.Request) (corebridge.StreamEmitter, error) {
	return r.NewStreamEmitter(responseEnvelope(req))
}

func (r *Runtime) ExtractSyncUsage(req *serving.Request, body []byte) domain.TokenUsage {
	return r.ExtractSyncUsageForProtocol(req.Candidate.Protocol, body)
}

func (r *Runtime) ExtractStreamUsage(req *serving.Request, prev domain.TokenUsage, data []byte, eventType string) (domain.TokenUsage, bool) {
	return r.ExtractStreamUsageForProtocol(req.Candidate.Protocol, prev, data, eventType)
}

func (r *Runtime) NormalizeResponseBody(req *serving.Request, body []byte) []byte {
	return upstreamcompat.UnwrapResponseBody(req.Candidate, body)
}

func (r *Runtime) StreamErrorFrame(req *serving.Request, code, msg string) []byte {
	return r.BuildStreamErrorFrame(req.ClientProtocol, code, msg)
}

func requestEnvelope(req *serving.Request) corebridge.RequestEnvelope {
	capability := runtimecompat.CapabilityToCore(req.CapabilityType)
	return corebridge.RequestEnvelope{
		Context:     requestContext(req),
		Capability:  capability,
		Kind:        bridgeKindForCapability(capability),
		ClientModel: req.PublicModel(),
		TargetModel: req.Candidate.UpstreamModel,
		Source:      runtimecompat.MustProtocolToSurfaceForCapability(req.ClientProtocol, capability),
		Target:      runtimecompat.MustProtocolToSurfaceForCapability(req.Candidate.Protocol, capability),
		RequestedAt: req.StartedAt,
		Metadata: map[string]any{
			bridgeMetaClientPath:         bridgeClientPath(req),
			bridgeMetaContentType:        bridgeContentType(req),
			bridgeMetaImageEditTransport: req.Candidate.ImageEditTransport,
		},
		Stream: req.UpstreamStream(),
	}
}

func requestContext(req *serving.Request) context.Context {
	if req != nil && req.Envelope != nil && req.Envelope.R != nil {
		return req.Envelope.R.Context()
	}
	return context.Background()
}

func responseEnvelope(req *serving.Request) corebridge.ResponseEnvelope {
	capability := runtimecompat.CapabilityToCore(req.CapabilityType)
	return corebridge.ResponseEnvelope{
		Capability: capability,
		Kind:       bridgeKindForCapability(capability),
		Source:     runtimecompat.MustProtocolToSurfaceForCapability(req.Candidate.Protocol, capability),
		Target:     runtimecompat.MustProtocolToSurfaceForCapability(req.ClientProtocol, capability),
		Model:      req.PublicModel(),
		ReceivedAt: time.Now(),
		StatusCode: req.UpstreamStatus,
	}
}

func buildUpstreamCompatMeta(req *serving.Request) upstreamcompat.RequestMeta {
	meta := upstreamcompat.RequestMeta{
		IsStream:           req != nil && req.UpstreamStream(),
		ClientPath:         bridgeClientPath(req),
		ContentType:        bridgeContentType(req),
		RequestID:          bridgeRequestID(req),
		SelectedCredential: bridgeSelectedCredential(req),
		StickyIdentity:     bridgeStickyIdentity(req),
		OutboundUserAgent:  bridgeOutboundUserAgent(req),
	}
	if req != nil && req.Envelope != nil && req.Envelope.R != nil {
		meta.IncomingAnthropicBeta = req.Envelope.R.Header.Get("anthropic-beta")
	}
	return meta
}

func bridgeClientPath(req *serving.Request) string {
	if req == nil {
		return ""
	}
	return req.ClientPath
}

func bridgeContentType(req *serving.Request) string {
	if req == nil || req.Envelope == nil || req.Envelope.R == nil {
		return ""
	}
	return req.Envelope.R.Header.Get("Content-Type")
}

func bridgeRequestID(req *serving.Request) string {
	if req == nil {
		return ""
	}
	return req.RequestID
}

func bridgeSelectedCredential(req *serving.Request) *domain.OAuthCredential {
	if req == nil {
		return nil
	}
	return req.SelectedCredential
}

func bridgeStickyIdentity(req *serving.Request) string {
	if req == nil {
		return ""
	}
	if subject := req.RuntimeSubject(); subject != nil {
		if subject.APIKeyID != "" {
			return subject.APIKeyID
		}
		return string(subject.RequestSource) + ":" + string(subject.Scope) + ":" + subject.TenantID + ":" + subject.UserID
	}
	return "web:" + req.RequestID
}

func bridgeOutboundUserAgent(req *serving.Request) string {
	if req == nil || req.Envelope == nil || req.Envelope.R == nil {
		return ""
	}
	if isWebRuntimeSubject(req.RuntimeSubject()) {
		return webOutboundUserAgent
	}
	ua := strings.TrimSpace(req.Envelope.R.Header.Get("User-Agent"))
	if len(ua) > outboundUserAgentMaxBytes {
		return ua[:outboundUserAgentMaxBytes]
	}
	return ua
}

func isWebRuntimeSubject(subject *coreidentity.Subject) bool {
	if subject == nil {
		return false
	}
	switch subject.RequestSource {
	case coreidentity.RequestSourceWebChat, coreidentity.RequestSourceWebImage:
		return true
	default:
		return false
	}
}

func imageBridgeRequest(req *serving.Request) bool {
	if req == nil || req.Candidate == nil {
		return false
	}
	if !isImageCapability(runtimecompat.CapabilityToCore(req.CapabilityType)) {
		return false
	}
	return (req.ClientProtocol == domain.ProtocolOpenAIImages && req.Candidate.Protocol == domain.ProtocolGeminiGenerate) ||
		(req.ClientProtocol == domain.ProtocolGeminiGenerate && req.Candidate.Protocol == domain.ProtocolOpenAIImages)
}

func imagePrepareRequest(req *serving.Request) bool {
	if imageBridgeRequest(req) {
		return true
	}
	if req == nil || req.Candidate == nil || req.Envelope == nil || req.Envelope.R == nil {
		return false
	}
	if !isImageCapability(runtimecompat.CapabilityToCore(req.CapabilityType)) {
		return false
	}
	if req.ClientProtocol != domain.ProtocolOpenAIImages || req.Candidate.Protocol != domain.ProtocolOpenAIImages {
		return false
	}
	if !strings.Contains(bridgeClientPath(req), "/images/edits") {
		return false
	}
	return true
}
