package transport

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/imageedit"
	"xiaodou/dai/internal/ai/imagepayload"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/upstreamcompat"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/libs/go/httpx"
)

// 上游账号连通性测试：管理端选账号+模型，按模型能力(生图/对话)对上游直发一条
// 真实请求并展示返回。纯连通性/可用性验证，不落库、不计费。

type upstreamAccountTestImageInput struct {
	Filename string `json:"filename" doc:"参考图片文件名"`
	MIMEType string `json:"mime_type" doc:"参考图片 MIME 类型；支持 image/png、image/jpeg、image/webp"`
	B64JSON  string `json:"b64_json" doc:"参考图片 Base64 内容（不含 data URL 前缀）"`
}

type upstreamAccountTestInput struct {
	AccountID string `path:"accountID" doc:"上游账号 ID"`
	Body      struct {
		ModelCode string                         `json:"model_code" doc:"要测试的对外 model_code（账号下的显式绑定）"`
		APIFormat string                         `json:"api_format" doc:"要测试的账号端点 API 格式"`
		Prompt    string                         `json:"prompt,omitempty" doc:"测试提示词；生图/对话通用，为空用默认值"`
		ImageEdit bool                           `json:"image_edit,omitempty" doc:"仅 OpenAI Images 生图模型：执行图片编辑测试"`
		Image     *upstreamAccountTestImageInput `json:"image,omitempty" doc:"图片编辑测试使用的真实参考图片；image_edit=true 时必填"`
	}
}

type upstreamAccountTestOutput struct {
	Body struct {
		ResponseContentType         string `json:"response_content_type,omitempty" doc:"上游响应 Content-Type"`
		UpstreamRequestID           string `json:"upstream_request_id,omitempty" doc:"上游返回的请求 ID，便于排查"`
		ErrorCode                   string `json:"error_code,omitempty" doc:"诊断错误代码"`
		OK                          bool   `json:"ok" doc:"上游是否返回可用结果"`
		HTTPStatus                  int    `json:"http_status" doc:"上游 HTTP 状态码"`
		LatencyMs                   int64  `json:"latency_ms" doc:"往返耗时(毫秒)"`
		Capability                  string `json:"capability" doc:"测试能力：chat / image / embedding"`
		APIFormat                   string `json:"api_format" doc:"使用的上游 API 格式"`
		UpstreamModel               string `json:"upstream_model" doc:"上游真实模型名"`
		ReplyText                   string `json:"reply_text,omitempty" doc:"对话测试的回复文本"`
		ImageB64                    string `json:"image_b64,omitempty" doc:"生图测试返回的 base64 图片"`
		ImageMime                   string `json:"image_mime,omitempty" doc:"生图 MIME 类型"`
		ImageURL                    string `json:"image_url,omitempty" doc:"生图测试返回的图片 URL(若上游直接给 url)"`
		ImageStreamMode             string `json:"image_stream_mode,omitempty" doc:"本次生图测试使用的上游流式策略"`
		ImageEditTransport          string `json:"image_edit_transport,omitempty" doc:"本次图片编辑测试使用的上游请求格式"`
		ImageUpstreamResponseFormat string `json:"image_upstream_response_format,omitempty" doc:"本次生图测试向上游发送的 response_format；为空表示未发送"`
		ActualImageFormat           string `json:"actual_image_format,omitempty" doc:"上游实际返回的图片格式"`
		PromptTokens                int64  `json:"prompt_tokens,omitempty" doc:"输入 token"`
		OutputTokens                int64  `json:"output_tokens,omitempty" doc:"输出 token"`
		TotalTokens                 int64  `json:"total_tokens,omitempty" doc:"总 token"`
		Error                       string `json:"error,omitempty" doc:"失败原因(上游报错/解析失败)"`
	}
}

type upstreamTestBinding struct {
	UpstreamModel  string
	CapabilityType string
	ImagePolicy    imageGenerationBindingPolicy
}

