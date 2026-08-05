// Package httpx defines UniHub 的统一 HTTP 契约原语：RFC 7807 problem+json
// 错误模型、业务错误（AppError）与强类型分页结构。这些类型刻意不依赖任何 HTTP
// 框架，可被 chi handler、Huma 适配层与单测共用。
package httpx

import (
	"encoding/json"
	"net/http"
)

// ProblemContentType 是 RFC 7807 规定的错误响应媒体类型。
const ProblemContentType = "application/problem+json"

// Problem 是 RFC 7807 的错误响应体。除标准字段外，扩展 code（业务错误码，保留
// v1 语义）、request_id 与 errors（字段级校验错误）。成功响应不使用本类型。
type Problem struct {
	// Type 是标识错误类型的 URI；省略时按 RFC 7807 视为 "about:blank"。
	Type string `json:"type,omitempty"`
	// Title 是该错误类型的人类可读简短摘要，不随单次发生而变化。
	Title string `json:"title"`
	// Status 是 HTTP 状态码，与响应行一致。
	Status int `json:"status"`
	// Detail 是本次发生的具体说明。
	Detail string `json:"detail,omitempty"`
	// Instance 标识本次错误的具体出处（如 request 路径）。
	Instance string `json:"instance,omitempty"`

	// --- UniHub 扩展字段 ---

	// Code 是稳定的业务错误码（snake_case），供前端做语义分支。
	Code string `json:"code,omitempty"`
	// RequestID 关联到访问日志，便于排障。
	RequestID string `json:"request_id,omitempty"`
	// Errors 承载字段级校验错误。
	Errors []FieldError `json:"errors,omitempty"`
	// Meta carries structured business context such as retry times and counters.
	Meta map[string]any `json:"meta,omitempty"`
}

// FieldError 描述单个输入字段的校验失败。
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteProblem 以 application/problem+json 写出错误响应，并补齐缺省的 Type/Title。
func WriteProblem(w http.ResponseWriter, p Problem) {
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
	w.Header().Set("Content-Type", ProblemContentType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
