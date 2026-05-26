package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestLogger creates a zap.Logger that writes JSON to a buffer, using the
// given appEnv and redact fields. It returns the logger and the buffer so the
// caller can inspect the output.
func newTestLogger(appEnv string, redactFields []string) (*zap.Logger, *bytes.Buffer) {
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
		redactFields = defaultRedactFields()
	}
	core = newRedactCore(core, appEnv, redactFields)
	return zap.New(core), &buf
}

// logOutput is a minimal struct for unmarshalling a single log line.
type logOutput struct {
	Msg   string `json:"msg"`
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------
// normalizeKey tests
// ---------------------------------------------------------------------------

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"api_key", "apikey"},
		{"API_KEY", "apikey"},
		{"api-key", "apikey"},
		{"API-KEY", "apikey"},
		{"apikey", "apikey"},
		{"APIKEY", "apikey"},
		{"  api_key  ", "apikey"},
		{"prompt_tokens", "prompttokens"},
		{"completion_tokens", "completiontokens"},
		{"first_token_ms", "firsttokenms"},
		{"access_token", "accesstoken"},
		{"refresh_token", "refreshtoken"},
		{"bearer_token", "bearertoken"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeKey(tt.input)
			if got != tt.want {
				t.Errorf("normalizeKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Exact match tests (production environment)
// ---------------------------------------------------------------------------

func TestRedactCore_ExactMatch_Production(t *testing.T) {
	logger, buf := newTestLogger("production", nil)

	// Fields that MUST be redacted.
	logger.Info("test",
		zap.String("api_key", "sk-secret-123"),
		zap.String("authorization", "Bearer abc"),
		zap.String("password", "hunter2"),
		zap.String("access_token", "tok-abc"),
		zap.String("refresh_token", "tok-ref"),
		zap.String("bearer_token", "tok-bearer"),
		zap.String("secret", "s3cret"),
		zap.String("client_secret", "cs-123"),
		zap.String("provider_key", "pk-123"),
		zap.String("ciphertext", "ct-xyz"),
	)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal(lines[0], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	redactedFields := []string{
		"api_key", "authorization", "password",
		"access_token", "refresh_token", "bearer_token",
		"secret", "client_secret", "provider_key", "ciphertext",
	}
	for _, field := range redactedFields {
		val, ok := entry[field]
		if !ok {
			t.Errorf("field %q missing from log output", field)
			continue
		}
		if val != "[REDACTED]" {
			t.Errorf("field %q = %v, want [REDACTED]", field, val)
		}
	}
}

func TestRedactCore_NoFalsePositives_Production(t *testing.T) {
	logger, buf := newTestLogger("production", nil)

	// Fields that MUST NOT be redacted — these were the original victims of
	// the strings.Contains("token") false positive.
	logger.Info("test",
		zap.String("prompt_tokens", "150"),
		zap.String("completion_tokens", "42"),
		zap.String("first_token_ms", "120"),
		zap.String("model_code", "gpt-5.4-mini"),
		zap.String("request_id", "abc-123"),
		zap.String("latency_ms", "3543"),
		zap.Int("total_tokens", 192),
	)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal(lines[0], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	safeFields := []string{
		"prompt_tokens", "completion_tokens", "first_token_ms",
		"model_code", "request_id", "latency_ms",
	}
	for _, field := range safeFields {
		val, ok := entry[field]
		if !ok {
			t.Errorf("field %q missing from log output", field)
			continue
		}
		if val == "[REDACTED]" {
			t.Errorf("field %q was incorrectly redacted", field)
		}
	}

	// total_tokens is an int field; after redaction it would become a string
	// "[REDACTED]". Verify it's still a number.
	if tt, ok := entry["total_tokens"]; ok {
		if _, isFloat := tt.(float64); !isFloat {
			t.Errorf("total_tokens = %v (%T), want number (not redacted)", tt, tt)
		}
	}
}

// ---------------------------------------------------------------------------
// Environment-aware tests
// ---------------------------------------------------------------------------

func TestRedactCore_Development_NoRedaction(t *testing.T) {
	logger, buf := newTestLogger("development", nil)

	logger.Info("test",
		zap.String("api_key", "sk-secret-123"),
		zap.String("password", "hunter2"),
		zap.String("access_token", "tok-abc"),
	)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal(lines[0], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// In development, nothing should be redacted.
	for _, field := range []string{"api_key", "password", "access_token"} {
		val, ok := entry[field]
		if !ok {
			t.Errorf("field %q missing from log output", field)
			continue
		}
		if val == "[REDACTED]" {
			t.Errorf("field %q was redacted in development mode", field)
		}
	}
}

// ---------------------------------------------------------------------------
// Custom redact list tests
// ---------------------------------------------------------------------------

func TestRedactCore_CustomRedactFields(t *testing.T) {
	// Only redact "custom_secret"; default fields like "password" should pass through.
	logger, buf := newTestLogger("production", []string{"custom_secret"})

	logger.Info("test",
		zap.String("custom_secret", "shh"),
		zap.String("password", "hunter2"),
	)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal(lines[0], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if val, ok := entry["custom_secret"]; !ok || val != "[REDACTED]" {
		t.Errorf("custom_secret = %v, want [REDACTED]", val)
	}
	if val, ok := entry["password"]; !ok || val == "[REDACTED]" {
		t.Errorf("password was redacted with custom list (should only redact custom_secret)")
	}
}

// ---------------------------------------------------------------------------
// Variant normalisation tests
// ---------------------------------------------------------------------------

func TestRedactKey_NormalisedVariants(t *testing.T) {
	// "api_key" in the list should also catch "API-KEY", "apikey", etc.
	logger, buf := newTestLogger("production", []string{"api_key"})

	logger.Info("test",
		zap.String("api_key", "val1"),
		zap.String("apikey", "val2"),
		zap.String("API-KEY", "val3"),
	)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal(lines[0], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, field := range []string{"api_key", "apikey", "API-KEY"} {
		val, ok := entry[field]
		if !ok {
			t.Errorf("field %q missing from log output", field)
			continue
		}
		if val != "[REDACTED]" {
			t.Errorf("field %q = %v, want [REDACTED] (normalised variant of api_key)", field, val)
		}
	}
}