func registerUpstreamAccountTest(api huma.API, d UpstreamDiagnosticsHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-test-account-upstream",
		Method:      http.MethodPost,
		Path:        "/api/v1/upstream-accounts/{accountID}/test",
		Summary:     "测试上游账号连通性",
		Description: "按所选模型的能力(生图/对话)对上游直发一条真实请求并返回结果，不计费。401 会把非停用账号标记为 invalid；invalid 账号验证成功后恢复 active。",
		Tags:        []string{"upstream-accounts"},
	}, func(ctx context.Context, in *upstreamAccountTestInput) (*upstreamAccountTestOutput, error) {
		if d.AccountReader == nil || d.ModelBindings == nil || d.ProviderSecrets == nil {
			return nil, httpx.ErrUnavailable.WithDetail("database or provider secret codec is not configured")
		}
		_, err := parseTransportUUID(in.AccountID)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("invalid accountID")
		}
		modelCode := strings.TrimSpace(in.Body.ModelCode)
		if modelCode == "" {
			return nil, httpx.ErrBadRequest.WithDetail("model_code is required")
		}
		account, err := d.AccountReader.GetAccountSecret(ctx, in.AccountID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		apiKey, err := d.ProviderSecrets.Decrypt(account.Ciphertext)
		if err != nil {
			return nil, httpx.ErrInternal.WithDetail("failed to decrypt api key")
		}
		binding, err := loadUpstreamTestBinding(ctx, d.ModelBindings, in.AccountID, modelCode)
		if err != nil {
			return nil, err
		}
		var image *imageedit.Source
		if in.Body.ImageEdit {
			decoded, decodeErr := decodeUpstreamTestImageInput(in.Body.Image)
			if decodeErr != nil {
				return nil, httpx.ErrBadRequest.WithDetail(decodeErr.Error())
			}
			image = &decoded
		}
		endpoint, err := selectUpstreamTestEndpoint(account.Endpoints, in.Body.APIFormat)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		if !domain.ProtocolSupportsCapability(endpoint.APIFormat, domain.CapabilityType(binding.CapabilityType)) {
			return nil, httpx.ErrBadRequest.WithDetail("selected endpoint API format is incompatible with the model capability")
		}
		result := runUpstreamAccountTest(ctx, d.HTTPClient, upstreamTestConfig{
			BaseURL:                     endpoint.BaseURL,
			PathOverride:                endpoint.PathOverride,
			APIKey:                      apiKey,
			AuthScheme:                  endpoint.AuthScheme,
			AuthHeader:                  endpoint.AuthHeader,
			ExtraHeaders:                endpoint.ExtraHeaders,
			APIFormat:                   string(endpoint.APIFormat),
			UpstreamModel:               binding.UpstreamModel,
			Capability:                  binding.CapabilityType,
			Prompt:                      strings.TrimSpace(in.Body.Prompt),
			ImageEdit:                   in.Body.ImageEdit,
			Image:                       image,
			ImageStreamMode:             binding.ImagePolicy.StreamMode,
			ImageEditTransport:          binding.ImagePolicy.EditTransport,
			ImageUpstreamResponseFormat: binding.ImagePolicy.UpstreamResponseFormat,
			Timeouts:                    domain.DefaultRouteTimeouts(domain.CapabilityType(binding.CapabilityType)),
		})
		redactUpstreamDiagnosticResultSecrets(&result, apiKey, endpoint.ExtraHeaders)
		if err := reconcileUpstreamAccountTestStatus(ctx, d.AccountHealth, d.EndpointManager, d.RuntimeHealth, in.AccountID, endpoint.ID, account.Status, result); err != nil {
			return nil, mapServiceError(err)
		}
		out := &upstreamAccountTestOutput{}
		out.Body.ResponseContentType = result.ResponseContentType
		out.Body.UpstreamRequestID = result.UpstreamRequestID
		out.Body.ErrorCode = result.ErrorCode
		out.Body.OK = result.OK
		out.Body.HTTPStatus = result.HTTPStatus
		out.Body.LatencyMs = result.LatencyMs
		out.Body.Capability = result.Capability
		out.Body.APIFormat = string(endpoint.APIFormat)
		out.Body.UpstreamModel = binding.UpstreamModel
		out.Body.ReplyText = result.ReplyText
		out.Body.ImageB64 = result.ImageB64
		out.Body.ImageMime = result.ImageMime
		out.Body.ImageURL = result.ImageURL
		out.Body.ImageStreamMode = result.ImageStreamMode
		out.Body.ImageEditTransport = result.ImageEditTransport
		out.Body.ImageUpstreamResponseFormat = result.ImageUpstreamResponseFormat
		out.Body.ActualImageFormat = result.ActualImageFormat
		out.Body.PromptTokens = result.PromptTokens
		out.Body.OutputTokens = result.OutputTokens
		out.Body.TotalTokens = result.TotalTokens
		out.Body.Error = result.Error
		return out, nil
	})
}

