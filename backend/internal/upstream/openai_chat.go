package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ProtocolOpenAIChatCompletions = "openai_chat_completions"

type OpenAIChatRequest struct {
	BaseURL            string
	CustomPath         string
	APIKey             string
	UpstreamModel      string
	ExtraHeaders       []byte
	UpstreamParameters []byte
	Timeout            time.Duration
	Body               map[string]json.RawMessage
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type StreamResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

func ForwardOpenAIChatCompletions(ctx context.Context, client *http.Client, req OpenAIChatRequest) (*Response, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	httpReq, err := NewOpenAIChatHTTPRequest(ctx, req, "application/json")
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call upstream: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}, nil
}

func ForwardOpenAIChatCompletionsStream(ctx context.Context, client *http.Client, req OpenAIChatRequest) (*StreamResponse, error) {
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	httpReq, err := NewOpenAIChatHTTPRequest(ctx, req, "text/event-stream")
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("call upstream: %w", err)
	}
	body := resp.Body
	if cancel != nil {
		body = cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
	}

	return &StreamResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       body,
	}, nil
}

func NewOpenAIChatHTTPRequest(ctx context.Context, req OpenAIChatRequest, accept string) (*http.Request, error) {
	if req.BaseURL == "" {
		return nil, errors.New("upstream base url is required")
	}
	if req.APIKey == "" {
		return nil, errors.New("upstream api key is required")
	}
	if req.UpstreamModel == "" {
		return nil, errors.New("upstream model is required")
	}

	body, err := buildOpenAIChatBody(req.Body, req.UpstreamModel, req.UpstreamParameters)
	if err != nil {
		return nil, err
	}
	endpoint, err := BuildEndpointURL(req.BaseURL, req.CustomPath, "/chat/completions")
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if accept != "" {
		httpReq.Header.Set("Accept", accept)
	}
	if err := applyExtraHeaders(httpReq.Header, req.ExtraHeaders); err != nil {
		return nil, err
	}

	return httpReq, nil
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func buildOpenAIChatBody(raw map[string]json.RawMessage, upstreamModel string, upstreamParameters []byte) ([]byte, error) {
	return buildOpenAIJSONBody(raw, upstreamModel, upstreamParameters)
}

func buildOpenAIJSONBody(raw map[string]json.RawMessage, upstreamModel string, upstreamParameters []byte) ([]byte, error) {
	body := make(map[string]json.RawMessage, len(raw)+1)
	for key, value := range raw {
		body[key] = value
	}

	model, err := json.Marshal(upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("marshal upstream model: %w", err)
	}
	body["model"] = model

	if len(upstreamParameters) > 0 {
		var defaults map[string]json.RawMessage
		if err := json.Unmarshal(upstreamParameters, &defaults); err != nil {
			return nil, fmt.Errorf("parse upstream parameters: %w", err)
		}
		for key, value := range defaults {
			if _, exists := body[key]; !exists {
				body[key] = value
			}
		}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal upstream body: %w", err)
	}
	return encoded, nil
}

func applyExtraHeaders(headers http.Header, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}

	var stringHeaders map[string]string
	if err := json.Unmarshal(raw, &stringHeaders); err == nil {
		for key, value := range stringHeaders {
			headers.Set(key, value)
		}
		return nil
	}

	var anyHeaders map[string]any
	if err := json.Unmarshal(raw, &anyHeaders); err != nil {
		return fmt.Errorf("parse upstream extra headers: %w", err)
	}
	for key, value := range anyHeaders {
		headers.Set(key, fmt.Sprint(value))
	}
	return nil
}

func BuildEndpointURL(baseURL string, customPath string, defaultPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", fmt.Errorf("parse upstream base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid upstream base url")
	}

	path := defaultPath
	if customPath != "" {
		path = customPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	return parsed.String(), nil
}
