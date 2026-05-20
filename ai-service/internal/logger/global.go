package logger

import "go.uber.org/zap"

// SetGlobal replaces the global zap logger used by zap.L() and zap.S().
func SetGlobal(l *zap.Logger) {
	zap.ReplaceGlobals(l)
}