func redactUpstreamDiagnosticSecrets(message, apiKey string, extraHeaders []byte) string {
	if message == "" {
		return ""
	}
	secrets := []string{apiKey}
	var headers map[string]string
	if json.Unmarshal(extraHeaders, &headers) == nil {
		for key, value := range headers {
			if upstreamcontrol.IsSensitiveHeaderKey(key) {
				secrets = append(secrets, value)
			}
		}
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return message
}

func redactUpstreamDiagnosticResultSecrets(result *upstreamTestResult, apiKey string, extraHeaders []byte) {
	if result == nil {
		return
	}
	for _, field := range []*string{
		&result.ResponseContentType,
		&result.UpstreamRequestID,
		&result.ReplyText,
		&result.ImageMime,
		&result.ImageURL,
		&result.Error,
	} {
		*field = redactUpstreamDiagnosticSecrets(*field, apiKey, extraHeaders)
	}
}

func reconcileUpstreamAccountTestStatus(ctx context.Context, health UpstreamAccountHealthWriter, endpoints UpstreamAccountEndpointManager, runtimeHealth routing.Availability, accountID, endpointID, currentStatus string, result upstreamTestResult) error {
	if endpoints != nil {
		status, lastError := domain.HealthHealthy, ""
		if !result.OK {
			status, lastError = domain.HealthUnhealthy, result.Error
			if result.ProbeBlocked {
				status = domain.HealthUnknown
			}
		}
		if _, err := endpoints.UpdateEndpointHealth(ctx, accountID, endpointID, status, lastError); err != nil {
			return err
		}
	}
	// A manual test is diagnostic only; it cannot clear faults in untested models.
	if health == nil || currentStatus == domain.UpstreamAccountStatusDisabled {
		return nil
	}
	if result.OK && currentStatus == domain.UpstreamAccountStatusInvalid {
		_, err := health.UpdateAccountStatus(ctx, accountID, domain.UpstreamAccountStatusActive)
		return err
	}
	if result.HTTPStatus == http.StatusUnauthorized {
		if suspend, ok := runtimeHealth.(interface {
			Suspend(context.Context, routing.FaultScope, time.Time, string) error
		}); ok {
			if err := suspend.Suspend(ctx, routing.NewFaultScope("authentication", "direct_upstream", accountID, "", "", "", ""), time.Now().Add(30*time.Minute), "unauthorized"); err != nil {
				return err
			}
		}
		_, err := health.MarkAccountInvalid(ctx, accountID, fmt.Sprintf("connectivity test: upstream returned HTTP %d", result.HTTPStatus))
		return err
	}
	return nil
}

func loadUpstreamTestBinding(ctx context.Context, store UpstreamModelBindingStore, accountID, modelCode string) (upstreamTestBinding, error) {
	if store == nil {
		return upstreamTestBinding{}, httpx.ErrUnavailable.WithDetail("model binding store is not configured")
	}
	binding, err := store.FindByModel(ctx, domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindDirect, ID: accountID}, modelCode)
	if err != nil {
		return upstreamTestBinding{}, mapServiceError(err)
	}
	b := upstreamTestBinding{
		UpstreamModel:  binding.UpstreamModelName,
		CapabilityType: binding.CapabilityType,
		ImagePolicy:    parseImageGenerationBindingPolicy(binding.ConfigJSON),
	}
	if strings.TrimSpace(b.UpstreamModel) == "" {
		b.UpstreamModel = modelCode
	}
	return b, nil
}

func selectUpstreamTestEndpoint(endpoints []domain.UpstreamAccountEndpoint, requested string) (domain.UpstreamAccountEndpoint, error) {
	requested = strings.TrimSpace(requested)
	active := make([]domain.UpstreamAccountEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Status != domain.EndpointStatusActive {
			continue
		}
		active = append(active, endpoint)
		if requested != "" && string(endpoint.APIFormat) == requested {
			return endpoint, nil
		}
	}
	if requested != "" {
		return domain.UpstreamAccountEndpoint{}, fmt.Errorf("active endpoint not found for api_format %q", requested)
	}
	if len(active) == 1 {
		return active[0], nil
	}
	if len(active) == 0 {
		return domain.UpstreamAccountEndpoint{}, fmt.Errorf("account has no active endpoint")
	}
	return domain.UpstreamAccountEndpoint{}, fmt.Errorf("api_format is required when account has multiple active endpoints")
}

type upstreamTestConfig struct {
	BaseURL                     string
	PathOverride                string
	APIKey                      string
	AuthScheme                  string
	AuthHeader                  string
	ExtraHeaders                []byte
	APIFormat                   string
	UpstreamModel               string
	Capability                  string
	Prompt                      string
	ImageEdit                   bool
	Image                       *imageedit.Source
	ImageStreamMode             string
	ImageEditTransport          string
	ImageUpstreamResponseFormat string
	Timeouts                    domain.RouteTimeouts
}

type upstreamTestResult struct {
	ResponseContentType         string
	UpstreamRequestID           string
	ErrorCode                   string
	ProbeBlocked                bool
	OK                          bool
	HTTPStatus                  int
	LatencyMs                   int64
	Capability                  string
	ReplyText                   string
	ImageB64                    string
	ImageMime                   string
	ImageURL                    string
	ImageStreamMode             string
	ImageEditTransport          string
	ImageUpstreamResponseFormat string
	ActualImageFormat           string
	PromptTokens                int64
	OutputTokens                int64
	TotalTokens                 int64
	Error                       string
}

const (
	defaultTestImagePrompt                      = "Generate a cute orange cat astronaut sticker on a clean pastel background."
	upstreamTestOpenAIImageSize                 = "1024x1024"
	upstreamTestGeminiImageSize                 = "1K"
	upstreamTestImageResponseMatched            = "matched"
	upstreamTestImageResponseConversionRequired = "runtime_conversion_required"
)

