// Package audit records immutable evidence for operator-initiated money repairs.
//
// Every event is written in the same transaction as the balance/state change.
// The database enforces unique repair and idempotency keys and rejects UPDATE
// or DELETE on the audit table, so a successful repair always leaves a durable
// before/after record.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"xiaodou/dai/internal/billing/ledger"
)

var ErrInvalidEvent = errors.New("invalid billing repair audit event")

// Event is one immutable financial repair record.
type Event struct {
	RepairID       string
	Action         string
	IdempotencyKey string
	TargetType     string
	TargetID       string
	OperatorID     string
	Reason         string
	BeforeState    json.RawMessage
	AfterState     json.RawMessage
}

// NewRepairID returns an opaque, non-sequential audit identifier.
func NewRepairID() string { return "REPAIR_" + uuid.NewString()[:24] }

// Snapshot encodes a JSON object suitable for before/after evidence.
func Snapshot(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal repair snapshot: %w", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return nil, fmt.Errorf("%w: snapshot must be a JSON object", ErrInvalidEvent)
	}
	return json.RawMessage(raw), nil
}

// Append writes one event. It deliberately does not use ON CONFLICT: a
// duplicate idempotency key is an explicit signal that the caller attempted a
// second repair, and the surrounding transaction must not silently proceed.
func Append(ctx context.Context, tx ledger.Execer, event Event) error {
	if strings.TrimSpace(event.RepairID) == "" ||
		strings.TrimSpace(event.Action) == "" ||
		strings.TrimSpace(event.IdempotencyKey) == "" ||
		strings.TrimSpace(event.TargetType) == "" ||
		strings.TrimSpace(event.TargetID) == "" ||
		strings.TrimSpace(event.OperatorID) == "" ||
		strings.TrimSpace(event.Reason) == "" {
		return fmt.Errorf("%w: required field is empty", ErrInvalidEvent)
	}
	if !isJSONObject(event.BeforeState) || !isJSONObject(event.AfterState) {
		return fmt.Errorf("%w: before_state and after_state must be JSON objects", ErrInvalidEvent)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bill_repair_audits
			(repair_id, action, idempotency_key, target_type, target_id,
			 operator_id, reason, before_state, after_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb)
	`, event.RepairID, event.Action, event.IdempotencyKey, event.TargetType, event.TargetID,
		event.OperatorID, event.Reason, []byte(event.BeforeState), []byte(event.AfterState)); err != nil {
		return fmt.Errorf("append billing repair audit: %w", err)
	}
	return nil
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(raw)
}
