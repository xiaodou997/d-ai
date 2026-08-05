package formats

import (
	"encoding/json"
	"fmt"
)

// 协议转换中枢的调度入口（抄 Aether formats/registry.rs）：
//
//	ConvertRequest  = ParseRequest(src) + EmitRequest(dst)
//	ConvertResponse = ParseResponse(src) + EmitResponse(dst)
//
// 同格式（src==dst）调用方应走 passthrough（rewrite.go），不必经此层。

// FormatID 是一个细粒度 wire 格式标识（family:kind）。
type FormatID string

const (
	FormatOpenAIChat      FormatID = "openai:chat"
	FormatOpenAIResponses FormatID = "openai:responses"
	FormatClaudeMessages  FormatID = "claude:messages"
	FormatGeminiGenerate  FormatID = "gemini:generate"
)

// ParseRequest 把某格式的请求 body 解析为规范化请求。
func ParseRequest(src FormatID, body json.RawMessage) (*Request, error) {
	var (
		req *Request
		ok  bool
	)
	switch src {
	case FormatOpenAIChat:
		req, ok = openaiChatRequestFrom(body)
	case FormatOpenAIResponses:
		req, ok = openaiResponsesRequestFrom(body)
	case FormatClaudeMessages:
		req, ok = claudeRequestFrom(body)
	case FormatGeminiGenerate:
		req, ok = geminiRequestFrom(body)
	default:
		return nil, fmt.Errorf("formats: unknown source format %q", src)
	}
	if !ok {
		return nil, fmt.Errorf("formats: parse %s request failed", src)
	}
	return req, nil
}

// EmitRequest 把规范化请求 emit 成某格式的请求 body。mappedModel 非空时覆盖 model
// （对齐上游真实模型名，对应 Aether ctx.mapped_model）。upstreamStream 控制是否写
// 流式标志。
func EmitRequest(dst FormatID, req *Request, mappedModel string, upstreamStream bool) (json.RawMessage, error) {
	if mappedModel != "" {
		clone := *req
		clone.Model = mappedModel
		req = &clone
	}
	switch dst {
	case FormatOpenAIChat:
		return openaiChatRequestTo(req, upstreamStream), nil
	case FormatOpenAIResponses:
		return openaiResponsesRequestTo(req, upstreamStream), nil
	case FormatClaudeMessages:
		return claudeRequestTo(req, upstreamStream), nil
	case FormatGeminiGenerate:
		return geminiRequestTo(req, upstreamStream), nil
	default:
		return nil, fmt.Errorf("formats: unknown target format %q", dst)
	}
}

// ConvertRequest 把 src 格式的请求 body 转换成 dst 格式。
func ConvertRequest(src, dst FormatID, body json.RawMessage, mappedModel string, upstreamStream bool) (json.RawMessage, error) {
	req, err := ParseRequest(src, body)
	if err != nil {
		return nil, err
	}
	return EmitRequest(dst, req, mappedModel, upstreamStream)
}

// ParseResponse 把某格式的响应 body 解析为规范化响应。
func ParseResponse(src FormatID, body json.RawMessage) (*Response, error) {
	var (
		resp *Response
		ok   bool
	)
	switch src {
	case FormatOpenAIChat:
		resp, ok = openaiChatResponseFrom(body)
	case FormatOpenAIResponses:
		resp, ok = openaiResponsesResponseFrom(body)
	case FormatClaudeMessages:
		resp, ok = claudeResponseFrom(body)
	case FormatGeminiGenerate:
		resp, ok = geminiResponseFrom(body)
	default:
		return nil, fmt.Errorf("formats: unknown source format %q", src)
	}
	if !ok {
		return nil, fmt.Errorf("formats: parse %s response failed", src)
	}
	return resp, nil
}

// EmitResponse 把规范化响应 emit 成某格式的响应 body。
func EmitResponse(dst FormatID, resp *Response) (json.RawMessage, error) {
	switch dst {
	case FormatOpenAIChat:
		return openaiChatResponseTo(resp), nil
	case FormatOpenAIResponses:
		return openaiResponsesResponseTo(resp), nil
	case FormatClaudeMessages:
		return claudeResponseTo(resp), nil
	case FormatGeminiGenerate:
		return geminiResponseTo(resp), nil
	default:
		return nil, fmt.Errorf("formats: unknown target format %q", dst)
	}
}

// ConvertResponse 把 src 格式的响应 body 转换成 dst 格式。
func ConvertResponse(src, dst FormatID, body json.RawMessage) (json.RawMessage, error) {
	resp, err := ParseResponse(src, body)
	if err != nil {
		return nil, err
	}
	return EmitResponse(dst, resp)
}