var (
	errUpstreamTestResponseHeaderTimeout = errors.New("response-header timeout: upstream did not return response headers in time")
	errUpstreamTestFirstByteTimeout      = errors.New("first-byte timeout: no response body from upstream in time")
	errUpstreamTestIdleTimeout           = errors.New("idle timeout: upstream stream stalled between chunks")
	errUpstreamTestMaxDuration           = errors.New("max-duration timeout: response exceeded total time budget")
)

func normalizeUpstreamTestTimeouts(capability domain.CapabilityType, timeouts domain.RouteTimeouts) domain.RouteTimeouts {
	defaults := domain.DefaultRouteTimeouts(capability)
	if timeouts.ResponseHeader <= 0 {
		timeouts.ResponseHeader = defaults.ResponseHeader
	}
	if timeouts.FirstByte <= 0 {
		timeouts.FirstByte = defaults.FirstByte
	}
	if timeouts.Idle <= 0 {
		timeouts.Idle = defaults.Idle
	}
	if timeouts.MaxDuration <= 0 {
		timeouts.MaxDuration = defaults.MaxDuration
	}
	return timeouts
}

type upstreamTestDeadline struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	t      domain.RouteTimeouts

	mu        sync.Mutex
	phase     *time.Timer
	maximum   *time.Timer
	firstByte bool
	stopped   bool
}

func newUpstreamTestDeadline(parent context.Context, timeouts domain.RouteTimeouts) *upstreamTestDeadline {
	ctx, cancel := context.WithCancelCause(parent)
	d := &upstreamTestDeadline{ctx: ctx, cancel: cancel, t: timeouts}
	d.phase = time.AfterFunc(timeouts.ResponseHeader, func() { cancel(errUpstreamTestResponseHeaderTimeout) })
	d.maximum = time.AfterFunc(timeouts.MaxDuration, func() { cancel(errUpstreamTestMaxDuration) })
	return d
}

func (d *upstreamTestDeadline) armPhaseLocked(duration time.Duration, cause error) {
	if d.phase != nil {
		d.phase.Stop()
	}
	d.phase = time.AfterFunc(duration, func() { d.cancel(cause) })
}

func (d *upstreamTestDeadline) headersReceived() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	d.armPhaseLocked(d.t.FirstByte, errUpstreamTestFirstByteTimeout)
}

func (d *upstreamTestDeadline) bodyChunk(streaming bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	if !d.firstByte {
		d.firstByte = true
		if streaming {
			d.armPhaseLocked(d.t.Idle, errUpstreamTestIdleTimeout)
		} else {
			if d.phase != nil {
				d.phase.Stop()
				d.phase = nil
			}
		}
		return
	}
	if streaming {
		d.armPhaseLocked(d.t.Idle, errUpstreamTestIdleTimeout)
	}
}

func (d *upstreamTestDeadline) cause() error {
	switch cause := context.Cause(d.ctx); cause {
	case errUpstreamTestResponseHeaderTimeout, errUpstreamTestFirstByteTimeout, errUpstreamTestIdleTimeout, errUpstreamTestMaxDuration:
		return cause
	default:
		return nil
	}
}

func (d *upstreamTestDeadline) stop() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.stopped = true
	if d.phase != nil {
		d.phase.Stop()
	}
	if d.maximum != nil {
		d.maximum.Stop()
	}
	d.mu.Unlock()
	d.cancel(nil)
}

type upstreamTestBodyReader struct {
	reader    io.Reader
	deadline  *upstreamTestDeadline
	streaming bool
}

func (r *upstreamTestBodyReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.deadline.bodyChunk(r.streaming)
	}
	return n, err
}

