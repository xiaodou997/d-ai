package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type RecordScope struct {
	TenantID string
	UserID   string
	Admin    bool
	EndUser  bool
}
type RecordQuery struct {
	RecordScope
	Kind   string
	Model  string
	Source string
	From   *time.Time
	To     *time.Time
	Cursor string
	Limit  int
}
type RecordError struct {
	Origin  string `json:"origin"`
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type RecordTokens struct {
	Input      *int64 `json:"input"`
	Output     *int64 `json:"output"`
	CacheRead  *int64 `json:"cache_read"`
	CacheWrite *int64 `json:"cache_write"`
	Reasoning  *int64 `json:"reasoning"`
}
type RecordCharge struct {
	ProcessingDetail string     `json:"processing_detail,omitempty"`
	State            string     `json:"state"`
	Reason           string     `json:"reason"`
	Source           string     `json:"source"`
	TenantDue        *int64     `json:"tenant_due_micro,omitempty"`
	TenantCharged    *int64     `json:"tenant_charged_micro,omitempty"`
	UserDue          int64      `json:"user_due_micro"`
	UserCharged      int64      `json:"user_charged_micro"`
	APIKeyUsed       int64      `json:"api_key_used_micro"`
	SubscriptionUsed int64      `json:"subscription_used_micro"`
	ReferenceCost    *int64     `json:"reference_cost_micro,omitempty"`
	RefundedAt       *time.Time `json:"refunded_at,omitempty"`
	PostedAt         *time.Time `json:"posted_at,omitempty"`
}
type RequestRecord struct {
	RequestID            string            `json:"request_id"`
	CreatedAt            time.Time         `json:"created_at"`
	TenantID             string            `json:"tenant_id,omitempty"`
	UserID               string            `json:"user_id,omitempty"`
	Model                string            `json:"model"`
	Source               string            `json:"source"`
	EndReason            string            `json:"end_reason"`
	Delivery             string            `json:"delivery"`
	HTTPStatus           *int              `json:"http_status"`
	Stream               bool              `json:"stream"`
	FirstTokenMs         *int              `json:"first_token_ms"`
	TotalMs              *int              `json:"total_ms"`
	MediaUnits           *float64          `json:"media_units,omitempty"`
	MediaUnitType        string            `json:"media_unit_type,omitempty"`
	Tokens               RecordTokens      `json:"tokens"`
	Charge               RecordCharge      `json:"charge"`
	Error                *RecordError      `json:"error,omitempty"`
	IsError              bool              `json:"is_error"`
	ExecutionAvailable   bool              `json:"execution_available"`
	DiagnosticsAvailable bool              `json:"diagnostics_available"`
	InternalDetail       string            `json:"internal_detail,omitempty"`
	Attempts             []json.RawMessage `json:"attempts,omitempty"`
	Evidence             json.RawMessage   `json:"evidence,omitempty"`
	Pricing              json.RawMessage   `json:"pricing,omitempty"`
}
type RecordPage struct {
	Records    []RequestRecord `json:"records"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
type RecordSummary struct {
	Requests       int64  `json:"requests"`
	Errors         int64  `json:"errors"`
	Interruptions  int64  `json:"interruptions"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
	TenantCharged  *int64 `json:"tenant_charged_micro,omitempty"`
	UserCharged    int64  `json:"user_charged_micro"`
	TenantRefunded *int64 `json:"tenant_refunded_micro,omitempty"`
	UserRefunded   int64  `json:"user_refunded_micro"`
}
type RequestRecordRepository interface {
	Records(context.Context, RecordQuery) (RecordPage, error)
	Record(context.Context, RecordScope, string) (RequestRecord, error)
	RecordSummary(context.Context, RecordQuery) (RecordSummary, error)
	Refund(context.Context, string, string, string) error
}

type DebugSessionInput struct {
	TenantID string `json:"tenant_id"`
	APIKeyID string `json:"api_key_id"`
	Model    string `json:"model"`
	Hours    int    `json:"hours" minimum:"1" maximum:"24" default:"1"`
}
type DebugSession struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}
type DebugRecordsRepository interface {
	StartDebug(context.Context, DebugSessionInput, string) (DebugSession, error)
	DebugPayload(context.Context, string) (json.RawMessage, error)
}

var ErrInvalidRecordCursor = errors.New("invalid record cursor")
