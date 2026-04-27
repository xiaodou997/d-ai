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

const ProtocolOpenAIEmbeddings = "openai_embeddings"

type OpenAIEmbeddingsRequest struct {
	BaseURL            string
	CustomPath         string
	APIKey             string
	UpstreamModel      string
	ExtraHeaders       []byte
	UpstreamParameters []byte
	Timeout            time.Duration
	Body               map[string]json.RawMessage
}

func ForwardOpenAIEmbeddings(ctx context.Context, client *http.Client, req OpenAIEmbeddingsRequest) (*Response, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	httpReq, err := NewOpenAIEmbeddingsHTTPRequest(ctx, req)
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

func NewOpenAIEmbeddingsHTTPRequest(ctx context.Context, req OpenAIEmbeddingsRequest) (*http.Request, error) {
	if req.BaseURL == "" {
		return nil, errors.New("upstream base url is required")
	}
	if req.APIKey == "" {
		return nil, errors.New("upstream api key is required")
	}
	if req.UpstreamModel == "" {
		return nil, errors.New("upstream model is required")
	}

	body, err := buildOpenAIEmbeddingsBody(req.Body, req.UpstreamModel, req.UpstreamParameters)
	if err != nil {
		return nil, err
	}
	endpoint, err := BuildEndpointURL(req.BaseURL, req.CustomPath, "/embeddings")
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if err := applyExtraHeaders(httpReq.Header, req.ExtraHeaders); err != nil {
		return nil, err
	}
	return httpReq, nil
}

func buildOpenAIEmbeddingsBody(raw map[string]json.RawMessage, upstreamModel string, upstreamParameters []byte) ([]byte, error) {
	return buildOpenAIJSONBody(raw, upstreamModel, upstreamParameters)
}
