package observability

import "go.uber.org/zap"

// IdentityEnrichmentLogger records fail-open identity enrichment errors.
type IdentityEnrichmentLogger struct {
	logger *zap.Logger
}

func NewIdentityEnrichmentLogger(logger *zap.Logger) *IdentityEnrichmentLogger {
	return &IdentityEnrichmentLogger{logger: logger}
}

func (l *IdentityEnrichmentLogger) ObserveFailure(kind string, err error) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Warn("identity enrichment failed open", zap.String("kind", kind), zap.Error(err))
}
