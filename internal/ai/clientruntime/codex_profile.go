package clientruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"xiaodou/dai/internal/ai/domain"
)

const (
	CodexProfileRevision = "codex-cli@0.144.1+unihub.1"
	codexBaseURL         = "https://chatgpt.com/backend-api/codex"
	codexUserAgent       = "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color"
	codexOriginator      = "codex_cli_rs"
	codexClientVersion   = "0.144.1"
)

var codexUnsupportedResponseFields = []string{
	"max_output_tokens",
	"max_completion_tokens",
	"temperature",
	"top_p",
	"frequency_penalty",
	"presence_penalty",
	"user",
	"metadata",
	"prompt_cache_options",
	"prompt_cache_retention",
	"safety_identifier",
	"stream_options",
}

type codexProfileV01441 struct{}

func (codexProfileV01441) revision() string {
	return CodexProfileRevision
}

func (codexProfileV01441) supports(protocol domain.UpstreamProtocol) bool {
	return protocol == domain.ProtocolOpenAIResponses
}

func (codexProfileV01441) prepare(in Invocation) (*WireRequest, error) {
	if in.Protocol != domain.ProtocolOpenAIResponses {
		return nil, fmt.Errorf("codex profile requires %q, got %q", domain.ProtocolOpenAIResponses, in.Protocol)
	}
	body, err := applyCodexResponsesContract(in.Body, in.Model, in.Stream, in.Operation)
	if err != nil {
		return nil, err
	}
	path := "/responses"
	if in.Operation == OperationCompact {
		path = "/responses/compact"
	}

	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	headers := map[string]string{
		"Authorization": "Bearer " + in.Credential.AccessToken,
		"Content-Type":  contentType,
		"OpenAI-Beta":   "responses=experimental",
		"User-Agent":    codexUserAgent,
		"originator":    codexOriginator,
		"version":       codexClientVersion,
	}
	if in.Stream && in.Operation != OperationCompact {
		headers["Accept"] = "text/event-stream"
	} else {
		headers["Accept"] = "application/json"
	}
	if validHeaderValue(in.Credential.AccountID) {
		headers["chatgpt-account-id"] = strings.TrimSpace(in.Credential.AccountID)
	}

	affinity := strings.TrimSpace(in.AffinityKey)
	if affinity == "" {
		affinity = codexPromptCacheKey(body)
	}
	if affinity == "" {
		affinity = strings.TrimSpace(in.RequestID)
	}
	if affinity != "" {
		sessionID := stableCodexID("session", in.Credential.ID+":"+affinity)
		headers["session_id"] = sessionID
		headers["conversation_id"] = sessionID
	}
	if in.RequestID != "" {
		headers["x-codex-window-id"] = stableCodexID("window", in.Credential.ID+":"+in.RequestID)
	}
	if installationID := credentialMetadataString(in.Credential.Metadata,
		"x_codex_installation_id", "installation_id"); validHeaderValue(installationID) {
		headers["x-codex-installation-id"] = installationID
	}

	return &WireRequest{
		Method:   http.MethodPost,
		URL:      codexBaseURL + path,
		Headers:  headers,
		Body:     body,
		Protocol: domain.ProtocolOpenAIResponses,
	}, nil
}

func (codexProfileV01441) prepareInspection(in Inspection) (*WireRequest, error) {
	if in.Credential.AccessToken == "" {
		return nil, fmt.Errorf("codex model inspection requires an access token")
	}
	headers := map[string]string{
		"Authorization": "Bearer " + in.Credential.AccessToken,
		"Accept":        "application/json",
		"User-Agent":    codexUserAgent,
		"originator":    codexOriginator,
		"version":       codexClientVersion,
	}
	if validHeaderValue(in.Credential.AccountID) {
		headers["chatgpt-account-id"] = strings.TrimSpace(in.Credential.AccountID)
	}
	if etag := strings.TrimSpace(in.IfNoneMatch); validHeaderValue(etag) {
		headers["If-None-Match"] = etag
	}
	return &WireRequest{
		Method:   http.MethodGet,
		URL:      codexBaseURL + "/models?client_version=" + url.QueryEscape(codexClientVersion),
		Headers:  headers,
		Protocol: domain.ProtocolOpenAIResponses,
	}, nil
}