func runUpstreamAccountTest(ctx context.Context, client HTTPDoer, cfg upstreamTestConfig) upstreamTestResult {
	if client == nil {
		client = http.DefaultClient
	}
	capability := strings.TrimSpace(cfg.Capability)
	isImage := capability == string(domain.CapabilityImage)
	isEmbedding := capability == string(domain.CapabilityEmbedding)
	res := upstreamTestResult{Capability: "chat"}
	if isImage {
		res.Capability = "image"
		res.ImageStreamMode = normalizedUpstreamTestImageStreamMode(cfg.ImageStreamMode)
		res.ImageEditTransport = normalizedUpstreamTestImageEditTransport(cfg.ImageEditTransport)
		res.ImageUpstreamResponseFormat = normalizedUpstreamTestImageResponseFormat(cfg.ImageUpstreamResponseFormat)
	} else if isEmbedding {
		res.Capability = "embedding"
	}

	prompt := strings.TrimSpace(cfg.Prompt)
	if prompt == "" {
		if isImage {
			prompt = defaultTestImagePrompt
		} else if isEmbedding {
			prompt = "A short sentence for embedding validation."
		} else {
			prompt = defaultTestChatPrompt
		}
	}

	capabilityType := domain.CapabilityType(capability)
	deadline := newUpstreamTestDeadline(ctx, normalizeUpstreamTestTimeouts(capabilityType, cfg.Timeouts))
	defer deadline.stop()

	req, err := buildUpstreamTestRequest(deadline.ctx, cfg, isImage, prompt)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.LatencyMs = time.Since(start).Milliseconds()
		if cause := deadline.cause(); cause != nil {
			res.Error = cause.Error()
		} else {
			res.Error = sanitizeUpstreamFetchError(err)
		}
		return res
	}
	deadline.headersReceived()
	res.ResponseContentType = truncateStr(resp.Header.Get("Content-Type"), 256)
	res.UpstreamRequestID = truncateStr(resp.Header.Get("X-Request-ID"), 256)
	defer resp.Body.Close()
	if estimated, blocked := upstreamcompat.ProbeBlock(resp.Header); blocked && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		res.ProbeBlocked = true
		res.ErrorCode = upstreamcompat.ProbeBlockedCode
		res.HTTPStatus = resp.StatusCode
		res.LatencyMs = time.Since(start).Milliseconds()
		res.Error = "上游将本次请求识别为探测并拦截（x-sub2api-probe-blocked=true），未返回模型结果。请检查上游防探测策略，或使用实际业务内容测试。"
		if estimated > 0 {
			res.Error += fmt.Sprintf("上游估算输入 %d tokens。", estimated)
		}
		return res
	}
	streaming := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") || (!isImage && !isEmbedding && req.Header.Get("Accept") == "text/event-stream") || (isImage && normalizedUpstreamTestImageStreamMode(cfg.ImageStreamMode) == domain.ImageStreamModeForceStream)
	body, readErr := readUpstreamTestBody(&upstreamTestBodyReader{reader: resp.Body, deadline: deadline, streaming: streaming}, cfg.APIFormat, strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream"))
	res.LatencyMs = time.Since(start).Milliseconds()
	res.HTTPStatus = resp.StatusCode
	if readErr != nil {
		if cause := deadline.cause(); cause != nil {
			res.Error = cause.Error()
		} else {
			res.Error = sanitizeUpstreamFetchError(readErr)
		}
		return res
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		res.Error = upstreamTestHTTPError(resp.StatusCode, body)
		if cfg.APIKey != "" {
			res.Error = strings.ReplaceAll(res.Error, cfg.APIKey, "[redacted]")
		}
		return res
	}

	if isImage {
		parseUpstreamTestImageResponse(&res, cfg.APIFormat, body)
	} else if isEmbedding {
		parseUpstreamTestEmbeddingResponse(&res, cfg.APIFormat, body)
	} else {
		parseUpstreamTestChatResponse(&res, cfg.APIFormat, body)
		if !res.OK && !json.Valid(body) && !bytes.Contains(body, []byte("data:")) {
			res.Error = fmt.Sprintf("上游返回了非 JSON/SSE 响应（HTTP %d，Content-Type: %s）。请检查请求端点及上游协议支持。", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	return res
}

func buildUpstreamTestRequest(ctx context.Context, cfg upstreamTestConfig, isImage bool, prompt string) (*http.Request, error) {
	format := strings.TrimSpace(cfg.APIFormat)
	if cfg.ImageEdit && (!isImage || format != string(domain.ProtocolOpenAIImages)) {
		return nil, fmt.Errorf("image edit compatibility test requires an OpenAI Images binding")
	}

	var (
		requestURL  string
		body        []byte
		contentType = "application/json"
	)
	imageStream := normalizedUpstreamTestImageStreamMode(cfg.ImageStreamMode) == domain.ImageStreamModeForceStream
	var buildErr error
	protocol := domain.UpstreamProtocol(format)
	switch {
	case cfg.ImageEdit:
		if cfg.Image == nil || len(cfg.Image.Data) == 0 {
			return nil, fmt.Errorf("image edit compatibility test requires an uploaded image")
		}
		encoded, err := imageedit.EncodeForUpstream(ctx, imageedit.Request{Model: cfg.UpstreamModel, Prompt: prompt, Size: upstreamTestOpenAIImageSize, Stream: imageStream, Images: []imageedit.Source{*cfg.Image}}, normalizedUpstreamTestImageEditTransport(cfg.ImageEditTransport), normalizedUpstreamTestImageResponseFormat(cfg.ImageUpstreamResponseFormat))
		if err != nil {
			return nil, err
		}
		body, contentType = encoded.Body, encoded.ContentType
	case isImage:
		size := upstreamTestOpenAIImageSize
		if protocol == domain.ProtocolGeminiGenerate {
			size = upstreamTestGeminiImageSize
		}
		body, buildErr = upstreamcompat.BuildImageRequest(protocol, cfg.UpstreamModel, prompt, upstreamcompat.ImageOptions{N: 1, Stream: imageStream, Size: size, ResponseFormat: normalizedUpstreamTestImageResponseFormat(cfg.ImageUpstreamResponseFormat)})
	case protocol == domain.ProtocolOpenAIEmbeddings:
		body, buildErr = json.Marshal(map[string]any{"model": cfg.UpstreamModel, "input": prompt})
	case protocol == domain.ProtocolGeminiEmbeddings:
		body, buildErr = json.Marshal(map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": prompt}}}})
	default:
		body, buildErr = upstreamcompat.BuildChatRequest(protocol, cfg.UpstreamModel, []upstreamcompat.Message{{Role: "user", Content: prompt}}, upstreamChatTestStreams(format), 0)
	}
	if buildErr != nil {
		return nil, buildErr
	}
	requestStream := (isImage && imageStream) || upstreamChatTestStreams(format)

	candidate := &domain.RouteCandidate{
		Protocol:           domain.UpstreamProtocol(format),
		BaseURL:            cfg.BaseURL,
		RequestPath:        cfg.PathOverride,
		UpstreamModel:      cfg.UpstreamModel,
		APIKeyCiphertext:   cfg.APIKey,
		EndpointAuthScheme: cfg.AuthScheme,
		EndpointAuthHeader: cfg.AuthHeader,
		ExtraHeaders:       map[string]string{},
	}
	if err := json.Unmarshal(cfg.ExtraHeaders, &candidate.ExtraHeaders); len(cfg.ExtraHeaders) > 0 && err != nil {
		return nil, fmt.Errorf("invalid extra_headers: %w", err)
	}
	clientPath := ""
	if cfg.ImageEdit {
		clientPath = "/v1/images/edits"
	}
	var err error
	requestURL, err = upstreamcompat.BuildURL(candidate, upstreamcompat.RequestMeta{ClientPath: clientPath})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if requestStream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	for key, value := range upstreamcompat.BuildHeaders(candidate, upstreamcompat.RequestMeta{ContentType: contentType, IsStream: requestStream}) {
		req.Header.Set(key, value)
	}
	return req, nil
}

func decodeUpstreamTestImageInput(input *upstreamAccountTestImageInput) (imageedit.Source, error) {
	if input == nil || strings.TrimSpace(input.B64JSON) == "" {
		return imageedit.Source{}, fmt.Errorf("image is required for an image edit test")
	}
	data, detectedMIME, err := imagepayload.DecodeBase64ImageBytes(input.B64JSON, "")
	if err != nil {
		return imageedit.Source{}, fmt.Errorf("image must be a valid PNG, JPEG, or WebP file")
	}
	if int64(len(data)) > imagepayload.MaxImageRawInputBytes {
		return imageedit.Source{}, fmt.Errorf("image must not exceed %d MiB", imagepayload.MaxImageRawInputBytes>>20)
	}
	detectedMIME = normalizedUpstreamTestImageMIME(detectedMIME)
	switch detectedMIME {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return imageedit.Source{}, fmt.Errorf("image must be a valid PNG, JPEG, or WebP file")
	}
	declaredMIME := normalizedUpstreamTestImageMIME(input.MIMEType)
	if declaredMIME != "" && declaredMIME != detectedMIME {
		return imageedit.Source{}, fmt.Errorf("image MIME type does not match the uploaded file")
	}
	filename := filepath.Base(strings.TrimSpace(input.Filename))
	if filename == "" || filename == "." {
		extensions, _ := mime.ExtensionsByType(detectedMIME)
		extension := ".png"
		if len(extensions) > 0 {
			extension = extensions[0]
		}
		filename = "reference" + extension
	}
	return imageedit.Source{Data: data, MIMEType: detectedMIME, Filename: filename}, nil
}

func normalizedUpstreamTestImageMIME(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	if value == "image/jpg" {
		return "image/jpeg"
	}
	return value
}

func parseUpstreamTestChatJSON(res *upstreamTestResult, format string, body []byte) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		res.Error = "parse response failed: " + err.Error()
		return
	}
	if message := upstreamTestDocumentError(doc); message != "" {
		res.Error = message
		return
	}
	switch strings.TrimSpace(format) {
	case string(domain.ProtocolOpenAIResponses):
		res.ReplyText, _ = doc["output_text"].(string)
		if res.ReplyText == "" {
			if output, ok := doc["output"].([]any); ok {
				for _, rawItem := range output {
					item, _ := rawItem.(map[string]any)
					content, _ := item["content"].([]any)
					for _, rawContent := range content {
						part, _ := rawContent.(map[string]any)
						if text, _ := part["text"].(string); strings.TrimSpace(text) != "" {
							res.ReplyText = text
							break
						}
					}
					if res.ReplyText != "" {
						break
					}
				}
			}
		}
		if usage, ok := doc["usage"].(map[string]any); ok {
			res.PromptTokens = jsonInt(usage["input_tokens"])
			res.OutputTokens = jsonInt(usage["output_tokens"])
			res.TotalTokens = jsonInt(usage["total_tokens"])
		}
	case string(domain.ProtocolGeminiGenerate):
		res.ReplyText = geminiFirstText(doc)
		if usage, ok := doc["usageMetadata"].(map[string]any); ok {
			res.PromptTokens = jsonInt(usage["promptTokenCount"])
			res.OutputTokens = jsonInt(usage["candidatesTokenCount"])
			res.TotalTokens = jsonInt(usage["totalTokenCount"])
		}
	case string(domain.ProtocolAnthropicMessages):
		if content, ok := doc["content"].([]any); ok {
			for _, c := range content {
				if m, ok := c.(map[string]any); ok {
					if s, _ := m["text"].(string); strings.TrimSpace(s) != "" {
						res.ReplyText = s
						break
					}
				}
			}
		}
		if usage, ok := doc["usage"].(map[string]any); ok {
			res.PromptTokens = jsonInt(usage["input_tokens"])
			res.OutputTokens = jsonInt(usage["output_tokens"])
			res.TotalTokens = res.PromptTokens + res.OutputTokens
		}
	default:
		if choices, ok := doc["choices"].([]any); ok && len(choices) > 0 {
			if ch, ok := choices[0].(map[string]any); ok {
				if msg, ok := ch["message"].(map[string]any); ok {
					res.ReplyText, _ = msg["content"].(string)
				}
			}
		}
		if usage, ok := doc["usage"].(map[string]any); ok {
			res.PromptTokens = jsonInt(usage["prompt_tokens"])
			res.OutputTokens = jsonInt(usage["completion_tokens"])
			res.TotalTokens = jsonInt(usage["total_tokens"])
		}
	}
	if res.ReplyText == "" {
		res.ReplyText = upstreamTestToolReply(doc)
	}
	res.OK = strings.TrimSpace(res.ReplyText) != ""
	if !res.OK && res.Error == "" {
		res.Error = "upstream returned no text content"
	}
}

func parseUpstreamTestEmbeddingResponse(res *upstreamTestResult, format string, body []byte) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		res.Error = "parse response failed: " + err.Error()
		return
	}
	var values []any
	if format == string(domain.ProtocolGeminiEmbeddings) {
		if embedding, ok := doc["embedding"].(map[string]any); ok {
			values, _ = embedding["values"].([]any)
		}
		if usage, ok := doc["usageMetadata"].(map[string]any); ok {
			res.PromptTokens = jsonInt(usage["promptTokenCount"])
			res.TotalTokens = jsonInt(usage["totalTokenCount"])
		}
	} else {
		if data, ok := doc["data"].([]any); ok && len(data) > 0 {
			if item, ok := data[0].(map[string]any); ok {
				values, _ = item["embedding"].([]any)
			}
		}
		if usage, ok := doc["usage"].(map[string]any); ok {
			res.PromptTokens = jsonInt(usage["prompt_tokens"])
			res.TotalTokens = jsonInt(usage["total_tokens"])
		}
	}
	res.OK = len(values) > 0
	if res.OK {
		res.ReplyText = fmt.Sprintf("embedding dimension: %d", len(values))
	} else {
		res.Error = "upstream returned no embedding vector"
	}
}

