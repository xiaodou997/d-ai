package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	billingsvc "xiaodou/dai/internal/billing/service"
)

const usageClientUserAgentMaxLen = 512

const (
	usageCompletionMaxAttempts = 3
	usageCompletionRetryBase   = 25 * time.Millisecond
)

// UsageLogger implements serving.UsageLogger.
// It creates the usage log row, upserts the hourly rollup, directly charges the
// account, and computes billing amounts via the unified Price Book model (PriceBookBiller):
// platform (tenant) price, cascaded user price, and the API-key quota debit.
type UsageLogger struct {
	pool              *pgxpool.Pool
	q                 *dbgen.Queries
	biller            usageBiller
	deduction         *billingsvc.DeductionService
	recoveryRedis     *redis.Client
	apiKeyInvalidator apiKeyCacheInvalidator
	logger            *zap.Logger
}

type apiKeyCacheInvalidator interface {
	DelByID(context.Context, string) error
}

type usageBiller interface {
	Calculate(context.Context, *serving.Request) (domain.BillingResult, error)
}

func NewUsageLogger(pool *pgxpool.Pool, biller usageBiller) *UsageLogger {
	return &UsageLogger{
		pool:      pool,
		q:         dbgen.New(pool),
		biller:    biller,
		deduction: billingsvc.NewDeductionService(pool, zap.NewNop()),
		logger:    zap.NewNop(),
	}
}

// WithDeductionService injects the shared in-process direct-charge service.
// The default created by NewUsageLogger keeps standalone recovery/tests safe;
// production wires the same service used by the rest of the application.
func (l *UsageLogger) WithDeductionService(deduction *billingsvc.DeductionService) *UsageLogger {
	if deduction != nil {
		l.deduction = deduction
	}
	return l
}

func (l *UsageLogger) WithRecoveryQueue(client *redis.Client, logger *zap.Logger) *UsageLogger {
	l.recoveryRedis = client
	if logger != nil {
		l.logger = logger
	}
	return l
}

func (l *UsageLogger) WithAPIKeyCacheInvalidator(invalidator apiKeyCacheInvalidator) *UsageLogger {
	l.apiKeyInvalidator = invalidator
	return l
}

// Log records a usage entry regardless of request success/failure.
// BillingResult is calculated here, then usage, API-key quota, subscription
// quota, and the direct balance charge are committed in one transaction.
func (l *UsageLogger) Log(ctx context.Context, req *serving.Request) error {
	subject := req.RuntimeSubject()
	if subject == nil || req.Candidate == nil {
		return nil
	}

	billing, err := l.biller.Calculate(ctx, req)
	if err != nil {
		return fmt.Errorf("calculate prepared billing: %w", err)
	}
	if len(req.Attempts) == 0 && req.RequestStatus == domain.RequestFailed {
		req.TokenUsage = domain.TokenUsage{}
		req.TokenCountSource = ""
		req.UpstreamStatus = 0
		billing = unattemptedBilling(billing)
	}
	req.BillingResult = billing
	var lastErr error
	for attempt := 1; attempt <= usageCompletionMaxAttempts; attempt++ {
		completed, accrued, err := l.logOnce(ctx, req, billing)
		if err == nil {
			if completed {
				l.afterCompletion(req, billing, accrued)
			}
			return nil
		}
		lastErr = err
		if attempt == usageCompletionMaxAttempts {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * usageCompletionRetryBase)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("complete usage after %d attempt(s): %w", attempt, errors.Join(lastErr, ctx.Err()))
		case <-timer.C:
		}
	}
	completionErr := fmt.Errorf("complete usage after %d attempts: %w", usageCompletionMaxAttempts, lastErr)
	if err := l.enqueueRecovery(ctx, req, billing); err != nil {
		return errors.Join(completionErr, fmt.Errorf("enqueue usage recovery: %w", err))
	}
	return fmt.Errorf("%w: %v", ErrUsageCompletionQueued, completionErr)
}

