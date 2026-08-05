package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func newRedactTestLogger(appEnv string, redactFields []string) (*zap.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = ""
	encoderCfg.LevelKey = ""
	encoderCfg.NameKey = ""
	encoderCfg.CallerKey = ""
	encoderCfg.MessageKey = "msg"

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(&buf),
		zap.DebugLevel,
	)
	if len(redactFields) == 0 {
		redactFields = DefaultRedactFields()
	}
	core = newRedactCore(core, appEnv, redactFields)
	return zap.New(core), &buf
}

func TestRedactCoreExactMatch(t *testing.T) {
	logger, buf := newRedactTestLogger("production", nil)

	logger.Info("test",
		zap.String("api_key", "sk-secret"),
		zap.String("authorization", "Bearer token"),
		zap.String("password", "hunter2"),
		zap.String("access_token", "access"),
		zap.String("refresh_token", "refresh"),
		zap.String("bearer_token", "bearer"),
		zap.String("client_secret", "client"),
	)

	entry := decodeLogLine(t, buf)
	for _, field := range []string{
		"api_key",
		"authorization",
		"password",
		"access_token",
		"refresh_token",
		"bearer_token",
		"client_secret",
	} {
		if got := entry[field]; got != "[REDACTED]" {
			t.Fatalf("%s = %v, want [REDACTED]", field, got)
		}
	}
}

func TestRedactCoreNoTokenFalsePositives(t *testing.T) {
	logger, buf := newRedactTestLogger("production", nil)

	logger.Info("test",
		zap.String("prompt_tokens", "150"),
		zap.String("completion_tokens", "42"),
		zap.String("first_token_ms", "120"),
		zap.Int("total_tokens", 192),
	)

	entry := decodeLogLine(t, buf)
	if got := entry["prompt_tokens"]; got != "150" {
		t.Fatalf("prompt_tokens = %v, want 150", got)
	}
	if got := entry["completion_tokens"]; got != "42" {
		t.Fatalf("completion_tokens = %v, want 42", got)
	}
	if got := entry["first_token_ms"]; got != "120" {
		t.Fatalf("first_token_ms = %v, want 120", got)
	}
	if got := entry["total_tokens"]; got != float64(192) {
		t.Fatalf("total_tokens = %v, want 192", got)
	}
}

func TestRedactCoreSkipsDevelopment(t *testing.T) {
	logger, buf := newRedactTestLogger("development", nil)

	logger.Info("test", zap.String("api_key", "sk-secret"))

	entry := decodeLogLine(t, buf)
	if got := entry["api_key"]; got != "sk-secret" {
		t.Fatalf("api_key = %v, want original value", got)
	}
}

func decodeLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("decode log line: %v; line=%s", err, buf.String())
	}
	return entry
}