func parseUpstreamTestImageResponse(res *upstreamTestResult, format string, body []byte) {
	documents, err := decodeUpstreamTestJSONDocuments(body)
	if err != nil {
		res.Error = "parse response failed: " + err.Error()
		return
	}
	invalidImagePayload := false
	for i := len(documents) - 1; i >= 0; i-- {
		found, invalid := applyUpstreamTestImageDocument(res, strings.TrimSpace(format), documents[i])
		invalidImagePayload = invalidImagePayload || invalid
		if found {
			break
		}
	}
	res.OK = res.ImageB64 != "" || res.ImageURL != ""
	if !res.OK && res.Error == "" {
		if invalidImagePayload {
			res.Error = "upstream returned image data that is neither valid base64 nor an http(s) URL"
		} else {
			res.Error = "upstream returned no image"
		}
	}
}

func applyUpstreamTestImageDocument(res *upstreamTestResult, format string, doc map[string]any) (bool, bool) {
	if format == string(domain.ProtocolGeminiGenerate) {
		mimeType, b64, text := geminiFirstInlineImage(doc)
		if strings.TrimSpace(b64) == "" {
			return false, false
		}
		res.ImageB64, res.ImageMime, res.ReplyText = b64, mimeType, text
		res.ActualImageFormat = domain.ImageResponseFormatB64
		if usage, ok := doc["usageMetadata"].(map[string]any); ok {
			res.PromptTokens = jsonInt(usage["promptTokenCount"])
			res.OutputTokens = jsonInt(usage["candidatesTokenCount"])
			res.TotalTokens = jsonInt(usage["totalTokenCount"])
		}
		return true, false
	}

	data, ok := doc["data"].([]any)
	if !ok || len(data) == 0 {
		return false, false
	}
	item, ok := data[0].(map[string]any)
	if !ok {
		return false, false
	}
	rawB64, _ := item["b64_json"].(string)
	rawURL, _ := item["url"].(string)
	if strings.TrimSpace(rawB64) != "" {
		b64, mimeType, valid := upstreamTestInlineBase64(rawB64)
		if !valid {
			return false, true
		}
		res.ImageB64 = b64
		res.ImageMime = mimeType
		res.ActualImageFormat = domain.ImageResponseFormatB64
	} else if strings.TrimSpace(rawURL) != "" {
		if b64, mimeType, valid := upstreamTestInlineBase64(rawURL); valid {
			res.ImageB64 = b64
			res.ImageMime = mimeType
			res.ActualImageFormat = domain.ImageResponseFormatB64
		} else if upstreamTestHTTPURL(rawURL) {
			res.ImageURL = rawURL
			res.ActualImageFormat = domain.ImageResponseFormatURL
		} else {
			return false, true
		}
	} else {
		return false, false
	}
	if revisedPrompt, _ := item["revised_prompt"].(string); revisedPrompt != "" {
		res.ReplyText = revisedPrompt
	}
	if usage, ok := doc["usage"].(map[string]any); ok {
		res.PromptTokens = jsonInt(usage["prompt_tokens"])
		if res.PromptTokens == 0 {
			res.PromptTokens = jsonInt(usage["input_tokens"])
		}
		res.OutputTokens = jsonInt(usage["completion_tokens"])
		if res.OutputTokens == 0 {
			res.OutputTokens = jsonInt(usage["output_tokens"])
		}
		res.TotalTokens = jsonInt(usage["total_tokens"])
	}
	return true, false
}

