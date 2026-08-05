package httpx

import (
	"errors"
	"net/http"
)

// AppError 是 UniHub 的统一业务错误：携带稳定业务码、HTTP 状态与可读标题，并实现
// error 接口。它同时满足 Huma 的 huma.StatusError（GetStatus）与 ContentTypeFilter
// （ContentType）鸭子接口，因此 handler 可直接返回 *AppError，由 Huma 用其 json tag
// 反射序列化为 RFC 7807 application/problem+json 响应体（字段与 Problem 对齐）。
//
// 预定义错误（如 ErrBadRequest）是共享模板：用 WithDetail / WithFields / WithCause
// 派生时一律返回副本，绝不修改原模板。
type AppError struct {
	Code   string         `json:"code,omitempty"`   // 稳定业务码（snake_case）
	Status int            `json:"status"`           // HTTP 状态码
	Title  string         `json:"title"`            // 人类可读简短标题
	Detail string         `json:"detail,omitempty"` // 本次具体说明
	Fields []FieldError   `json:"errors,omitempty"` // 字段级校验错误（可选）
	Meta   map[string]any `json:"meta,omitempty"`   // 结构化业务上下文（可选）
	cause  error          `json:"-"`                // 包装的底层错误（日志 / errors.Unwrap）
}

// New 构造一个业务错误模板。code 为 snake_case 业务码，status 为 HTTP 状态码。
func New(code string, status int, title string) *AppError {
	return &AppError{Code: code, Status: status, Title: title}
}

// Error 实现 error 接口。
func (e *AppError) Error() string {
	msg := e.Title
	if e.Detail != "" {
		msg = e.Title + ": " + e.Detail
	}
	if e.cause != nil {
		return msg + ": " + e.cause.Error()
	}
	return msg
}

// Unwrap 暴露底层错误，支持 errors.Is / errors.As。
func (e *AppError) Unwrap() error { return e.cause }

// GetStatus 满足 huma.StatusError 接口，返回 HTTP 状态码。
func (e *AppError) GetStatus() int { return e.Status }

// clone 返回浅副本，保证模板不被派生操作污染。
func (e *AppError) clone() *AppError {
	c := *e
	if e.Fields != nil {
		c.Fields = append([]FieldError(nil), e.Fields...)
	}
	if e.Meta != nil {
		c.Meta = make(map[string]any, len(e.Meta))
		for key, value := range e.Meta {
			c.Meta[key] = value
		}
	}
	return &c
}

// WithDetail 派生一个带具体说明的副本。
func (e *AppError) WithDetail(detail string) *AppError {
	c := e.clone()
	c.Detail = detail
	return c
}

// WithCause 派生一个包装底层错误的副本。
func (e *AppError) WithCause(cause error) *AppError {
	c := e.clone()
	c.cause = cause
	return c
}

// WithFields 派生一个携带字段级校验错误的副本。
func (e *AppError) WithFields(fields ...FieldError) *AppError {
	c := e.clone()
	c.Fields = append(c.Fields, fields...)
	return c
}

// WithMeta derives a copy with structured business context.
func (e *AppError) WithMeta(meta map[string]any) *AppError {
	c := e.clone()
	c.Meta = make(map[string]any, len(meta))
	for key, value := range meta {
		c.Meta[key] = value
	}
	return c
}

// Problem 把业务错误转换为 RFC 7807 响应体（供非 Huma 的 chi handler / 中间件经
// WriteProblem 写出），并关联 requestID。Huma 路径不经此方法，而是直接反射序列化
// AppError 自身（字段 tag 与本结构一致）。
func (e *AppError) Problem(requestID string) Problem {
	return Problem{
		Title:     e.Title,
		Status:    e.Status,
		Detail:    e.Detail,
		Code:      e.Code,
		RequestID: requestID,
		Errors:    e.Fields,
		Meta:      e.Meta,
	}
}

// ContentType 满足 Huma 的 huma.ContentTypeFilter 鸭子接口，把错误响应媒体类型
// 固定为 application/problem+json（本方法不引入对 huma 包的依赖）。
func (e *AppError) ContentType(string) string { return ProblemContentType }

// ProblemFrom 把任意 error 归一成 Problem：若链路中存在 *AppError 则按其渲染，
// 否则回退为 500 内部错误（不泄露底层细节）。
func ProblemFrom(err error, requestID string) Problem {
	if ae, ok := errors.AsType[*AppError](err); ok {
		return ae.Problem(requestID)
	}
	return Problem{
		Title:     http.StatusText(http.StatusInternalServerError),
		Status:    http.StatusInternalServerError,
		Code:      "internal",
		RequestID: requestID,
	}
}

// 通用错误模板，覆盖跨服务共用的 HTTP 语义。业务专属错误由各服务在自己的包内用
// New 定义（如 urm 的 "insufficient_balance"）。
var (
	ErrBadRequest   = New("bad_request", http.StatusBadRequest, "Bad Request")
	ErrUnauthorized = New("unauthorized", http.StatusUnauthorized, "Unauthorized")
	ErrForbidden    = New("forbidden", http.StatusForbidden, "Forbidden")
	ErrNotFound     = New("not_found", http.StatusNotFound, "Not Found")
	ErrConflict     = New("conflict", http.StatusConflict, "Conflict")
	ErrValidation   = New("validation_failed", http.StatusUnprocessableEntity, "Validation Failed")
	ErrTooManyReqs  = New("too_many_requests", http.StatusTooManyRequests, "Too Many Requests")
	ErrInternal     = New("internal", http.StatusInternalServerError, "Internal Server Error")
	ErrUnavailable  = New("service_unavailable", http.StatusServiceUnavailable, "Service Unavailable")
)
