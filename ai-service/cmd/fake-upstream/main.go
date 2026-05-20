package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultAddr = ":18080"

type requestEnvelope struct {
	Model  string          `json:"model"`
	Stream bool            `json:"stream"`
	Input  json.RawMessage `json:"input"`
	Prompt json.RawMessage `json:"prompt"`
}

func main() {
	addr := os.Getenv("FAKE_UPSTREAM_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           newHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("starting fake upstream", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("fake upstream failed", "error", err)
		os.Exit(1)
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleHealth)
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions)
	mux.HandleFunc("/chat/completions", handleChatCompletions)
	mux.HandleFunc("/v1/responses", handleResponses)
	mux.HandleFunc("/responses", handleResponses)
	mux.HandleFunc("/v1/embeddings", handleEmbeddings)
	mux.HandleFunc("/embeddings", handleEmbeddings)
	mux.HandleFunc("/v1/images/generations", handleImageGenerations)
	mux.HandleFunc("/images/generations", handleImageGenerations)
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req requestEnvelope
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, openAIError("Invalid request body."))
		return
	}
	if req.Model == "" {
		req.Model = "fake-chat-model"
	}
	if req.Stream {
		writeChatStream(w, req.Model)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-fake-local",
		"object":  "chat.completion",
		"created": 1710000000,
		"model":   req.Model,
		"choices": []map[string]any{
			{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "fake upstream chat response",
				},
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     7,
			"completion_tokens": 5,
			"total_tokens":      12,
		},
	})
}

func writeChatStream(w http.ResponseWriter, model string) {
	prepareSSE(w)
	events := []map[string]any{
		{
			"id":      "chatcmpl-fake-local",
			"object":  "chat.completion.chunk",
			"created": 1710000000,
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]string{"role": "assistant"}, "finish_reason": nil}},
		},
		{
			"id":      "chatcmpl-fake-local",
			"object":  "chat.completion.chunk",
			"created": 1710000000,
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]string{"content": "fake "}, "finish_reason": nil}},
		},
		{
			"id":      "chatcmpl-fake-local",
			"object":  "chat.completion.chunk",
			"created": 1710000000,
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]string{"content": "stream"}, "finish_reason": nil}},
		},
		{
			"id":      "chatcmpl-fake-local",
			"object":  "chat.completion.chunk",
			"created": 1710000000,
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]string{}, "finish_reason": "stop"}},
			"usage": map[string]int{
				"prompt_tokens":     7,
				"completion_tokens": 3,
				"total_tokens":      10,
			},
		},
	}
	writeSSEEvents(w, events, true)
}

func handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req requestEnvelope
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, openAIError("Invalid request body."))
		return
	}
	if req.Model == "" {
		req.Model = "fake-responses-model"
	}
	if req.Stream {
		writeResponsesStream(w, req.Model)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          "resp_fake_local",
		"object":      "response",
		"created_at":  1710000000,
		"model":       req.Model,
		"status":      "completed",
		"output_text": "fake upstream responses output",
		"output": []map[string]any{
			{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": "fake upstream responses output"},
				},
			},
		},
		"usage": map[string]int{
			"input_tokens":  11,
			"output_tokens": 6,
			"total_tokens":  17,
		},
	})
}

func writeResponsesStream(w http.ResponseWriter, model string) {
	prepareSSE(w)
	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_fake_local", "model": model, "status": "in_progress"}},
		{"type": "response.output_text.delta", "delta": "fake "},
		{"type": "response.output_text.delta", "delta": "responses stream"},
		{
			"type": "response.completed",
			"response": map[string]any{
				"id":     "resp_fake_local",
				"model":  model,
				"status": "completed",
				"usage": map[string]int{
					"input_tokens":  11,
					"output_tokens": 4,
					"total_tokens":  15,
				},
			},
		},
	}
	writeSSEEvents(w, events, false)
}

func handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req requestEnvelope
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, openAIError("Invalid request body."))
		return
	}
	if req.Model == "" {
		req.Model = "fake-embedding-model"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"model":  req.Model,
		"data": []map[string]any{
			{
				"object":    "embedding",
				"index":     0,
				"embedding": []float64{0.101, 0.202, 0.303, 0.404, 0.505, 0.606, 0.707, 0.808},
			},
		},
		"usage": map[string]int{
			"prompt_tokens": 9,
			"total_tokens":  9,
		},
	})
}

func handleImageGenerations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, openAIError("Invalid request body."))
		return
	}
	n := imageCount(raw)
	data := make([]map[string]string, 0, n)
	for i := 0; i < n; i++ {
		data = append(data, map[string]string{
			"url": fmt.Sprintf("https://fake.local/images/%03d.png", i+1),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"created": 1710000000,
		"data":    data,
	})
}

func imageCount(raw map[string]json.RawMessage) int {
	if raw == nil {
		return 1
	}
	value, ok := raw["n"]
	if !ok {
		return 1
	}
	var n int
	if err := json.Unmarshal(value, &n); err != nil || n <= 0 {
		return 1
	}
	if n > 10 {
		return 10
	}
	return n
}

func prepareSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

func writeSSEEvents(w http.ResponseWriter, events []map[string]any, includeDone bool) {
	flusher, _ := w.(http.Flusher)
	for _, event := range events {
		b, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	if includeDone {
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}
	if flusher != nil {
		flusher.Flush()
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func openAIError(message string) map[string]any {
	return map[string]any{
		"error": map[string]string{
			"message": strings.TrimSpace(message),
			"type":    "invalid_request_error",
			"code":    "invalid_request",
		},
	}
}