func (codexProfileV01441) decodeModels(body []byte) ([]ModelCard, error) {
	var envelope struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode codex models: %w", err)
	}
	if len(envelope.Models) == 0 {
		return nil, fmt.Errorf("codex models manifest is empty")
	}
	capabilityFields := []string{
		"use_responses_lite",
		"supports_reasoning_summary_parameter",
		"default_reasoning_level",
		"default_reasoning_summary",
		"supported_reasoning_levels",
		"supports_parallel_tool_calls",
		"support_verbosity",
		"default_verbosity",
		"service_tiers",
	}
	seen := make(map[string]struct{}, len(envelope.Models))
	models := make([]ModelCard, 0, len(envelope.Models))
	for _, raw := range envelope.Models {
		id := strings.TrimSpace(anyString(raw["slug"]))
		if id == "" {
			id = strings.TrimSpace(anyString(raw["id"]))
		}
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		capabilities := make(map[string]any)
		for _, field := range capabilityFields {
			if value, ok := raw[field]; ok {
				capabilities[field] = value
			}
		}
		models = append(models, ModelCard{ID: id, Capabilities: capabilities})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("codex models manifest has no usable model IDs")
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func applyCodexResponsesContract(body []byte, model string, stream bool, operation OperationKind) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var doc map[string]any
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode responses body: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("responses body must be an object")
	}
	if strings.TrimSpace(model) != "" {
		doc["model"] = strings.TrimSpace(model)
	}

	for _, field := range codexUnsupportedResponseFields {
		delete(doc, field)
	}
	stripCodexCacheControl(doc)
	normalizeCodexLegacyTools(doc)
	normalizeCodexToolChoice(doc)
	normalizeCodexInput(doc)
	ensureCodexReasoningEncryptedContent(doc)

	if operation == OperationCompact {
		delete(doc, "store")
		delete(doc, "stream")
		allowed := map[string]bool{
			"model": true, "input": true, "instructions": true, "tools": true,
			"parallel_tool_calls": true, "reasoning": true, "service_tier": true,
			"prompt_cache_key": true, "text": true,
		}
		for field := range doc {
			if !allowed[field] {
				delete(doc, field)
			}
		}
		if _, ok := doc["parallel_tool_calls"]; !ok {
			doc["parallel_tool_calls"] = true
		}
	} else {
		doc["store"] = false
		doc["stream"] = stream
		if _, ok := doc["tool_choice"]; !ok {
			doc["tool_choice"] = "auto"
		}
	}

	if instructions, ok := doc["instructions"].(string); !ok || strings.TrimSpace(instructions) == "" {
		doc["instructions"] = "You are ChatGPT."
	}
	return json.Marshal(doc)
}

func normalizeCodexInput(doc map[string]any) {
	input, ok := doc["input"].(string)
	if !ok {
		return
	}
	doc["input"] = []any{
		map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": input},
			},
		},
	}
}

func normalizeCodexLegacyTools(doc map[string]any) {
	if functions, ok := doc["functions"].([]any); ok {
		tools := make([]any, 0, len(functions))
		for _, raw := range functions {
			function, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			tool := cloneAnyMap(function)
			tool["type"] = "function"
			tools = append(tools, tool)
		}
		doc["tools"] = tools
		delete(doc, "functions")
	}

	tools, ok := doc["tools"].([]any)
	if !ok {
		return
	}
	for index, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || !strings.EqualFold(anyString(tool["type"]), "function") {
			continue
		}
		function, ok := tool["function"].(map[string]any)
		if !ok {
			continue
		}
		flat := cloneAnyMap(function)
		flat["type"] = "function"
		tools[index] = flat
	}
	doc["tools"] = tools
}

func normalizeCodexToolChoice(doc map[string]any) {
	if legacy, ok := doc["function_call"]; ok {
		switch value := legacy.(type) {
		case string:
			doc["tool_choice"] = value
		case map[string]any:
			if name := strings.TrimSpace(anyString(value["name"])); name != "" {
				doc["tool_choice"] = map[string]any{"type": "function", "name": name}
			}
		}
		delete(doc, "function_call")
	}
	choice, ok := doc["tool_choice"].(map[string]any)
	if !ok || !strings.EqualFold(anyString(choice["type"]), "function") {
		return
	}
	if function, ok := choice["function"].(map[string]any); ok {
		if name := strings.TrimSpace(anyString(function["name"])); name != "" {
			choice["name"] = name
		}
		delete(choice, "function")
	}
}

func ensureCodexReasoningEncryptedContent(doc map[string]any) {
	if _, ok := doc["reasoning"].(map[string]any); !ok {
		return
	}
	include, ok := doc["include"].([]any)
	if !ok {
		if _, exists := doc["include"]; !exists || doc["include"] == nil {
			doc["include"] = []any{"reasoning.encrypted_content"}
		}
		return
	}
	for _, item := range include {
		if anyString(item) == "reasoning.encrypted_content" {
			return
		}
	}
	doc["include"] = append(include, "reasoning.encrypted_content")
}

func stripCodexCacheControl(value any) {
	switch current := value.(type) {
	case map[string]any:
		delete(current, "cache_control")
		for _, child := range current {
			stripCodexCacheControl(child)
		}
	case []any:
		for _, child := range current {
			stripCodexCacheControl(child)
		}
	}
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func credentialMetadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func codexPromptCacheKey(body []byte) string {
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return ""
	}
	value, _ := doc["prompt_cache_key"].(string)
	return strings.TrimSpace(value)
}

func stableCodexID(kind, seed string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("unihub:codex:"+kind+":v1:"+seed)).String()
}

func validHeaderValue(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 1024 && !strings.ContainsAny(value, "\r\n")
}
