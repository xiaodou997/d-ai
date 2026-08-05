package clientruntime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

// Invoker is the serving-facing interface of the fixed-provider client runtime.
// It owns the final provider request contract and the upstream HTTP exchange,
// while serving retains routing, retry budgets, protocol relay, billing, and
// audit responsibilities.
type Invoker interface {
	SupportsInvocation(provider domain.FixedProviderType, protocol domain.UpstreamProtocol) bool
	Invoke(ctx context.Context, in Invocation) (*Exchange, error)
}

type Inspector interface {
	SupportsInspection(provider domain.FixedProviderType, want InspectionWant) bool
	Inspect(ctx context.Context, in Inspection) (InspectionSnapshot, error)
}

type OperationKind string

const (
	OperationResponses       OperationKind = "responses"
	OperationCompact         OperationKind = "compact"
	OperationMessages        OperationKind = "messages"
	OperationGenerateContent OperationKind = "generate_content"
)

// Invocation is already converted to the provider's semantic protocol. The
// runtime applies only the selected official-client profile.
type Invocation struct {
	Provider    domain.FixedProviderType
	Operation   OperationKind
	Protocol    domain.UpstreamProtocol
	Model       string
	Body        []byte
	ContentType string
	Stream      bool
	RequestID   string
	AffinityKey string
	// IncomingAnthropicBeta is semantic client intent. The Claude profile
	// merges it with the required official-client beta set.
	IncomingAnthropicBeta string
	Credential            Credential
}

// Credential is a runtime snapshot. It intentionally has no JSON tags so it
// cannot accidentally be serialized into logs or an HTTP response.
type Credential struct {
	ID           string
	PoolID       string
	AccessToken  string
	RefreshToken string
	AccountID    string
	ExpiresAt    *time.Time
	Metadata     map[string]any
	TokenVersion int64
}

func SnapshotCredential(credential *domain.OAuthCredential) Credential {
	if credential == nil {
		return Credential{}
	}
	accountID := credential.AccountID()
	if accountID == "" {
		for _, key := range []string{"chatgpt_account_id", "accountId"} {
			if value, ok := credential.AuthMetadata[key].(string); ok && strings.TrimSpace(value) != "" {
				accountID = strings.TrimSpace(value)
				break
			}
		}
	}
	return Credential{
		ID:           credential.ID,
		PoolID:       credential.PoolID,
		AccessToken:  credential.AccessToken,
		RefreshToken: credential.RefreshToken,
		AccountID:    accountID,
		ExpiresAt:    credential.ExpiresAt,
		Metadata:     credential.AuthMetadata,
		TokenVersion: credential.TokenVersion,
	}
}

// CredentialRefresher refreshes and reloads one credential. Production uses a
// token-store adapter; tests use an in-memory adapter.
type CredentialRefresher interface {
	Refresh(ctx context.Context, credentialID string) (Credential, error)
}

type WireRequest struct {
	Method   string
	URL      string
	Headers  map[string]string
	Body     []byte
	Protocol domain.UpstreamProtocol
}

type WireResponse struct {
	StatusCode int
	Headers    http.Header
	Body       io.ReadCloser
}

// Transport is the true-external provider seam. Production and tests provide
// separate adapters.
type Transport interface {
	Do(ctx context.Context, req *WireRequest) (*WireResponse, error)
}

type CredentialEffect string

const (
	CredentialEffectNone          CredentialEffect = "none"
	CredentialEffectRefreshed     CredentialEffect = "refreshed"
	CredentialEffectRefreshFailed CredentialEffect = "refresh_failed"
	CredentialEffectCooldown      CredentialEffect = "cooldown"
	CredentialEffectInvalidate    CredentialEffect = "invalidate"
)

type WireAttempt struct {
	StatusCode int
	StartedAt  time.Time
	FinishedAt time.Time
}

type Trace struct {
	ProfileRevision  string
	RequestURL       string
	CredentialID     string
	ProviderCalls    int
	RefreshCalls     int
	CredentialEffect CredentialEffect
	CooldownUntil    time.Time
	WireAttempts     []WireAttempt
}

type Exchange struct {
	Response *WireResponse
	Trace    Trace
}

type InspectionWant uint8

const (
	InspectModels InspectionWant = 1 << iota
	InspectQuota
	InspectAccountState
)

type Inspection struct {
	Provider    domain.FixedProviderType
	Credential  Credential
	Want        InspectionWant
	IfNoneMatch string
}

type ModelCard struct {
	ID           string
	Capabilities map[string]any
}

type InspectionSnapshot struct {
	ProfileRevision string
	Models          []ModelCard
	ETag            string
	NotModified     bool
	Source          string
	ObservedAt      time.Time
}

type ErrorCode string

const (
	ErrorRuntimeNotConfigured ErrorCode = "runtime_not_configured"
	ErrorUnsupportedProvider  ErrorCode = "unsupported_provider"
	ErrorInvalidInvocation    ErrorCode = "invalid_invocation"
	ErrorRequestContract      ErrorCode = "request_contract"
	ErrorTransport            ErrorCode = "transport"
)

type Error struct {
	Code            ErrorCode
	ProfileRevision string
	SafeDetail      string
	Cause           error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.SafeDetail != "" {
		return e.SafeDetail
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func IsTransportError(err error) bool {
	var runtimeErr *Error
	return errors.As(err, &runtimeErr) && runtimeErr.Code == ErrorTransport
}
