package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchUpstreamModelListUsesHTTPDoer(t *testing.T) {
	client := httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "https://upstream.example/v1/models"; got != want {
			t.Fatalf("request URL: got %q want %q", got, want)
		}
		if got, want := req.Header.Get("Authorization"), "Bearer provider-secret"; got != want {
			t.Fatalf("authorization header: got %q want %q", got, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"data":[{"id":"gpt-test","display_name":"GPT Test"}]}`,
			)),
		}, nil
	})

	models, err := fetchUpstreamModelList(
		context.Background(),
		client,
		domain.UpstreamAccountEndpoint{
			APIFormat:  domain.ProtocolOpenAIResponses,
			BaseURL:    "https://upstream.example",
			AuthScheme: domain.EndpointAuthFormatDefault,
		},
		"provider-secret",
	)
	if err != nil {
		t.Fatalf("fetch model list: %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-test" || models[0].Name != "GPT Test" {
		t.Fatalf("unexpected model list: %#v", models)
	}
}

func TestRunUpstreamAccountTestUsesHTTPDoer(t *testing.T) {
	client := httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "https://upstream.example/v1/chat/completions"; got != want {
			t.Fatalf("request URL: got %q want %q", got, want)
		}
		if got, want := req.Header.Get("Authorization"), "Bearer provider-secret"; got != want {
			t.Fatalf("authorization header: got %q want %q", got, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
			)),
		}, nil
	})

	result := runUpstreamAccountTest(context.Background(), client, upstreamTestConfig{
		BaseURL:       "https://upstream.example",
		APIKey:        "provider-secret",
		APIFormat:     string(domain.ProtocolOpenAIChat),
		UpstreamModel: "gpt-test",
		Capability:    string(domain.CapabilityChat),
	})
	if !result.OK || result.HTTPStatus != http.StatusOK || result.ReplyText != "hello" {
		t.Fatalf("unexpected connectivity result: %#v", result)
	}
	if result.PromptTokens != 2 || result.OutputTokens != 1 || result.TotalTokens != 3 {
		t.Fatalf("unexpected usage: %#v", result)
	}
}

func TestRunUpstreamAccountEmbeddingTestUsesEmbeddingEndpoint(t *testing.T) {
	client := httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "https://upstream.example/v1/embeddings"; got != want {
			t.Fatalf("request URL: got %q want %q", got, want)
		}
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), `"input":"hello"`) {
			t.Fatalf("request body = %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`)),
		}, nil
	})

	result := runUpstreamAccountTest(context.Background(), client, upstreamTestConfig{
		BaseURL:       "https://upstream.example",
		APIKey:        "provider-secret",
		APIFormat:     string(domain.ProtocolOpenAIEmbeddings),
		UpstreamModel: "embed-test",
		Capability:    string(domain.CapabilityEmbedding),
		Prompt:        "hello",
	})
	if !result.OK || result.Capability != "embedding" || result.ReplyText != "embedding dimension: 3" {
		t.Fatalf("unexpected embedding result: %#v", result)
	}
}