func decodeUpstreamTestJSONDocuments(body []byte) ([]map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(body, &document); err == nil {
		return []map[string]any{document}, nil
	}

	documents := make([]map[string]any, 0, 4)
	dataLines := make([]string, 0, 1)
	eventType := ""
	var decodeErr error
	flushEvent := func() {
		if len(dataLines) == 0 {
			return
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if payload == "" || payload == "[DONE]" {
			return
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err == nil && event != nil {
			if _, exists := event["type"]; !exists && eventType != "" {
				event["type"] = eventType
			}
			documents = append(documents, event)
		} else {
			decodeErr = fmt.Errorf("SSE event is not a JSON object")
		}
	}
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if line == "" {
			flushEvent()
			eventType = ""
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	flushEvent()
	if decodeErr != nil {
		return nil, decodeErr
	}
	if len(documents) > 0 {
		return documents, nil
	}
	return nil, fmt.Errorf("response is neither a JSON object nor a valid SSE JSON stream")
}

func normalizedUpstreamTestImageStreamMode(value string) string {
	if strings.TrimSpace(value) == domain.ImageStreamModeForceStream {
		return domain.ImageStreamModeForceStream
	}
	return domain.ImageStreamModeForceSync
}

func normalizedUpstreamTestImageEditTransport(value string) string {
	if strings.TrimSpace(value) == domain.ImageEditTransportJSON {
		return domain.ImageEditTransportJSON
	}
	return domain.ImageEditTransportMultipart
}

func normalizedUpstreamTestImageResponseFormat(value string) string {
	normalized, ok := normalizeImageResponseFormat(value)
	if !ok {
		return ""
	}
	return normalized
}

func upstreamTestHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func upstreamTestInlineBase64(raw string) (string, string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", false
	}
	mimeType := ""
	if strings.HasPrefix(value, "data:") {
		comma := strings.IndexByte(value, ',')
		if comma <= len("data:") || !strings.Contains(strings.ToLower(value[:comma]), ";base64") {
			return "", "", false
		}
		metadata := strings.TrimPrefix(value[:comma], "data:")
		mimeType = strings.TrimSpace(strings.Split(metadata, ";")[0])
		value = value[comma+1:]
	}
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return "", "", false
	}
	return value, mimeType, true
}

func geminiFirstText(doc map[string]any) string {
	candidates, _ := doc["candidates"].([]any)
	for _, cv := range candidates {
		cand, _ := cv.(map[string]any)
		content, _ := cand["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, pv := range parts {
			p, _ := pv.(map[string]any)
			if s, _ := p["text"].(string); strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func geminiFirstInlineImage(doc map[string]any) (mime, b64, text string) {
	candidates, _ := doc["candidates"].([]any)
	for _, cv := range candidates {
		cand, _ := cv.(map[string]any)
		content, _ := cand["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, pv := range parts {
			p, _ := pv.(map[string]any)
			if s, _ := p["text"].(string); strings.TrimSpace(s) != "" && text == "" {
				text = s
			}
			inline, ok := p["inlineData"].(map[string]any)
			if !ok {
				inline, ok = p["inline_data"].(map[string]any)
			}
			if ok {
				m, _ := inline["mimeType"].(string)
				if m == "" {
					m, _ = inline["mime_type"].(string)
				}
				dataStr, _ := inline["data"].(string)
				if dataStr != "" {
					return m, dataStr, text
				}
			}
		}
	}
	return "", "", text
}

func jsonInt(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
