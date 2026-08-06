// Package server 提供 D-AI 后端的 HTTP 基座：chi 路由 + Huma v2
// （code-first OpenAPI）+ 统一中间件链，并把 Huma 内部错误（请求体校验、路由未命中
// 等）一并归一为 RFC 7807 application/problem+json。
package server

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	"xiaodou/dai/libs/go/httpx"
	"xiaodou/dai/libs/go/logger"
)

func init() {
	// 覆写 Huma 的错误工厂，使框架自身产生的错误（如 422 请求校验、404 路由未命中）
	// 也走 D-AI 统一的 problem+json 模型，与业务 AppError 同形。
	huma.NewErrorWithContext = func(ctx huma.Context, status int, message string, errs ...error) huma.StatusError {
		ae := &httpx.AppError{Status: status, Title: http.StatusText(status), Detail: message}
		switch status {
		case http.StatusUnprocessableEntity:
			ae.Code = "validation_failed"
		case http.StatusInternalServerError:
			ae.Code = "internal"
		}
		for _, e := range errs {
			if d, ok := e.(huma.ErrorDetailer); ok {
				det := d.ErrorDetail()
				ae.Fields = append(ae.Fields, httpx.FieldError{Field: det.Location, Message: det.Message})
			}
		}
		if ctx != nil {
			logger.RecordRequestError(ctx.Context(), ae)
		}
		return ae
	}
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		return huma.NewErrorWithContext(nil, status, message, errs...)
	}
}

// Options 配置一个服务基座。
type Options struct {
	// Title / Version 写入生成的 OpenAPI 文档。
	Title   string
	Version string
	// Logger 为请求日志与 panic 日志使用；为 nil 时跳过结构化日志。
	Logger *zap.Logger
	// CORSOrigins 是允许的跨域来源；为空表示不挂 CORS 中间件。
	CORSOrigins []string
}

// New 构造一个挂好统一中间件链的 chi 路由与 Huma API。
//
// 中间件顺序（自外向内）：RequestID → CORS → 请求日志 → Recoverer。
// 这样跨域头与 request_id 对所有响应（含 panic 的 500）均可用。
func New(opts Options) (*chi.Mux, huma.API) {
	r := chi.NewMux()

	r.Use(chimw.RequestID)
	if len(opts.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   opts.CORSOrigins,
			AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
			AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-ID"},
			ExposedHeaders:   []string{"X-Request-ID"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}
	if opts.Logger != nil {
		r.Use(logger.ChiRequestLogger(opts.Logger))
	}
	r.Use(Recoverer(opts.Logger))

	cfg := huma.DefaultConfig(opts.Title, opts.Version)
	cfg.Transformers = append([]huma.Transformer{recordErrorResponse}, cfg.Transformers...)
	api := humachi.New(r, cfg)
	return r, api
}

func recordErrorResponse(ctx huma.Context, status string, v any) (any, error) {
	if err, ok := v.(error); ok {
		logger.RecordRequestError(ctx.Context(), err)
	}
	return v, nil
}

// Recoverer 捕获 handler panic，记录日志并写出统一的 problem+json 500 响应。
func Recoverer(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// http.ErrAbortHandler 是 net/http 的约定信号，需原样向上抛。
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				reqID := chimw.GetReqID(r.Context())
				if log != nil {
					log.Error("HTTP handler panic",
						zap.Any("panic", rec),
						zap.String("request_id", reqID),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
						zap.Stack("stack"),
					)
				}
				httpx.WriteProblem(w, httpx.Problem{
					Status:    http.StatusInternalServerError,
					Code:      "internal",
					RequestID: reqID,
				})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
