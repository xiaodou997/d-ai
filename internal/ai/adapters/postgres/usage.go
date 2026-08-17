package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/billing/outbox"
)

const usageClientUserAgentMaxLen = 512

const (
	usageCompletionMaxAttempts = 5
	usageCompletionRetryBase   = 50 * time.Millisecond
)

// UsageLogger implements serving.UsageLogger.
// It creates the usage log row, upserts the hourly rollup, enqueues the balance
// charge, and computes billing amounts via the unified Price Book model
// (PriceBookBiller): platform (tenant) price, cascaded user price, and the
// API-key quota debit.
//
// It does not move money itself. The charge is enqueued on bill_charge_outbox
// in the same transaction as the usage row, and applied to the ledger by
// billing/outbox shortly afterwards. Keeping the account update out of the
// request transaction is what lets requests for one tenant settle in parallel.
type UsageLogger struct {
	pool              *pgxpool.Pool
	q                 *dbgen.Queries
	biller            usageBiller
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
		pool:   pool,
		q:      dbgen.New(pool),
		biller: biller,
		logger: zap.NewNop(),
	}
}

func (l *UsageLogger) WithLogger(logger *zap.Logger) *UsageLogger {
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
	backoff := usageCompletionRetryBase
	for attempt := 1; attempt <= usageCompletionMaxAttempts; attempt++ {
		completed, err := l.logOnce(ctx, req, billing)
		if err == nil {
			if completed {
				l.invalidateAPIKeyCache(req.RuntimeSubject())
			}
			return nil
		}
		lastErr = err
		if attempt == usageCompletionMaxAttempts {
			break
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("complete usage after %d attempt(s): %w", attempt, errors.Join(lastErr, ctx.Err()))
		case <-timer.C:
		}
		backoff *= 3
	}
	// Nothing was committed, so nothing is half-recorded: the usage row and the
	// charge share one transaction. Reaching here means PostgreSQL was
	// unreachable for the whole retry window, which the admission gate would
	// also have failed closed on.
	return fmt.Errorf("complete usage after %d attempts: %w", usageCompletionMaxAttempts, lastErr)
}

// logOnce performs one idempotent completion attempt. A commit whose result is
// unknown can be retried safely: request_id is unique and every financial
// mutation is in the same transaction as that insert.
func (l *UsageLogger) logOnce(ctx context.Context, req *serving.Request, billing domain.BillingResult) (bool, error) {
	subject := req.RuntimeSubject()
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin usage completion: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := l.q.WithTx(tx)

	if _, err := l.createUsageLog(ctx, qtx, req, billing); errors.Is(err, pgx.ErrNoRows) {
		// request_id is the completion idempotency key. Duplicate completion must
		// not increment API key quota or rollups again.
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("create usage log: %w", err)
	}
	if subject.AuthMethod == coreidentity.AuthMethodAPIKey && subject.APIKeyID != "" {
		rows, err := qtx.ConfirmAPIKeyQuotaUsage(ctx, dbgen.ConfirmAPIKeyQuotaUsageParams{
			ID:        mustParseUUID(subject.APIKeyID),
			QuotaUsed: billing.APIKeyQuotaCostMicro,
		})
		if err != nil {
			return false, fmt.Errorf("confirm api key quota: %w", err)
		}
		if rows != 1 {
			return false, fmt.Errorf("confirm api key quota: key not found")
		}
	}
	if err := l.accrueFinancials(ctx, tx, qtx, req, billing); err != nil {
		return false, err
	}
	if err := qtx.UpsertUsageRollupHourly(ctx, buildUsageRollupParams(req, billing)); err != nil {
		return false, fmt.Errorf("complete usage rollup: %w", err)
	}
	if err := l.reconcileAsyncTaskCharge(ctx, tx, req, billing); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit usage completion: %w", err)
	}
	return true, nil
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
// The usage row is therefore the idempotency anchor for quota, subscription
// quota, and the enqueued balance charge.
//
// Subscription quota is debited here because it is a counter on a row this
// request already owns. The balance charge is only enqueued: applying it needs
// the account row, and holding that lock inside the request transaction is what
// made every request for one tenant settle in single file.
func (l *UsageLogger) accrueFinancials(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	req *serving.Request,
	billing domain.BillingResult,
) error {
	subject := req.RuntimeSubject()
	if subject == nil || subject.TenantID == "" {
		return nil
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
				return fmt.Errorf("complete subscription billing: admitted usage is not meterable")
			}
			subMicro = 0
		}
		if subMicro > 0 {
			if _, err := q.DebitSubscription(ctx, dbgen.DebitSubscriptionParams{
				ID:             mustParseUUID(req.SubscriptionID),
				Win5hUsedMicro: subMicro,
			}); err != nil {
				return fmt.Errorf("complete subscription billing: %w", err)
			}
		}
		userMicro = 0
	}
	if tenantMicro == 0 && userMicro == 0 {
		return nil
	}
	if err := outbox.Enqueue(ctx, tx, outbox.Charge{
		RequestID:   req.RequestID,
		TenantID:    subject.TenantID,
		UserID:      subject.UserID,
		TenantMicro: tenantMicro,
		UserMicro:   userMicro,
		Description: "AI 请求额度扣费",
	}); err != nil {
		return fmt.Errorf("enqueue balance charge: %w", err)
	}
	return nil
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

// usageBillingSource coalesces an empty gate decision to "payg" so the usage
// log never violates the billing_source CHECK constraint.
func usageBillingSource(src string) string {
	if src == "" {
		return "payg"
	}
	return src
}

// billingStatus is the settlement state at insert time. A billable request is
// written "pending" and the outbox consumer promotes it to "confirmed" once the
// balance has actually moved, so the column now reports what really happened
// instead of asserting a charge that had not been applied yet.
func billingStatus(req *serving.Request) string {
	if req.BillingResult.TenantPayableMicro == 0 && req.BillingResult.UserChargedMicro == 0 {
		return string(domain.BillingFree)
	}
	return string(domain.BillingPending)
}
