package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/billingledger"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
)

const (
	usageRecoveryStream           = billingledger.UsageRecoveryStream
	usageRecoveryQuarantineStream = billingledger.UsageRecoveryQuarantineStream
)

var ErrUsageCompletionQueued = errors.New("usage completion queued for recovery")

type usageRecoveryPayload struct {
	Version                int                                 `json:"version"`
	RequestID              string                              `json:"request_id"`
	LeaseID                string                              `json:"lease_id,omitempty"`
	BillingAdmissionActive bool                                `json:"billing_admission_active"`
	Usage                  dbgen.CreateUsageLogParams          `json:"usage"`
	Rollup                 dbgen.UpsertUsageRollupHourlyParams `json:"rollup"`
	HasAPIKey              bool                                `json:"has_api_key"`
	APIKeyID               pgtype.UUID                         `json:"api_key_id"`
	APIKeyQuotaMicro       int64                               `json:"api_key_quota_micro"`
	OwnerType              string                              `json:"owner_type"`
	TenantID               string                              `json:"tenant_id"`
	UserID                 string                              `json:"user_id,omitempty"`
	TenantMicro            int64                               `json:"tenant_micro"`
	UserMicro              int64                               `json:"user_micro"`
	SubscriptionID         pgtype.UUID                         `json:"subscription_id"`
	SubscriptionDebitMicro int64                               `json:"subscription_debit_micro"`
}

func buildUsageRecoveryPayload(req *serving.Request, billing domain.BillingResult) (usageRecoveryPayload, error) {
	subject := req.RuntimeSubject()
	if subject == nil {
		return usageRecoveryPayload{}, errors.New("usage recovery subject is required")
	}
	payload := usageRecoveryPayload{
		Version:                2,
		RequestID:              req.RequestID,
		LeaseID:                req.BillingLeaseID,
		BillingAdmissionActive: req.BillingAdmissionActive,
		Usage:                  buildUsageLogParams(req, billing),
		Rollup:                 buildUsageRollupParams(req, billing),
		OwnerType:              string(runtimeSubjectOwnerType(subject)),
		TenantID:               subject.TenantID,
		UserID:                 subject.UserID,
		TenantMicro:            billing.TenantPayableMicro,
		UserMicro:              billing.UserChargedMicro,
		APIKeyQuotaMicro:       billing.APIKeyQuotaCostMicro,
	}
	if subject.AuthMethod == coreidentity.AuthMethodAPIKey && subject.APIKeyID != "" {
		payload.HasAPIKey = true
		payload.APIKeyID = mustParseUUID(subject.APIKeyID)
	}
	if runtimeSubjectOwnerType(subject) == domain.OwnerTenant {
		payload.UserID = ""
		payload.UserMicro = 0
	}
	if req.BillingSource == subscription.BillingSourceSubscription &&
		runtimeSubjectOwnerType(subject) == domain.OwnerUser {
		debitMicro, ok := serving.SubscriptionDebitMicro(req)
		if !ok && billing.RetailBaseMicro > 0 {
			return usageRecoveryPayload{}, errors.New("subscription usage is not meterable")
		}
		payload.SubscriptionID = mustParseUUID(req.SubscriptionID)
		payload.SubscriptionDebitMicro = debitMicro
		payload.UserMicro = 0
	}
	return payload, nil
}

func (l *UsageLogger) enqueueRecovery(ctx context.Context, req *serving.Request, billing domain.BillingResult) error {
	if l.recoveryRedis == nil {
		return errors.New("usage recovery queue is unavailable")
	}
	payload, err := buildUsageRecoveryPayload(req, billing)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal usage recovery payload: %w", err)
	}
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if _, err := l.recoveryRedis.XAdd(recoveryCtx, newUsageRecoveryXAddArgs(string(raw))).Result(); err != nil {
		return fmt.Errorf("append usage recovery stream: %w", err)
	}
	return nil
}

func newUsageRecoveryXAddArgs(payload string) *redis.XAddArgs {
	return &redis.XAddArgs{
		Stream: usageRecoveryStream,
		Values: map[string]any{"payload": payload},
	}
}

