package logger

import (
	"strings"

	"go.uber.org/zap/zapcore"
)

// redactCore wraps a zapcore.Core to redact sensitive fields.
type redactCore struct {
	next   zapcore.Core
	fields map[string]struct{}
	env    string
}

// newRedactCore creates a zapcore.Core that redacts values for matching field keys.
func newRedactCore(next zapcore.Core, appEnv string, fields []string) zapcore.Core {
	normalized := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		key := normalizeKey(field)
		if key != "" {
			normalized[key] = struct{}{}
		}
	}
	return redactCore{next: next, fields: normalized, env: appEnv}
}

func (c redactCore) Enabled(level zapcore.Level) bool {
	return c.next.Enabled(level)
}

func (c redactCore) With(fields []zapcore.Field) zapcore.Core {
	redacted := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		redacted = append(redacted, c.redactField(field))
	}
	return redactCore{next: c.next.With(redacted), fields: c.fields, env: c.env}
}

func (c redactCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c redactCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	redacted := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		redacted = append(redacted, c.redactField(field))
	}
	return c.next.Write(entry, redacted)
}

func (c redactCore) Sync() error {
	return c.next.Sync()
}

func (c redactCore) redactField(field zapcore.Field) zapcore.Field {
	if c.shouldRedact(field.Key) {
		return zapcore.Field{
			Key:    field.Key,
			Type:   zapcore.StringType,
			String: "[REDACTED]",
		}
	}
	return field
}

func (c redactCore) shouldRedact(key string) bool {
	if c.env == "development" {
		return false
	}
	normalizedKey := normalizeKey(key)
	_, ok := c.fields[normalizedKey]
	return ok
}

func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

// DefaultRedactFields returns the standard list of field keys whose values
// should be redacted in all log output.
func DefaultRedactFields() []string {
	return []string{
		"authorization",
		"api_key",
		"apikey",
		"provider_key",
		"providerkey",
		"appsecret",
		"password",
		"secret",
		"ciphertext",
		"client_secret",
		"access_token",
		"refresh_token",
		"bearer_token",
	}
}

// Ensure redactCore satisfies zapcore.Core at compile time.
var _ zapcore.Core = redactCore{}
