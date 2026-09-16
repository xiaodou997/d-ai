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
	RequestID  string
	TenantName string
	UserName   string
	Group      string
	APIKeyName string
	Kind       string
	Model      string
	Source     string
	From       *time.Time
	To         *time.Time
	Cursor     string
	Limit      int
}
type RecordError struct {
	Origin  string `json:"origin"`
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type RecordTokens struct {
	Total      *int64 `json:"total"`
	Protocol   string `json:"protocol,omitempty"`
	Input      *int64 `json:"input"`
	Output     *int64 `json:"output"`
	CacheRead  *int64 `json:"cache_read"`
	CacheWrite *int64 `json:"cache_write"`
	Reasoning  *int64 `json:"reasoning"`
}
type RecordCharge struct {
	RefundReason     string     `json:"refund_reason,omitempty"`
	RefundOperator   string     `json:"refund_operator,omitempty"`
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

// RecordProfile contains display facts safe for the scoped caller.
type RecordProfile struct {
	TenantName          string   `json:"tenant_name,omitempty"`
	UserName            string   `json:"user_name,omitempty"`
	APIKeyID            string   `json:"api_key_id,omitempty"`
	APIKeyName          string   `json:"api_key_name,omitempty"`
	APIKeyLastFour      string   `json:"api_key_last_four,omitempty"`
	KeyOwnerType        string   `json:"key_owner_type,omitempty"`
	AuthMethod          string   `json:"auth_method,omitempty"`
	GroupID             string   `json:"group_id,omitempty"`
	GroupName           string   `json:"group_name_snapshot,omitempty"`
	RequestedModel      string   `json:"requested_model,omitempty"`
	ReasoningEffort     string   `json:"reasoning_effort,omitempty"`
	Resolution          string   `json:"resolution,omitempty"`
	Capability          string   `json:"capability_type,omitempty"`
	GroupMultiplier     *float64 `json:"group_default_user_multiplier_snapshot,omitempty"`
	UserOverride        *float64 `json:"user_multiplier_override_snapshot,omitempty"`
	EffectiveMultiplier *float64 `json:"effective_user_multiplier_snapshot,omitempty"`
}

// Internal routing and diagnostics are omitted entirely for non-admin callers.
type RecordAdminContext struct {
	TraceID            string   `json:"trace_id,omitempty"`
	AccountID          string   `json:"upstream_account_id,omitempty"`
	AccountName        string   `json:"upstream_account_name,omitempty"`
	Provider           string   `json:"provider_code,omitempty"`
	UpstreamModel      string   `json:"upstream_model,omitempty"`
	LogicalModel       string   `json:"resolved_logical_model,omitempty"`
	ResponseModel      string   `json:"public_response_model,omitempty"`
	EndpointID         string   `json:"endpoint_id,omitempty"`
	DispatchRule       string   `json:"matched_dispatch_rule_summary,omitempty"`
	ClientProtocol     string   `json:"client_protocol,omitempty"`
	ProviderFormat     string   `json:"provider_format,omitempty"`
	ProtocolConversion bool     `json:"protocol_conversion_enabled"`
	UserAgent          string   `json:"client_user_agent,omitempty"`
	ProviderTerminal   string   `json:"provider_terminal_state,omitempty"`
	CancellationOrigin string   `json:"cancellation_origin,omitempty"`
	TenantMultiplier   *float64 `json:"tenant_multiplier,omitempty"`
	SetupMs            *int     `json:"request_setup_ms,omitempty"`
	HeaderMs           *int     `json:"final_attempt_header_ms,omitempty"`
	FirstByteMs        *int     `json:"first_response_byte_ms,omitempty"`
	TailMs             *int     `json:"response_tail_ms,omitempty"`
}
type RequestRecord struct {
	Profile              RecordProfile       `json:"profile"`
	AdminContext         *RecordAdminContext `json:"admin_context,omitempty"`
	RequestID            string              `json:"request_id"`
	CreatedAt            time.Time           `json:"created_at"`
	TenantID             string              `json:"tenant_id,omitempty"`
	UserID               string              `json:"user_id,omitempty"`
	Model                string              `json:"model"`
	Source               string              `json:"source"`
	EndReason            string              `json:"end_reason"`
	Delivery             string              `json:"delivery"`
	HTTPStatus           *int                `json:"http_status"`
	Stream               bool                `json:"stream"`
	FirstTokenMs         *int                `json:"first_token_ms"`
	TotalMs              *int                `json:"total_ms"`
	MediaUnits           *float64            `json:"media_units,omitempty"`
	MediaUnitType        string              `json:"media_unit_type,omitempty"`
	Tokens               RecordTokens        `json:"tokens"`
	Charge               RecordCharge        `json:"charge"`
	Error                *RecordError        `json:"error,omitempty"`
	IsError              bool                `json:"is_error"`
	ExecutionAvailable   bool                `json:"execution_available"`
	DiagnosticsAvailable bool                `json:"diagnostics_available"`
	InternalDetail       string              `json:"internal_detail,omitempty"`
	Attempts             []json.RawMessage   `json:"attempts,omitempty"`
	Evidence             json.RawMessage     `json:"evidence,omitempty"`
	Pricing              json.RawMessage     `json:"pricing,omitempty"`
}
type RecordPage struct {
	Records    []RequestRecord `json:"records"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
type RecordSummary struct {
	CacheReadTokens   int64    `json:"cache_read_tokens"`
	CacheWriteTokens  int64    `json:"cache_write_tokens"`
	TotalTokens       int64    `json:"total_tokens"`
	TokenSamples      int64    `json:"token_samples"`
	AvgTotalMs        *float64 `json:"avg_total_ms"`
	AvgFirstTokenMs   *float64 `json:"avg_first_token_ms"`
	TimingSamples     int64    `json:"timing_samples"`
	FirstTokenSamples int64    `json:"first_token_samples"`
	Requests          int64    `json:"requests"`
	Errors            int64    `json:"errors"`
	Interruptions     int64    `json:"interruptions"`
	InputTokens       int64    `json:"input_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
	TenantCharged     *int64   `json:"tenant_charged_micro,omitempty"`
	UserCharged       int64    `json:"user_charged_micro"`
	TenantRefunded    *int64   `json:"tenant_refunded_micro,omitempty"`
	UserRefunded      int64    `json:"user_refunded_micro"`
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