// logOnce performs one idempotent completion attempt. A commit whose result is
// unknown can be retried safely: request_id is unique and every financial
// mutation is in the same transaction as that insert.
func (l *UsageLogger) logOnce(ctx context.Context, req *serving.Request, billing domain.BillingResult) (bool, bool, error) {
	subject := req.RuntimeSubject()
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return false, false, fmt.Errorf("begin usage completion: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := l.q.WithTx(tx)

	if _, err := l.createUsageLog(ctx, qtx, req, billing); errors.Is(err, pgx.ErrNoRows) {
		// request_id is the completion idempotency key. Duplicate completion must
		// not increment API key quota or rollups again.
		return false, false, nil
	} else if err != nil {
		return false, false, fmt.Errorf("create usage log: %w", err)
	}
	if subject.AuthMethod == coreidentity.AuthMethodAPIKey && subject.APIKeyID != "" {
		rows, err := qtx.ConfirmAPIKeyQuotaUsage(ctx, dbgen.ConfirmAPIKeyQuotaUsageParams{
			ID:        mustParseUUID(subject.APIKeyID),
			QuotaUsed: billing.APIKeyQuotaCostMicro,
		})
		if err != nil {
			return false, false, fmt.Errorf("confirm api key quota: %w", err)
		}
		if rows != 1 {
			return false, false, fmt.Errorf("confirm api key quota: key not found")
		}
	}
	accrued, err := l.accrueFinancials(ctx, tx, qtx, req, billing)
	if err != nil {
		return false, false, err
	}
	if err := qtx.UpsertUsageRollupHourly(ctx, buildUsageRollupParams(req, billing)); err != nil {
		return false, false, fmt.Errorf("complete usage rollup: %w", err)
	}
	if err := l.reconcileAsyncTaskCharge(ctx, tx, req, billing); err != nil {
		return false, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, false, fmt.Errorf("commit usage completion: %w", err)
	}
	return true, accrued, nil
}

func (l *UsageLogger) afterCompletion(req *serving.Request, billing domain.BillingResult, accrued bool) {
	l.invalidateAPIKeyCache(req.RuntimeSubject())
}

// reconcileAsyncTaskCharge makes the usage completion authoritative for a
// queued task, including tasks cancelled after upstream work began.
func (l *UsageLogger) reconcileAsyncTaskCharge(ctx context.Context, tx pgx.Tx, req *serving.Request, billing domain.BillingResult) error {
	if req == nil || req.RequestID == "" {
		return nil
	}
	charge := billing.TenantPayableMicro
	if subject := req.RuntimeSubject(); subject != nil && runtimeSubjectOwnerType(subject) == domain.OwnerUser {
		charge = billing.UserChargedMicro
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_async_tasks
		SET caller_charge = GREATEST(caller_charge, $2)
		WHERE request_id = $1
	`, req.RequestID, charge); err != nil {
		return fmt.Errorf("reconcile async task charge: %w", err)
	}
	return nil
}

func (l *UsageLogger) invalidateAPIKeyCache(subject *coreidentity.Subject) {
	if l.apiKeyInvalidator == nil || subject == nil || subject.APIKeyID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.apiKeyInvalidator.DelByID(ctx, subject.APIKeyID); err != nil {
		l.logger.Warn("invalidate API key quota cache failed", zap.String("api_key_id", subject.APIKeyID), zap.Error(err))
	}
}

// accrueFinancials runs in the same transaction as the unique usage insert.
// The usage row is therefore the idempotency anchor for quota, subscription,
// and direct balance charging.
func (l *UsageLogger) accrueFinancials(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	req *serving.Request,
	billing domain.BillingResult,
) (bool, error) {
	subject := req.RuntimeSubject()
	if subject == nil || subject.TenantID == "" {
		return false, nil
	}
	ownerType := runtimeSubjectOwnerType(subject)
	tenantMicro := billing.TenantPayableMicro
	userMicro := billing.UserChargedMicro
	if ownerType == domain.OwnerTenant {
		userMicro = 0
	}

	if req.BillingSource == subscription.BillingSourceSubscription && ownerType == domain.OwnerUser {
		subMicro, ok := serving.SubscriptionDebitMicro(req)
		if !ok {
			// A failed request with no billable output is still an observable usage
			// record, but consumes no package quota. A positive unmeterable amount
			// indicates a broken admission snapshot and must fail closed.
			if billing.RetailBaseMicro > 0 {
				return false, fmt.Errorf("complete subscription billing: admitted usage is not meterable")
			}
			subMicro = 0
		}
		if subMicro > 0 {
			if _, err := q.DebitSubscription(ctx, dbgen.DebitSubscriptionParams{
				ID:             mustParseUUID(req.SubscriptionID),
				Win5hUsedMicro: subMicro,
			}); err != nil {
				return false, fmt.Errorf("complete subscription billing: %w", err)
			}
		}
		userMicro = 0
	}
	if tenantMicro == 0 && userMicro == 0 {
		return false, nil
	}
	if l.deduction == nil {
		return false, errors.New("complete direct billing: deduction service is unavailable")
	}
	result, err := l.deduction.ConsumeTx(ctx, tx, billingsvc.ConsumeParams{
		IdempotencyKey: "ai-usage:" + req.RequestID,
		ClientID:       "dai-ai",
		TenantID:       subject.TenantID,
		UserID:         subject.UserID,
		Description:    "AI 请求额度扣费",
		TenantAmount:   tenantMicro,
		UserAmount:     userMicro,
		AllowOverdraft: true,
	})
	if err != nil {
		return false, fmt.Errorf("complete direct billing: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_usage_logs
		SET billing_event_id = $1, billing_status = $2
		WHERE request_id = $3
	`, result.EventID, string(domain.BillingConfirmed), req.RequestID); err != nil {
		return false, fmt.Errorf("link direct billing event to usage: %w", err)
	}
	return true, nil
}

func usageUserMultiplierOverrideSnapshot(billing domain.BillingResult) pgtype.Numeric {
	if billing.UserMultiplierOverride != nil {
		return floatPtrToNumeric(billing.UserMultiplierOverride)
	}
	return pgtype.Numeric{}
}

// ============================================================================
// DB writes
// ============================================================================

func buildUsageLogParams(req *serving.Request, billing domain.BillingResult) dbgen.CreateUsageLogParams {
	c := req.Candidate
	subject := req.RuntimeSubject()
	usage := req.TokenUsage

	attempted := len(req.Attempts) > 0
	groupTargetUUID := pgtype.UUID{}
	apiKeyUUID := pgtype.UUID{}
	if subject != nil && subject.APIKeyID != "" {
		apiKeyUUID = mustParseUUID(subject.APIKeyID)
	}

	// 账号级路由：命中目标记 endpoint_id(账号)或 credential_pool_id(池)。
	endpointUUID := pgtype.UUID{}
	poolUUID := pgtype.UUID{}
	var credUUID pgtype.UUID
	if attempted {
		groupTargetUUID = mustParseUUID(c.RouteID)
		endpointUUID = mustParseUUID(c.EndpointID)
		poolUUID = mustParseUUID(c.PoolID)
	}
	if attempted && req.SelectedCredential != nil {
		credUUID = mustParseUUID(req.SelectedCredential.ID)
	}

	// Pool routes use PoolUpstreamModel; direct routes use UpstreamModel.
	upstreamModel, providerCode, providerFormat := "", "", ""
	resolvedProviderFamily := ""
	protocolConversionEnabled := false
	upstreamModelMappingApplied := false
	if attempted {
		upstreamModel = c.UpstreamModel
		if c.IsPoolRoute() {
			upstreamModel = c.PoolUpstreamModel
		}
		providerCode = c.ProviderCode
		providerFormat = string(c.Protocol)
		resolvedProviderFamily = req.ResolvedProviderFamily
		protocolConversionEnabled = req.ProtocolConversionEnabled
		upstreamModelMappingApplied = req.UpstreamModelMappingApplied
	}
	requestTotalMs, hasRequestTotalMs := req.RequestTotalMs()
	requestSetupMs, hasRequestSetupMs := req.RequestSetupMs()
	firstResponseByteMs, hasFirstResponseByteMs := req.FirstResponseByteDurationMs()
	responseTailMs, hasResponseTailMs := req.ResponseTailMs()
	finalAttemptHeaderMs, hasFinalAttemptHeaderMs := req.FinalAttemptHeaderMs()
	finalAttemptTotalMs, hasFinalAttemptTotalMs := req.FinalAttemptTotalMs()

	params := dbgen.CreateUsageLogParams{
		RequestID:                          req.RequestID,
		TraceID:                            nullableText(req.TraceID),
		ApiKeyID:                           apiKeyUUID,
		KeyOwnerType:                       string(runtimeSubjectOwnerType(subject)),
		AuthMethod:                         string(runtimeSubjectAuthMethod(subject)),
		RequestSource:                      string(runtimeSubjectRequestSource(subject)),
		TenantID:                           runtimeSubjectTenantID(subject),
		UserID:                             nullableText(runtimeSubjectUserID(subject)),
		ClientUserAgent:                    usageClientUserAgent(req),
		GroupID:                            nullableUUID(c.GroupID),
		GroupNameSnapshot:                  billing.GroupNameSnapshot,
		GroupDefaultUserMultiplierSnapshot: floatToNumeric(billing.GroupDefaultUserMultiplier),
		UserMultiplierOverrideSnapshot:     usageUserMultiplierOverrideSnapshot(billing),
		EffectiveUserMultiplierSnapshot:    floatToNumeric(billing.EffectiveUserMultiplier),
		BillingGroupLabelSnapshot:          billing.BillingGroupLabel,
		ModelCode:                          req.ModelCode,
		RequestedModel:                     req.PublicModel(),
		MatchedDispatchRuleID:              nullableUUID(req.MatchedDispatchRuleID),
		MatchedDispatchRuleSummary:         nullableText(req.MatchedDispatchRuleSummary),
		ResolvedLogicalModel:               nullableText(req.ResolvedLogicalModel),
		ResolvedProviderFamily:             nullableText(resolvedProviderFamily),
		CapabilityType:                     string(req.CapabilityType),
		GroupTargetID:                      groupTargetUUID,
		EndpointID:                         endpointUUID,
		CredentialPoolID:                   poolUUID,
		OauthCredentialID:                  credUUID,
		ProviderCode:                       nullableText(providerCode),
		UpstreamModel:                      nullableText(upstreamModel),
		ProviderFormat:                     nullableText(providerFormat),
		Stream:                             req.IsStream,
		PromptTokens:                       int32(usage.PromptTokens),
		CompletionTokens:                   int32(usage.CompletionTokens),
		CacheWriteTokens:                   int32(usage.CacheWriteTokens),
		CacheReadTokens:                    int32(usage.CacheReadTokens),
		ReasoningTokens:                    int32(usage.ReasoningTokens),
		ReasoningEffort:                    nullableText(req.ReasoningEffort),
		TotalTokens:                        int32(usage.TotalTokens()),
		BillableUnitType:                   billing.BillableUnitType,
		BillableUnits:                      billing.BillableUnits,
		CatalogBase:                        billing.CatalogBaseMicro,
		TenantPayable:                      billing.TenantPayableMicro,
		RetailBase:                         billing.RetailBaseMicro,
		UserPayable:                        billing.UserPayableMicro,
		UserCharged:                        billing.UserChargedMicro,
		ApiKeyQuotaCost:                    billing.APIKeyQuotaCostMicro,
		ServiceTier:                        string(billing.ServiceTier),
		BillingBreakdown:                   billing.BillingBreakdownJSON,
		BillingEventID:                     pgtype.Text{},
		BillingStatus:                      billingStatus(req),
		RequestStatus:                      string(req.RequestStatus),
		HttpStatus:                         nullableInt4(req.HTTPStatus),
		UpstreamStatus:                     nullableInt4(req.UpstreamStatus),
		LatencyMs:                          nullableInt4(req.LatencyMs),
		FirstTokenLatencyMs:                nullableInt4(req.FirstTokenMs),
		RequestTotalMs:                     nullableInt4WithValid(requestTotalMs, hasRequestTotalMs),
		RequestSetupMs:                     nullableInt4WithValid(requestSetupMs, hasRequestSetupMs),
		FirstResponseByteMs:                nullableInt4WithValid(firstResponseByteMs, hasFirstResponseByteMs),
		ResponseTailMs:                     nullableInt4WithValid(responseTailMs, hasResponseTailMs),
		FinalAttemptHeaderMs:               nullableInt4WithValid(finalAttemptHeaderMs, hasFinalAttemptHeaderMs),
		FinalAttemptTotalMs:                nullableInt4WithValid(finalAttemptTotalMs, hasFinalAttemptTotalMs),
		ErrorCode:                          nullableText(req.ErrorCode),
		ErrorMessage:                       nullableText(req.ErrorMessage),
		UsageEstimated:                     req.TokenCountSource == domain.TokenUsageSourceEstimated || req.TokenCountSource == domain.TokenUsageSourceMixed,
		TokenUsageSource:                   tokenCountSource(req.TokenCountSource),
		AttemptsCount:                      int32(len(req.Attempts)),
		FinalRouteID:                       groupTargetUUID,
		ClientProtocol:                     string(req.ClientProtocol),
		Resolution:                         nullableText(resolution(req)),
		ProtocolConversionEnabled:          protocolConversionEnabled,
		UpstreamModelMappingApplied:        upstreamModelMappingApplied,
		PublicResponseModel:                nullableText(req.PublicModel()),
		// 计费来源快照：SubscriptionGateStep 在管线里判定后写入 req。空串
		// 兜底为 payg，防未经 gate 的路径违反 CHECK IN ('payg','subscription')。
		BillingSource:  usageBillingSource(req.BillingSource),
		SubscriptionID: nullableUUID(req.SubscriptionID),
	}

	return params
}

func (l *UsageLogger) createUsageLog(ctx context.Context, q *dbgen.Queries, req *serving.Request, billing domain.BillingResult) (pgtype.UUID, error) {
	return q.CreateUsageLog(ctx, buildUsageLogParams(req, billing))
}

func usageClientUserAgent(req *serving.Request) string {
	if req == nil || req.Envelope == nil || req.Envelope.R == nil {
		return ""
	}
	ua := req.Envelope.R.Header.Get("User-Agent")
	if len(ua) > usageClientUserAgentMaxLen {
		return ua[:usageClientUserAgentMaxLen]
	}
	return ua
}

func buildUsageRollupParams(req *serving.Request, billing domain.BillingResult) dbgen.UpsertUsageRollupHourlyParams {
	subject := req.RuntimeSubject()
	usage := req.TokenUsage
	apiKeyID := mustParseUUID("00000000-0000-0000-0000-000000000000")
	if subject != nil && subject.APIKeyID != "" {
		apiKeyID = mustParseUUID(subject.APIKeyID)
	}
	requestTotalMs, hasRequestTotalMs := req.RequestTotalMs()
	firstResponseByteMs, hasFirstResponseByteMs := req.FirstResponseByteDurationMs()
	providerCode := ""
	if len(req.Attempts) > 0 && req.Candidate != nil {
		providerCode = req.Candidate.ProviderCode
	}

	return dbgen.UpsertUsageRollupHourlyParams{
		TenantID:            runtimeSubjectTenantID(subject),
		UserID:              nullableText(runtimeSubjectUserID(subject)),
		ApiKeyID:            apiKeyID,
		RequestSource:       string(runtimeSubjectRequestSource(subject)),
		CapabilityType:      string(req.CapabilityType),
		ModelCode:           req.ModelCode,
		ProviderCode:        nullableText(providerCode),
		RequestStatus:       string(req.RequestStatus),
		BillableUnitType:    billing.BillableUnitType,
		PromptTokens:        int64(usage.PromptTokens),
		CompletionTokens:    int64(usage.CompletionTokens),
		CacheWriteTokens:    int64(usage.CacheWriteTokens),
		CacheReadTokens:     int64(usage.CacheReadTokens),
		ReasoningTokens:     int64(usage.ReasoningTokens),
		TotalTokens:         int64(usage.TotalTokens()),
		BillableUnits:       billing.BillableUnits,
		CatalogBase:         billing.CatalogBaseMicro,
		TenantPayable:       billing.TenantPayableMicro,
		RetailBase:          billing.RetailBaseMicro,
		UserPayable:         billing.UserPayableMicro,
		UserCharged:         billing.UserChargedMicro,
		ApiKeyQuotaCost:     billing.APIKeyQuotaCostMicro,
		LatencyMs:           nullableInt4(req.LatencyMs),
		RequestTotalMs:      nullableInt4WithValid(requestTotalMs, hasRequestTotalMs),
		FirstResponseByteMs: nullableInt4WithValid(firstResponseByteMs, hasFirstResponseByteMs),
	}
}

func unattemptedBilling(source domain.BillingResult) domain.BillingResult {
	source.CatalogBaseMicro = 0
	source.TenantPayableMicro = 0
	source.RetailBaseMicro = 0
	source.UserPayableMicro = 0
	source.UserChargedMicro = 0
	source.APIKeyQuotaCostMicro = 0
	source.BillableUnits = 0
	source.BillingBreakdownJSON = []byte(`{"reason":"upstream_not_attempted"}`)
	return source
}

// tokenCountSource normalises the source label; defaults to "upstream" when empty
// (e.g. failed requests where Execute never reached the estimation path).
func tokenCountSource(s string) string {
	switch s {
	case domain.TokenUsageSourceEstimated, domain.TokenUsageSourceMixed, domain.TokenUsageSourceUpstream:
		return s
	default:
		return domain.TokenUsageSourceUpstream
	}
}

// resolution returns the request resolution for image/video billing audit,
// or empty string for token-based requests.
func resolution(req *serving.Request) string {
	if req.TokenUsage.ImageResolution != "" {
		return req.TokenUsage.ImageResolution
	}
	return req.TokenUsage.VideoResolution
}

// billingStatus returns the status written with the usage record. Direct
// charging is in the same transaction, so a positive amount is confirmed
// before the usage row becomes visible.
// usageBillingSource coalesces an empty gate decision to "payg" so the usage
// log never violates the billing_source CHECK constraint.
func usageBillingSource(src string) string {
	if src == "" {
		return "payg"
	}
	return src
}

func billingStatus(req *serving.Request) string {
	if req.BillingResult.TenantPayableMicro == 0 && req.BillingResult.UserChargedMicro == 0 {
		return string(domain.BillingFree)
	}
	return string(domain.BillingConfirmed)
}
