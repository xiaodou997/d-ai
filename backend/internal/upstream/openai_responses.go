package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const ProtocolOpenAIResponses = "openai_responses"

type OpenAIResponsesRequest struct {
	BaseURL            string
	CustomPath         string
	APIKey             string
	UpstreamModel      string
	ExtraHeaders       []byte
	UpstreamParameters []byte
	Timeout            time.Duration
	Body               map[string]json.RawMessage
}

func ForwardOpenAIResponses(ctx context.Context, client *http.Client, req OpenAIResponsesRequest) (*Response, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	httpReq, err := NewOpenAIResponsesHTTPRequest(ctx, req, "application/json")
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

	return &Response{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: respBody}, nil
}

func ForwardOpenAIResponsesStream(ctx context.Context, client *http.Client, req OpenAIResponsesRequest) (*StreamResponse, error) {
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	httpReq, err := NewOpenAIResponsesHTTPRequest(ctx, req, "text/event-stream")
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
	return &StreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: body}, nil
}

func NewOpenAIResponsesHTTPRequest(ctx context.Context, req OpenAIResponsesRequest, accept string) (*http.Request, error) {
	if req.BaseURL == "" {
		return nil, errors.New("upstream base url is required")
	}
	if req.APIKey == "" {
		return nil, errors.New("upstream api key is required")
	}
	if req.UpstreamModel == "" {
		return nil, errors.New("upstream model is required")
	}

	body, err := buildOpenAIResponsesBody(req.Body, req.UpstreamModel, req.UpstreamParameters)
	if err != nil {
		return nil, err
	}
	endpoint, err := BuildEndpointURL(req.BaseURL, req.CustomPath, "/responses")
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

func buildOpenAIResponsesBody(raw map[string]json.RawMessage, upstreamModel string, upstreamParameters []byte) ([]byte, error) {
	return buildOpenAIJSONBody(raw, upstreamModel, upstreamParameters)
}
