package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

func New() *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	handlerOptions := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) == "development" {
		handler = slog.NewTextHandler(os.Stdout, handlerOptions)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, handlerOptions)
	}

	handler = newRedactHandler(handler, defaultRedactFields())
	return slog.New(handler)
}

func defaultRedactFields() []string {
	return []string{
		"authorization",
		"api_key",
		"apikey",
		"provider_key",
		"providerkey",
		"appsecret",
		"password",
		"token",
		"secret",
		"ciphertext",
	}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type redactHandler struct {
	next   slog.Handler
	fields map[string]struct{}
}

func newRedactHandler(next slog.Handler, fields []string) slog.Handler {
	normalized := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		key := normalizeKey(field)
		if key != "" {
			normalized[key] = struct{}{}
		}
	}
	return redactHandler{next: next, fields: normalized}
}

func (h redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(h.redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, h.redactAttr(attr))
	}
	return redactHandler{next: h.next.WithAttrs(redacted), fields: h.fields}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{next: h.next.WithGroup(name), fields: h.fields}
}

func (h redactHandler) redactAttr(attr slog.Attr) slog.Attr {
	if h.shouldRedact(attr.Key) {
		return slog.String(attr.Key, "[REDACTED]")
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		redactedGroup := make([]slog.Attr, 0, len(group))
		for _, groupAttr := range group {
			redactedGroup = append(redactedGroup, h.redactAttr(groupAttr))
		}
		return slog.Group(attr.Key, attrsToAny(redactedGroup)...)
	}
	return attr
}

func (h redactHandler) shouldRedact(key string) bool {
	normalizedKey := normalizeKey(key)
	if _, ok := h.fields[normalizedKey]; ok {
		return true
	}
	for field := range h.fields {
		if strings.Contains(normalizedKey, field) {
			return true
		}
	}
	return false
}

func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attr)
	}
	return out
}

