package logger

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogConfig holds logging configuration.
type LogConfig struct {
	Level      string
	File       string
	MaxSize    int
	MaxBackups int
	MaxAge     int
}

// InitLogger creates a *zap.Logger with unified formatting rules.
func InitLogger(appEnv string, cfg LogConfig) *zap.Logger {
	logLevel := zap.InfoLevel
	if appEnv == "development" {
		logLevel = zap.DebugLevel
	}
	if cfg.Level != "" {
		if parsed, err := zapcore.ParseLevel(cfg.Level); err == nil {
			logLevel = parsed
		}
	}

	// ---- Console core ----
	var consoleCore zapcore.Core
	if appEnv == "development" {
		devEncoderCfg := zap.NewDevelopmentEncoderConfig()
		devEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		devEncoderCfg.EncodeDuration = zapcore.StringDurationEncoder
		devEncoderCfg.TimeKey = "time"
		devEncoderCfg.EncodeTime = consoleTimeEncoder
		consoleCore = zapcore.NewCore(
			zapcore.NewConsoleEncoder(devEncoderCfg),
			zapcore.AddSync(os.Stdout),
			logLevel,
		)
	} else {
		prodEncoderCfg := zap.NewProductionEncoderConfig()
		prodEncoderCfg.TimeKey = "time"
		prodEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		prodEncoderCfg.EncodeDuration = zapcore.MillisDurationEncoder
		consoleCore = zapcore.NewCore(
			zapcore.NewJSONEncoder(prodEncoderCfg),
			zapcore.AddSync(os.Stdout),
			logLevel,
		)
	}

	// ---- File core (optional) ----
	var cores []zapcore.Core
	cores = append(cores, consoleCore)

	if cfg.File != "" {
		maxSize := cfg.MaxSize
		if maxSize <= 0 {
			maxSize = 100
		}
		maxBackups := cfg.MaxBackups
		if maxBackups <= 0 {
			maxBackups = 30
		}
		maxAge := cfg.MaxAge
		if maxAge <= 0 {
			maxAge = 30
		}
		_ = os.MkdirAll("./logs", 0755)

		fileEncoderCfg := zap.NewProductionEncoderConfig()
		fileEncoderCfg.TimeKey = "time"
		fileEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		fileEncoderCfg.EncodeDuration = zapcore.MillisDurationEncoder

		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   true,
		})
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(fileEncoderCfg),
			fileWriter,
			logLevel,
		)
		cores = append(cores, fileCore)
	}

	core := zapcore.NewTee(cores...)
	core = newRedactCore(core, defaultRedactFields())

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

type redactCore struct {
	next   zapcore.Core
	fields map[string]struct{}
}

func newRedactCore(next zapcore.Core, fields []string) zapcore.Core {
	normalized := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		key := normalizeKey(field)
		if key != "" {
			normalized[key] = struct{}{}
		}
	}
	return redactCore{next: next, fields: normalized}
}

func (c redactCore) Enabled(level zapcore.Level) bool {
	return c.next.Enabled(level)
}

func (c redactCore) With(fields []zapcore.Field) zapcore.Core {
	redacted := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		redacted = append(redacted, c.redactField(field))
	}
	return redactCore{next: c.next.With(redacted), fields: c.fields}
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
	normalizedKey := normalizeKey(key)
	if _, ok := c.fields[normalizedKey]; ok {
		return true
	}
	for field := range c.fields {
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

// consoleTimeEncoder outputs time in local timezone, precision to seconds.
// Format: 2026-05-20 10:56:35
func consoleTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Local().Format("2006-01-02 15:04:05"))
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
		"client_secret",
		"access_token",
		"refresh_token",
	}
}

var _ zapcore.Core = redactCore{}
