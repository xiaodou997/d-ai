package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

// RequestStore is the execution/financial boundary. It never changes balances
// on the relay path and optional debugging cannot prevent financial sealing.
type RequestStore struct {
	financialPool *pgxpool.Pool
	mu            sync.Mutex
	cancel        context.CancelFunc
	done          chan struct{}
	pool          *pgxpool.Pool
	biller        usageBiller
	logger        *zap.Logger
	invalidator   apiKeyCacheInvalidator
}

func NewRequestStore(pool *pgxpool.Pool, biller usageBiller, logger *zap.Logger) *RequestStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RequestStore{pool: pool, financialPool: pool, biller: biller, logger: logger}
}
func (s *RequestStore) WithAPIKeyCacheInvalidator(i apiKeyCacheInvalidator) *RequestStore {
	s.invalidator = i
	return s
}
func (s *RequestStore) Name() string                               { return "request_registration" }
func (s *RequestStore) Rollback(context.Context, *serving.Request) {}
func (s *RequestStore) Execute(ctx context.Context, req *serving.Request) error {
	var healthy bool
	err := s.pool.QueryRow(ctx, `SELECT accepting AND NOT EXISTS(SELECT 1 FROM bill_settlements WHERE state='pending' AND sealed_at<now()-interval '30 seconds') AND NOT EXISTS(SELECT 1 FROM bill_settlements WHERE state='review') FROM bill_record_control WHERE singleton`).Scan(&healthy)
	if err != nil || !healthy {
		return &serving.APIError{Status: 503, Code: "settlement_unavailable", Message: "billing is temporarily unavailable"}
	}
	if err = s.register(ctx, req, true); err != nil {
		req.RecordRegistrationRejected = true
		return &serving.APIError{Status: 503, Code: "request_registration_failed", Message: "request storage is temporarily unavailable"}
	}
	tenant, user, key, _ := requestOwners(req)
	_ = user
	_ = s.pool.QueryRow(ctx, `SELECT id::text FROM ai_debug_sessions WHERE expires_at>now() AND (tenant_id='' OR tenant_id=$1) AND (api_key_id='' OR api_key_id=$2) AND (model_code='' OR model_code=$3) ORDER BY created_at DESC LIMIT 1`, tenant, key, req.ModelCode).Scan(&req.RecordDebugSessionID)
	req.CaptureBody = req.RecordDebugSessionID != ""
	leaseCtx, stop := context.WithCancelCause(ctx)
	req.RecordLeaseContext = leaseCtx
	req.RecordLeaseStop = func() { stop(nil) }
	id := req.RequestID
	go func() {
		timer := time.NewTicker(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-timer.C:
				heartbeat, cancel := context.WithTimeout(leaseCtx, 5*time.Second)
				_, err := s.pool.Exec(heartbeat, `UPDATE ai_request_keys SET lease_until=now()+interval '3 minutes' WHERE request_id=$1 AND sealed_at IS NULL`, id)
				cancel()
				if err != nil {
					s.logger.Warn("request lease heartbeat", zap.Error(err))
					stop(fmt.Errorf("request record lease unavailable: %w", err))
					return
				}
			}
		}
	}()
	return nil
}
func requestOwners(req *serving.Request) (tenant, user, key, source string) {
	if subject := req.RuntimeSubject(); subject != nil {
		tenant = subject.TenantID
		user = subject.UserID
		key = subject.APIKeyID
	}
	if subject := req.RuntimeSubject(); subject != nil {
		source = string(subject.RequestSource)
	}
	if source == "" {
		source = "api_key"
	}
	return
}
func (s *RequestStore) register(ctx context.Context, req *serving.Request, fresh bool) error {
	if req.RequestID == "" {
		return errors.New("request identity is missing")
	}
	tenant, user, key, source := requestOwners(req)
	if req.StartedAt.IsZero() {
		req.StartedAt = time.Now()
	}
	origin := req.BillingOriginAt
	if origin.IsZero() {
		origin = req.StartedAt
	}
	pricing, err := json.Marshal(req.BillingSnapshots)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var at time.Time
	err = tx.QueryRow(ctx, `INSERT INTO ai_request_keys(request_id,epoch,lease_until,tenant_id,user_id)
 SELECT $1,epoch,now()+interval '3 minutes',$2,$3 FROM bill_record_control WHERE singleton AND $4::timestamptz>=closed_before
 ON CONFLICT(request_id) DO UPDATE SET request_id=EXCLUDED.request_id
 WHERE NOT $5 AND ai_request_keys.tenant_id=EXCLUDED.tenant_id AND ai_request_keys.user_id=EXCLUDED.user_id
 RETURNING registered_at`, req.RequestID, tenant, user, origin, fresh).Scan(&at)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO ai_requests(created_at,request_id,tenant_id,user_id,api_key_id,model_code,request_source,facts)
 VALUES($1,$2,$3,$4,$5,$6,$7,jsonb_build_object('pricing', $8::jsonb)) ON CONFLICT DO NOTHING`, at, req.RequestID, tenant, user, key, req.ModelCode, source, pricing)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Log seals a request and its charge decision in one transaction. Re-entry may
// update delivery facts, but never replace sealed evidence or enqueue a charge.
func (s *RequestStore) Log(ctx context.Context, req *serving.Request) error {
	if req.RecordRegistrationRejected {
		return nil
	}
	if req.RecordLeaseStop != nil {
		defer req.RecordLeaseStop()
	}
	serving.EnsureSettlementState(req)
	if len(req.Attempts) == 0 {
		req.UsageEvidence = domain.UsageEvidence{}
		req.TokenUsage = domain.TokenUsage{}
		req.MediaUsageConfirmed = false
	}
	serving.ApplyReportedUsage(req)
	if err := s.register(ctx, req, false); err != nil {
		return err
	}
	decision := serving.DecideCompletion(req)
	var bill domain.BillingResult
	var err error
	if req.Candidate != nil && len(req.Attempts) > 0 && s.biller != nil {
		bill, err = s.biller.Calculate(ctx, req)
		if err != nil {
			return err
		}
	}
	req.BillingResult = bill
	tenant, user, key, source := requestOwners(req)
	meta := map[string]any{"model_code": req.ModelCode, "requested_model": req.RequestedModel, "request_source": source,
		"http_status": req.HTTPStatus, "upstream_status": req.UpstreamStatus, "provider_terminal_state": req.ProviderTerminalState,
		"client_delivery_state": req.ClientDeliveryState, "cancellation_origin": req.CancellationOrigin,
		"first_token_latency_ms": req.FirstTokenMs, "stream": req.IsStream, "billing_source": req.BillingSource}
	if req.Candidate != nil {
		p := buildUsageLogParams(req, bill)
		raw, e := json.Marshal(p)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &meta); e != nil {
			return e
		}
	}
	for _, k := range []string{"request_status", "error_message", "error_code", "billing_breakdown", "billing_status", "settlement_error"} {
		delete(meta, k)
	}
	facts, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	mediaUnits := float64(req.TokenUsage.ImageCount)
	if req.CapabilityType == domain.CapabilityVideo {
		mediaUnits = req.TokenUsage.VideoSeconds
	}
	evidence, err := json.Marshal(map[string]any{"usage": req.UsageEvidence, "media_confirmed": req.MediaUsageConfirmed, "media_units": mediaUnits, "media_unit_type": bill.BillableUnitType, "end_reason": decision.EndReason, "reference_cost_micro": bill.CatalogBaseMicro})
	if err != nil {
		return err
	}
	var selectedPricing any
	tenantMultiplier := 0.0
	if req.Candidate != nil {
		selectedPricing = req.BillingSnapshots[req.Candidate.RouteID]
		tenantMultiplier = req.Candidate.TenantMultiplier
	}
	pricing, err := json.Marshal(map[string]any{"snapshot": selectedPricing, "tenant_multiplier": tenantMultiplier, "subscription_group_multipliers": req.SubscriptionGroupQuotaDebitMultipliers, "calculation": json.RawMessage(bill.BillingBreakdownJSON)})
	if err != nil {
		return err
	}
	state := "pending"
	tenantDue, userDue, keyDue, subDue := bill.TenantPayableMicro, bill.UserChargedMicro, bill.APIKeyQuotaCostMicro, int64(0)
	if user == "" {
		userDue = 0
	}
	if key == "" {
		keyDue = 0
	}
	if req.BillingSource == "subscription" {
		var metered bool
		subDue, metered = serving.SubscriptionDebitMicro(req)
		if decision.Billable && bill.RetailBaseMicro > 0 && (!metered || req.SubscriptionID == "") {
			return errors.New("subscription settlement evidence is missing")
		}
		userDue = 0
	}
	if !decision.Billable {
		state = "waived"
		tenantDue = 0
		userDue = 0
		keyDue = 0
		subDue = 0
	}
	if state == "pending" && tenantDue == 0 && userDue == 0 && keyDue == 0 && subDue == 0 {
		state = "waived"
		decision.ChargeReason = "zero_charge"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var at time.Time
	var sealed *time.Time
	err = tx.QueryRow(ctx, `SELECT registered_at,sealed_at FROM ai_request_keys WHERE request_id=$1 FOR UPDATE`, req.RequestID).Scan(&at, &sealed)
	if err != nil {
		return err
	}
	interrupted := req.CancellationOrigin == domain.CancellationClient || req.ClientDeliveryState == domain.ClientDeliveryDisconnected || req.ClientDeliveryState == domain.ClientDeliveryWriteFailed
	if sealed != nil {
		_, err = tx.Exec(ctx, `UPDATE ai_requests SET delivery_state=$3,end_reason=CASE WHEN NOT is_error AND $3 IN ('disconnected','write_failed') THEN 'client_interrupted' ELSE end_reason END WHERE created_at=$1 AND request_id=$2`, at, req.RequestID, string(req.ClientDeliveryState))
		if err != nil {
			return err
		}
		if interrupted {
			tag, e := tx.Exec(ctx, `UPDATE bill_settlements SET delivery_interrupted=true WHERE created_at=$1 AND request_id=$2 AND NOT delivery_interrupted`, at, req.RequestID)
			if e != nil {
				return e
			}
			if tag.RowsAffected() > 0 {
				if _, e = tx.Exec(ctx, `UPDATE bill_daily_totals SET interruptions=interruptions+1 WHERE day=($1::timestamptz AT TIME ZONE 'UTC')::date AND tenant_id=$2 AND user_id=$3`, at, tenant, user); e != nil {
					return e
				}
			}
		}
		return tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `UPDATE ai_requests SET end_reason=$3,delivery_state=$4,is_error=$5,completed_at=now(),facts=$6 WHERE created_at=$1 AND request_id=$2`, at, req.RequestID, decision.EndReason, string(req.ClientDeliveryState), decision.Error != nil, facts)
	if err != nil {
		return err
	}
	if f := decision.Error; f != nil {
		_, err = tx.Exec(ctx, `INSERT INTO ai_request_errors(created_at,request_id,origin,stage,code,message,internal_detail) VALUES($1,$2,$3,$4,$5,$6,$7)`, at, req.RequestID, f.Origin, f.Stage, f.Code, serving.RedactCredentialText(f.Message), serving.RedactInternalErrorDetail(req.InternalErrorDetail))
		if err != nil {
			return err
		}
	}
	for i, attempt := range req.Attempts {
		attempt.ErrorMsg = serving.RedactInternalErrorDetail(attempt.ErrorMsg)
		raw, e := json.Marshal(attempt)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ai_request_attempts(created_at,request_id,ordinal,facts) VALUES($1,$2,$3,$4)`, at, req.RequestID, i+1, raw); e != nil {
			return e
		}
	}
	billingSource := req.BillingSource
	if billingSource == "" {
		billingSource = "payg"
	}
	_, err = tx.Exec(ctx, `INSERT INTO bill_settlements(created_at,request_id,tenant_id,user_id,api_key_id,subscription_id,billing_source,state,reason,tenant_due,user_due,key_due,subscription_due,evidence,pricing,dimensions,delivery_interrupted)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, at, req.RequestID, tenant, user, key, req.SubscriptionID, billingSource, state, decision.ChargeReason, tenantDue, userDue, keyDue, subDue, evidence, pricing, facts, interrupted)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE ai_request_keys SET sealed_at=now() WHERE request_id=$1`, req.RequestID)
	if err != nil {
		return err
	}
	ecount, icount := 0, 0
	if decision.Error != nil {
		ecount = 1
	}
	if interrupted {
		icount = 1
	}
	_, err = tx.Exec(ctx, `INSERT INTO bill_daily_totals(day,tenant_id,user_id,requests,errors,interruptions,prompt_tokens,completion_tokens)
 VALUES(($1::timestamptz AT TIME ZONE 'UTC')::date,$2,$3,1,$4,$5,$6,$7)
 ON CONFLICT(day,tenant_id,user_id) DO UPDATE SET requests=bill_daily_totals.requests+1,errors=bill_daily_totals.errors+EXCLUDED.errors,interruptions=bill_daily_totals.interruptions+EXCLUDED.interruptions,prompt_tokens=bill_daily_totals.prompt_tokens+EXCLUDED.prompt_tokens,completion_tokens=bill_daily_totals.completion_tokens+EXCLUDED.completion_tokens`, at, tenant, user, ecount, icount, req.TokenUsage.PromptTokens, req.TokenUsage.CompletionTokens)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("seal request: %w", err)
	}
	if state == "waived" {
		req.BillingStatus = domain.BillingVoid
	} else {
		req.BillingStatus = domain.BillingPending
	}
	req.BillingReason = decision.ChargeReason
	return nil
}

var _ serving.Step = (*RequestStore)(nil)
var _ serving.UsageLogger = (*RequestStore)(nil)
var _ = pgx.ErrNoRows

func (s *RequestStore) WithFinancialPool(pool *pgxpool.Pool) *RequestStore {
	if pool != nil {
		s.financialPool = pool
	}
	return s
}

func (s *RequestStore) invalidateAPIKeyCache(ctx context.Context, subject *coreidentity.Subject) {
	if s == nil || s.invalidator == nil || subject == nil || subject.APIKeyID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	completion, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.invalidator.DelByID(completion, subject.APIKeyID); err != nil {
		s.logger.Warn("invalidate settled key", zap.Error(err))
	}
}
