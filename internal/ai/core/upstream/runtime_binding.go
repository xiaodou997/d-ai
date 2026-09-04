package upstream

import (
	"context"
	"errors"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
)

var ErrNoRuntimeBinding = errors.New("no runtime binding")

type BindingRejectionCode string

const (
	BindingRejectionAccessDenied          BindingRejectionCode = "access_denied"
	BindingRejectionTargetNotFound        BindingRejectionCode = "target_not_found"
	BindingRejectionTargetInactive        BindingRejectionCode = "target_inactive"
	BindingRejectionModelBindingMissing   BindingRejectionCode = "model_binding_missing"
	BindingRejectionProtocolIncompatible  BindingRejectionCode = "protocol_incompatible"
	BindingRejectionCredentialUnavailable BindingRejectionCode = "credential_unavailable"
	BindingRejectionBindingInvalid        BindingRejectionCode = "binding_invalid"
	BindingRejectionUnavailable           BindingRejectionCode = "binding_unavailable"
)

type RuntimeBindingRejection struct {
	Code   BindingRejectionCode
	Detail string
}

func (e *RuntimeBindingRejection) Error() string {
	if e == nil || e.Detail == "" {
		return ErrNoRuntimeBinding.Error()
	}
	return ErrNoRuntimeBinding.Error() + ": " + e.Detail
}

func (e *RuntimeBindingRejection) Unwrap() error {
	return ErrNoRuntimeBinding
}

func NewRuntimeBindingRejection(code BindingRejectionCode, detail string) error {
	return &RuntimeBindingRejection{Code: code, Detail: detail}
}

func RuntimeBindingRejectionFromError(err error) (RuntimeBindingRejection, bool) {
	var rejection *RuntimeBindingRejection
	if !errors.As(err, &rejection) || rejection == nil {
		return RuntimeBindingRejection{}, false
	}
	return *rejection, true
}

// RuntimeBindingRequest is the runtime-kernel side input for selecting one
// concrete upstream target + model binding from a commercial dispatch target.
type RuntimeBindingRequest struct {
	TenantID                string
	Capability              catalog.Capability
	ClientSurface           surface.ID
	RequestedModel          string
	ResolvedModelID         string
	Stream                  bool
	AllowProtocolConversion bool
	TargetMode              AccessMode
	TargetID                string
}

// RuntimeBinding is the upstream-layer result returned to the runtime kernel
// before actual bridge execution starts.
type RuntimeBinding struct {
	Upstream           Upstream
	ModelBinding       ModelBinding
	EndpointID         string
	RequestPath        string
	EndpointAuthScheme string
	EndpointAuthHeader string
	ConversionBucket   int
	APIKeyCiphertext   string
	ExtraHeaders       map[string]string
	FixedProviderType  FixedProviderType
	SelectionStrategy  SelectionStrategy
	CostPriceBookID    string
	TenantMultiplier   float64
	CostPer1kTokens    float64
}

// RuntimeBindingResolver resolves one commercial target into a concrete
// upstream resource + model binding pair.
type RuntimeBindingResolver interface {
	ResolveRuntimeBinding(ctx context.Context, req RuntimeBindingRequest) (RuntimeBinding, error)
}
