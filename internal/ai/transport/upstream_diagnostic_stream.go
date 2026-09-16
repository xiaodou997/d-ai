package transport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

func upstreamChatTestStreams(format string) bool {
	return format == string(domain.ProtocolOpenAIResponses) || format == string(domain.ProtocolOpenAIChat) || format == string(domain.ProtocolAnthropicMessages)
}

// Stop at the provider terminal event even when a streaming connection stays
// open. The deadline reader still applies header/first-byte/idle limits.
func readUpstreamTestBody(r io.Reader, format string, stream bool) ([]byte, error) {
	const limit = 64 << 20
	if !stream {
		b, e := io.ReadAll(io.LimitReader(r, limit+1))
		if len(b) > limit {
			return nil, fmt.Errorf("upstream test response exceeds 64 MiB")
		}
		return b, e
	}
	reader := bufio.NewReader(io.LimitReader(r, limit+1))
	var body bytes.Buffer
	var data []string
	event := ""
	for {
		line, e := reader.ReadString('\n')
		body.WriteString(line)
		if body.Len() > limit {
			return nil, fmt.Errorf("upstream test response exceeds 64 MiB")
		}
		text := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if strings.HasPrefix(text, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(text, "data:"), " "))
		}
		if strings.HasPrefix(text, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(text, "event:"))
		}
		if text == "" || e != nil {
			payload := strings.Join(data, "\n")
			out := formats.InspectStreamOutcome([]byte(payload), event)
			terminal := out.State == domain.ProviderTerminalFailed || out.State == domain.ProviderTerminalCancelled || out.State == domain.ProviderTerminalIncomplete
			if format == string(domain.ProtocolOpenAIChat) {
				terminal = terminal || payload == "[DONE]"
			} else {
				terminal = terminal || out.State == domain.ProviderTerminalCompleted
			}
			if terminal {
				return body.Bytes(), nil
			}
			data = nil
			event = ""
		}
		if e == io.EOF {
			return body.Bytes(), nil
		}
		if e != nil {
			return body.Bytes(), e
		}
	}
}

func upstreamTestDocumentError(doc map[string]any) string {
	if response, ok := doc["response"].(map[string]any); ok {
		if message := upstreamTestDocumentError(response); message != "" {
			return message
		}
	}
	if raw, ok := doc["error"]; ok && raw != nil {
		if e, ok := raw.(map[string]any); ok {
			if message, ok := e["message"].(string); ok && message != "" {
				return "上游报错：" + truncateStr(message, 1024)
			}
		}
		return "上游返回错误，未完成测试请求"
	}
	if status, _ := doc["status"].(string); status == "incomplete" || status == "failed" || status == "cancelled" {
		return "上游未完成测试请求（" + status + "）"
	}
	if typ, _ := doc["type"].(string); typ == "error" || typ == "response.failed" || typ == "response.incomplete" {
		return "上游未完成测试请求（" + typ + "）"
	}
	return ""
}

func parseUpstreamTestChatResponse(res *upstreamTestResult, format string, body []byte) {
	if json.Valid(body) {
		parseUpstreamTestChatJSON(res, format, body)
		return
	}
	docs, err := decodeUpstreamTestJSONDocuments(body)
	if err != nil {
		res.Error = "上游响应不符合所选协议：需要 JSON 或 SSE，请检查请求端点及上游协议支持"
		return
	}
	complete := false
	var text strings.Builder
	for _, doc := range docs {
		if message := upstreamTestDocumentError(doc); message != "" {
			res.Error = message
			return
		}
		typ, _ := doc["type"].(string)
		switch format {
		case string(domain.ProtocolOpenAIResponses):
			if typ == "response.output_text.delta" {
				if delta, ok := doc["delta"].(string); ok {
					text.WriteString(delta)
				}
			}
			if typ == "response.completed" {
				complete = true
			}
		case string(domain.ProtocolAnthropicMessages):
			if delta, ok := doc["delta"].(map[string]any); ok {
				if value, ok := delta["text"].(string); ok {
					text.WriteString(value)
				}
			}
			if typ == "message_stop" {
				complete = true
			}
		default:
			if choices, ok := doc["choices"].([]any); ok {
				for _, raw := range choices {
					choice, _ := raw.(map[string]any)
					if delta, ok := choice["delta"].(map[string]any); ok {
						if value, ok := delta["content"].(string); ok {
							text.WriteString(value)
						}
					}
					if finish, _ := choice["finish_reason"].(string); finish != "" {
						complete = true
					}
				}
			}
		}
		payload := doc
		if nested, ok := doc["response"].(map[string]any); ok {
			payload = nested
		}
		if nested, ok := doc["message"].(map[string]any); ok {
			payload = nested
		}
		encoded, _ := json.Marshal(payload)
		var parsed upstreamTestResult
		parseUpstreamTestChatJSON(&parsed, format, encoded)
		if parsed.ReplyText != "" {
			res.ReplyText = parsed.ReplyText
		}
		res.PromptTokens = max(res.PromptTokens, parsed.PromptTokens)
		res.OutputTokens = max(res.OutputTokens, parsed.OutputTokens)
		res.TotalTokens = max(res.TotalTokens, parsed.TotalTokens)
	}
	if res.ReplyText == "" {
		res.ReplyText = text.String()
	}
	res.TotalTokens = max(res.TotalTokens, res.PromptTokens+res.OutputTokens)
	res.OK = complete && strings.TrimSpace(res.ReplyText) != ""
	if !complete {
		res.Error = "上游流式响应提前结束，未收到完成事件"
	} else if !res.OK {
		res.Error = "上游已结束响应，但未返回文本；请检查模型及输出额度"
	}
}

func upstreamTestHTTPError(status int, body []byte) string {
	message := "上游请求失败"
	switch status {
	case 401:
		message = "上游拒绝了凭据，请检查账号 API Key"
	case 403:
		message = "上游拒绝了请求，请检查账号权限及网络出口"
	case 404:
		message = "上游未找到请求端点或模型，请检查端点路径和模型绑定"
	case 429:
		message = "上游限流或额度不足，请稍后重试或检查上游额度"
	default:
		if status >= 500 {
			message = "上游服务暂时不可用，请稍后重试或选择其他账号"
		}
	}
	detail := strings.TrimSpace(string(body))
	var doc map[string]any
	if json.Unmarshal(body, &doc) == nil {
		if parsed := upstreamTestDocumentError(doc); parsed != "" {
			detail = parsed
		}
	}
	if detail == "" {
		return fmt.Sprintf("HTTP %d：%s", status, message)
	}
	return fmt.Sprintf("HTTP %d：%s。详情：%s", status, message, truncateStr(detail, 1024))
}