// RunRecovery continuously replays durable completion envelopes. PostgreSQL's
// unique request_id remains the idempotency anchor, so multiple service
// instances may safely observe the same stream entry.
func (l *UsageLogger) RunRecovery(ctx context.Context) {
	if l == nil || l.recoveryRedis == nil {
		return
	}
	for ctx.Err() == nil {
		streams, err := l.recoveryRedis.XRead(ctx, &redis.XReadArgs{
			Streams: []string{usageRecoveryStream, "0-0"},
			Count:   32,
			Block:   5 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			l.logger.Warn("read usage recovery stream failed", zap.Error(err))
			waitRecoveryRetry(ctx)
			continue
		}
		hadFailure := false
		for _, stream := range streams {
			for _, message := range stream.Messages {
				raw, ok := message.Values["payload"].(string)
				if !ok {
					if err := l.quarantineRecoveryEntry(ctx, message, "payload field is missing or not a string"); err != nil {
						hadFailure = true
						l.logger.Error("quarantine malformed usage recovery entry failed",
							zap.String("stream_id", message.ID), zap.Error(err))
						continue
					}
					l.logger.Error("quarantined malformed usage recovery entry", zap.String("stream_id", message.ID))
					continue
				}
				var payload usageRecoveryPayload
				if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload.Version != 2 || payload.RequestID == "" {
					reason := fmt.Sprintf("incompatible payload: version=%d request_id_present=%t decode_error=%v",
						payload.Version, payload.RequestID != "", err)
					if quarantineErr := l.quarantineRecoveryEntry(ctx, message, reason); quarantineErr != nil {
						hadFailure = true
						l.logger.Error("quarantine incompatible usage recovery payload failed",
							zap.String("stream_id", message.ID), zap.Error(quarantineErr))
						continue
					}
					l.logger.Error("quarantined incompatible usage recovery payload",
						zap.String("stream_id", message.ID), zap.Int("version", payload.Version), zap.Error(err))
					continue
				}
				completed, accrued, err := l.completeRecoveryPayload(ctx, payload)
				if err != nil {
					hadFailure = true
					l.logger.Warn("usage recovery replay failed", zap.String("request_id", payload.RequestID), zap.Error(err))
					continue
				}
				if completed {
					if l.apiKeyInvalidator != nil && payload.HasAPIKey {
						_ = l.apiKeyInvalidator.DelByID(ctx, recoveryUUIDString(payload.APIKeyID))
					}
				}
				if accrued && l.billingCoordinator != nil {
					l.billingCoordinator.Trigger()
				}
				_ = l.recoveryRedis.XDel(ctx, usageRecoveryStream, message.ID).Err()
			}
		}
		if hadFailure {
			waitRecoveryRetry(ctx)
		}
	}
}

func (l *UsageLogger) quarantineRecoveryEntry(ctx context.Context, message redis.XMessage, reason string) error {
	values, err := json.Marshal(message.Values)
	if err != nil {
		return fmt.Errorf("marshal recovery quarantine envelope: %w", err)
	}
	pipe := l.recoveryRedis.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: usageRecoveryQuarantineStream,
		Values: map[string]any{
			"source_stream":  usageRecoveryStream,
			"source_id":      message.ID,
			"reason":         reason,
			"values":         string(values),
			"quarantined_at": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	pipe.XDel(ctx, usageRecoveryStream, message.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("move recovery entry to quarantine: %w", err)
	}
	return nil
}

func (l *UsageLogger) completeRecoveryPayload(ctx context.Context, payload usageRecoveryPayload) (bool, bool, error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback(ctx)
	q := l.q.WithTx(tx)
	if _, err := q.CreateUsageLog(ctx, payload.Usage); errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}
	if payload.HasAPIKey {
		rows, err := q.ConfirmAPIKeyQuotaUsage(ctx, dbgen.ConfirmAPIKeyQuotaUsageParams{
			ID: payload.APIKeyID, QuotaUsed: payload.APIKeyQuotaMicro,
		})
		if err != nil || rows != 1 {
			return false, false, fmt.Errorf("recover api key quota: rows=%d: %w", rows, err)
		}
	}
	if payload.SubscriptionDebitMicro > 0 {
		if _, err := q.DebitSubscription(ctx, dbgen.DebitSubscriptionParams{
			ID: payload.SubscriptionID, Win5hUsedMicro: payload.SubscriptionDebitMicro,
		}); err != nil {
			return false, false, fmt.Errorf("recover subscription debit: %w", err)
		}
	}
	accrued := payload.BillingAdmissionActive
	if payload.BillingAdmissionActive {
		if l.billingCoordinator == nil {
			return false, false, errors.New("recover billing ledger: coordinator unavailable")
		}
		if err := l.billingCoordinator.Complete(ctx, tx, billingledger.Completion{
			RequestID:       payload.RequestID,
			ExpectedLeaseID: payload.LeaseID,
			TenantMicro:     payload.TenantMicro,
			UserMicro:       payload.UserMicro,
		}); err != nil {
			return false, false, fmt.Errorf("recover billing ledger: %w", err)
		}
	} else if payload.TenantMicro > 0 || payload.UserMicro > 0 {
		return false, false, errors.New("recover billing ledger: positive usage has no durable admission")
	}
	if err := q.UpsertUsageRollupHourly(ctx, payload.Rollup); err != nil {
		return false, false, fmt.Errorf("recover usage rollup: %w", err)
	}
	charge := payload.TenantMicro
	if payload.OwnerType == string(domain.OwnerUser) {
		charge = payload.UserMicro
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_async_tasks
		SET caller_charge = GREATEST(caller_charge, $2)
		WHERE request_id = $1
	`, payload.RequestID, charge); err != nil {
		return false, false, fmt.Errorf("recover async task charge: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, false, err
	}
	return true, accrued, nil
}

func waitRecoveryRetry(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func recoveryUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
