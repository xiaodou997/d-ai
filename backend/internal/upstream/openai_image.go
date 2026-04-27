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

const ProtocolOpenAIImagesGenerations = "openai_images_generations"

type OpenAIImageRequest struct {
	BaseURL            string
	CustomPath         string
	APIKey             string
	UpstreamModel      string
	ExtraHeaders       []byte
	UpstreamParameters []byte
	Timeout            time.Duration
	Body               map[string]json.RawMessage
}

func ForwardOpenAIImageGeneration(ctx context.Context, client *http.Client, req OpenAIImageRequest) (*Response, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	httpReq, err := NewOpenAIImageHTTPRequest(ctx, req)
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

func NewOpenAIImageHTTPRequest(ctx context.Context, req OpenAIImageRequest) (*http.Request, error) {
	if req.BaseURL == "" {
		return nil, errors.New("upstream base url is required")
	}
	if req.APIKey == "" {
		return nil, errors.New("upstream api key is required")
	}
	if req.UpstreamModel == "" {
		return nil, errors.New("upstream model is required")
	}

	body, err := buildOpenAIImageBody(req.Body, req.UpstreamModel, req.UpstreamParameters)
	if err != nil {
		return nil, err
	}
	endpoint, err := BuildEndpointURL(req.BaseURL, req.CustomPath, "/images/generations")
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

func buildOpenAIImageBody(raw map[string]json.RawMessage, upstreamModel string, upstreamParameters []byte) ([]byte, error) {
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
