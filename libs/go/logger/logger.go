// Package logger provides unified zap-based logging across UniHub services.
//
// Design principles:
//   - Development: ConsoleEncoder with colorized levels, human-readable key=value output
//   - Production:  JSONEncoder, structured fields for log aggregation
//   - File output:  lumberjack rotation (configurable)
//   - Security:     sensitive field redaction via zapcore.Core wrapper
//   - Observability: caller info + error-level stacktraces
package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogConfig holds logging configuration. Embed this in your service's config struct.
type LogConfig struct {
	// Level: debug | info | warn | error
	Level string `mapstructure:"level"`
	// File: log file path (empty = console only)
	File string `mapstructure:"file"`
	// MaxSize: lumberjack max size in MB (default 100)
	MaxSize int `mapstructure:"max_size"`
	// MaxBackups: lumberjack max backup count (default 30)
	MaxBackups int `mapstructure:"max_backups"`
	// MaxAge: lumberjack max age in days (default 30)
	MaxAge int `mapstructure:"max_age"`
	// Redact: custom sensitive field names; empty uses DefaultRedactFields.
	Redact []string `mapstructure:"redact"`
}

// InitLogger creates a *zap.Logger with unified formatting rules.
//
// appEnv: "development" enables ConsoleEncoder + DebugLevel;
//
//	any other value enables JSONEncoder + InfoLevel.
//
// cfg: LogConfig for level override and file output.
func InitLogger(appEnv string, cfg LogConfig) *zap.Logger {
	// Determine log level
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

	fields := cfg.Redact
	if len(fields) == 0 {
		fields = DefaultRedactFields()
	}
	core = newRedactCore(core, appEnv, fields)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

// SetGlobal replaces the global zap logger used by zap.L() and zap.S().
func SetGlobal(l *zap.Logger) {
	zap.ReplaceGlobals(l)
}

// consoleTimeEncoder outputs time in local timezone, precision to seconds.
// Format: 2026-05-20 10:56:35
// Only used for ConsoleEncoder (development). Production JSON uses ISO8601TimeEncoder.
func consoleTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Local().Format("2006-01-02 15:04:05"))
}
