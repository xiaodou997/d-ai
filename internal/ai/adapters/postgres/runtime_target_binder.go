package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/runtimecompat"
	"xiaodou/dai/internal/ai/secret"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/clientsecret"
)

// RuntimeTargetBinder bridges rebuilt runtime planning to the current
// upstream/account/pool backing storage. Runtime binding selection now resolves
// directly from ai_upstream_models explicit bindings; account/pool projection
// fields remain only as compatibility metadata on the backing records and are
// no longer consulted by the binder itself.
type RuntimeTargetBinder struct {
	q             *dbgen.Queries
	pool          *translatingPool
	poolStore     *OAuthCredentialStore
	bridgeSupport corebridge.SupportMatrix
	masterKey     string
}

func NewRuntimeTargetBinder(q *dbgen.Queries, pool *pgxpool.Pool, masterKey string) *RuntimeTargetBinder {
	return &RuntimeTargetBinder{
		q:             q,
		pool:          newTranslatingPool(pool),
		poolStore:     NewOAuthCredentialStore(pool, masterKey),
		bridgeSupport: normalizeBridgeSupport(nil),
		masterKey:     masterKey,
	}
}

func (b *RuntimeTargetBinder) WithBridgeSupport(support corebridge.SupportMatrix) *RuntimeTargetBinder {
	b.bridgeSupport = normalizeBridgeSupport(support)
	return b
}

var _ coreupstream.RuntimeBindingResolver = (*RuntimeTargetBinder)(nil)

func (b *RuntimeTargetBinder) ResolveRuntimeBinding(
	ctx context.Context,
	req coreupstream.RuntimeBindingRequest,
) (coreupstream.RuntimeBinding, error) {
	switch req.TargetMode {
	case coreupstream.AccessModeDirect:
		return b.bindDirectUpstream(ctx, req)
	case coreupstream.AccessModeOAuthPool:
		return b.bindOAuthPool(ctx, req)
	default:
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, fmt.Sprintf("unsupported target mode %q", req.TargetMode))
	}
}

func (b *RuntimeTargetBinder) bindDirectUpstream(
	ctx context.Context,
	req coreupstream.RuntimeBindingRequest,
) (coreupstream.RuntimeBinding, error) {
	accountID, err := akUUID(req.TargetID)
	if err != nil {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionTargetNotFound, "invalid upstream target id")
	}
	row, err := b.q.GetUpstreamAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionTargetNotFound, "upstream account not found")
		}
		return coreupstream.RuntimeBinding{}, err
	}
	if row.Status != string(coreupstream.StatusActive) && row.Status != "active" {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionTargetInactive, "upstream account is not active")
	}

	clientProtocol, err := runtimecompat.SurfaceToProtocolForCapability(req.ClientSurface, req.Capability)
	if err != nil {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, err.Error())
	}
	support := normalizeBridgeSupport(b.bridgeSupport)
	binding, err := loadUpstreamModelBinding(ctx, b.pool, upstreamKindDirect, uuidToString(row.ID), req.ResolvedModelID, runtimecompat.CapabilityFromCore(req.Capability))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionModelBindingMissing, fmt.Sprintf("no active binding for model %q", req.ResolvedModelID))
		}
		return coreupstream.RuntimeBinding{}, err
	}
	selectedProtocol, conversionBucket, ok := chooseProviderProtocolWithSupport(support, runtimecompat.CapabilityFromCore(req.Capability), clientProtocol, []domain.UpstreamProtocol{binding.APIFormat}, req.AllowProtocolConversion, req.Stream)
	if !ok {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionProtocolIncompatible, "no compatible protocol")
	}
	providerFamily := upstreamProviderFamilyFromProtocol(selectedProtocol)
	providerSurface, err := runtimecompat.ProtocolToSurfaceForCapability(selectedProtocol, req.Capability)
	if err != nil {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, err.Error())
	}
	requestSurface := providerSurface
	if binding.APIFormat != "" {
		if requestSurface, err = runtimecompat.ProtocolToSurfaceForCapability(binding.APIFormat, req.Capability); err != nil {
			return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, err.Error())
		}
	}
	responseSurface := requestSurface
	bridgeRequired := support.NeedsBridge(req.ClientSurface, providerSurface)
	if bridgeRequired && !bridgeSurfaceSupportedForCapability(support, req.ClientSurface, providerSurface, req.Capability, req.Stream) {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionProtocolIncompatible, "unsupported bridge surface pair")
	}

	upstreamModelName := binding.UpstreamModelName
	if upstreamModelName == "" {
		upstreamModelName = req.ResolvedModelID
	}
	resource := directUpstreamToCore(row)
	resource.ProviderFamily = providerFamily
	apiKey, err := decryptDirectProviderKey(b.masterKey, row.ApiKeyCiphertext)
	if err != nil {
		return coreupstream.RuntimeBinding{}, credentialRejection(err)
	}
	if clientsecret.IsConfigured() && clientsecret.NeedsReencrypt(row.ApiKeyCiphertext) {
		if encrypted, err := secret.EncryptProviderKey(b.masterKey, apiKey); err == nil {
			_, _ = b.pool.Exec(ctx, `
				UPDATE ai_upstream_accounts SET api_key_ciphertext = $1, updated_at = now() WHERE id = $2
			`, encrypted, row.ID)
		}
	}
	defaultMultiplier := 1.0
	if row.TenantMultiplier.Valid {
		defaultMultiplier = numericToFloat(row.TenantMultiplier)
	}
	costMultiplier, err := b.resolveTenantMultiplier(
		ctx, req.TenantID, upstreamaccess.KindDirectUpstream, uuidToString(row.ID), defaultMultiplier,
	)
	if err != nil {
		return coreupstream.RuntimeBinding{}, err
	}
	costPer1k, err := loadRuntimeCostPer1k(ctx, b.pool, uuidToString(row.PriceBookID), req.ResolvedModelID, string(req.Capability), costMultiplier)
	if err != nil {
		return coreupstream.RuntimeBinding{}, err
	}
	return coreupstream.RuntimeBinding{
		Upstream: resource,
		ModelBinding: coreupstream.ModelBinding{
			UpstreamKind:      coreupstream.AccessModeDirect,
			UpstreamID:        uuidToString(row.ID),
			ModelID:           req.ResolvedModelID,
			Capability:        req.Capability,
			RequestSurface:    requestSurface,
			ResponseSurface:   responseSurface,
			UpstreamModelName: upstreamModelName,
			Priority:          req.Priority,
			Status:            coreupstream.StatusActive,
			Config:            binding.Config,
		},
		ConversionBucket: conversionBucket,
		APIKeyCiphertext: apiKey,
		ExtraHeaders:     unmarshalStringMap(row.ExtraHeaders),
		CostPriceBookID:  uuidToString(row.PriceBookID),
		TenantMultiplier: costMultiplier,
		CostPer1kTokens:  costPer1k,
	}, nil
}

// credentialRejection turns a credential failure into a routing rejection that
// keeps its cause.
//
// A master key that cannot decrypt an account's credential and an account whose
// credential is simply missing both silently remove the target from routing,
// and a bare "provider credential is unavailable" cannot tell an operator which
// one happened — that ambiguity is exactly what makes a mismatched
// DAI_SECURITY_SECRET_MASTER_KEY present as an unexplained "no available route".
//
// The detail is the AES-GCM failure text; it carries no key material, and it
// only ever reaches server logs and the admin dispatch preview, never a
// runtime client.
func credentialRejection(err error) error {
	return coreupstream.NewRuntimeBindingRejection(
		coreupstream.BindingRejectionCredentialUnavailable,
		fmt.Sprintf("decrypt provider credential: %v", err))
}

func decryptDirectProviderKey(masterKey, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	return secret.DecryptProviderKey(masterKey, ciphertext)
}

func (b *RuntimeTargetBinder) bindOAuthPool(
	ctx context.Context,
	req coreupstream.RuntimeBindingRequest,
) (coreupstream.RuntimeBinding, error) {
	pool, err := b.poolStore.GetPool(ctx, req.TargetID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionTargetNotFound, "oauth pool not found")
		}
		return coreupstream.RuntimeBinding{}, err
	}
	if pool.Status != "active" {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionTargetInactive, "oauth pool is not active")
	}

	clientProtocol, err := runtimecompat.SurfaceToProtocolForCapability(req.ClientSurface, req.Capability)
	if err != nil {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, err.Error())
	}
	support := normalizeBridgeSupport(b.bridgeSupport)
	binding, err := loadUpstreamModelBinding(ctx, b.pool, upstreamKindPool, pool.ID, req.ResolvedModelID, runtimecompat.CapabilityFromCore(req.Capability))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionModelBindingMissing, fmt.Sprintf("no active binding for model %q", req.ResolvedModelID))
		}
		return coreupstream.RuntimeBinding{}, err
	}
	selectedProtocol, conversionBucket, ok := chooseProviderProtocolWithSupport(support, runtimecompat.CapabilityFromCore(req.Capability), clientProtocol, []domain.UpstreamProtocol{binding.APIFormat}, req.AllowProtocolConversion, req.Stream)
	if !ok {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionProtocolIncompatible, "no compatible protocol")
	}
	providerFamily := upstreamProviderFamilyFromProtocol(selectedProtocol)
	providerSurface, err := runtimecompat.ProtocolToSurfaceForCapability(selectedProtocol, req.Capability)
	if err != nil {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, err.Error())
	}
	requestSurface := providerSurface
	if binding.APIFormat != "" {
		if requestSurface, err = runtimecompat.ProtocolToSurfaceForCapability(binding.APIFormat, req.Capability); err != nil {
			return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionBindingInvalid, err.Error())
		}
	}
	responseSurface := requestSurface
	bridgeRequired := support.NeedsBridge(req.ClientSurface, providerSurface)
	if bridgeRequired && !bridgeSurfaceSupportedForCapability(support, req.ClientSurface, providerSurface, req.Capability, req.Stream) {
		return coreupstream.RuntimeBinding{}, coreupstream.NewRuntimeBindingRejection(coreupstream.BindingRejectionProtocolIncompatible, "unsupported bridge surface pair")
	}

	upstreamModelName := binding.UpstreamModelName
	if upstreamModelName == "" {
		upstreamModelName = req.ResolvedModelID
	}
	resource := credentialPoolToCore(pool)
	resource.ProviderFamily = providerFamily
	costMultiplier, err := b.resolveTenantMultiplier(
		ctx, req.TenantID, upstreamaccess.KindOAuthPool, pool.ID, pool.TenantMultiplier,
	)
	if err != nil {
		return coreupstream.RuntimeBinding{}, err
	}
	costPer1k, err := loadRuntimeCostPer1k(ctx, b.pool, pool.PriceBookID, req.ResolvedModelID, string(req.Capability), costMultiplier)
	if err != nil {
		return coreupstream.RuntimeBinding{}, err
	}
	return coreupstream.RuntimeBinding{
		Upstream: resource,
		ModelBinding: coreupstream.ModelBinding{
			UpstreamKind:      coreupstream.AccessModeOAuthPool,
			UpstreamID:        pool.ID,
			ModelID:           req.ResolvedModelID,
			Capability:        req.Capability,
			RequestSurface:    requestSurface,
			ResponseSurface:   responseSurface,
			UpstreamModelName: upstreamModelName,
			Priority:          req.Priority,
			Status:            coreupstream.StatusActive,
			Config:            binding.Config,
		},
		ConversionBucket:  conversionBucket,
		FixedProviderType: coreupstream.FixedProviderType(pool.FixedProviderType),
		SelectionStrategy: coreupstream.SelectionStrategy(pool.OAuthStrategy),
		CostPriceBookID:   pool.PriceBookID,
		TenantMultiplier:  costMultiplier,
		CostPer1kTokens:   costPer1k,
	}, nil
}

func loadRuntimeCostPer1k(ctx context.Context, pool dbgen.DBTX, priceBookID, modelCode, capability string, multiplier float64) (float64, error) {
	if priceBookID == "" || modelCode == "" {
		return 0, nil
	}
	if multiplier < 0 {
		multiplier = 1
	}
	var cost float64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE((
		  ((token_price_tiers->0->>'input_per_token')::numeric
		   + (token_price_tiers->0->>'output_per_token')::numeric)
		  * 1000 * $4::numeric
		)::float8, 0)
		FROM ai_price_book_entries
		WHERE price_book_id = $1 AND model_code = $2 AND capability_type = $3`, priceBookID, modelCode, capability, multiplier).Scan(&cost)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load runtime route cost: %w", err)
	}
	return cost, nil
}

func (b *RuntimeTargetBinder) resolveTenantMultiplier(
	ctx context.Context,
	tenantID, resourceKind, resourceID string,
	defaultMultiplier float64,
) (float64, error) {
	if tenantID == "" {
		return defaultMultiplier, nil
	}
	var override pgtype.Numeric
	err := b.pool.QueryRow(ctx, `
		SELECT tenant_multiplier_override
		FROM ai_upstream_resource_tenant_policies
		WHERE tenant_id = $1 AND resource_kind = $2 AND resource_id = $3::uuid
	`, tenantID, resourceKind, resourceID).Scan(&override)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultMultiplier, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve tenant upstream multiplier: %w", err)
	}
	if !override.Valid {
		return defaultMultiplier, nil
	}
	return numericToFloat(override), nil
}

func credentialPoolToCore(pool *domain.CredentialPool) coreupstream.Upstream {
	if pool == nil {
		return coreupstream.Upstream{}
	}
	return coreupstream.Upstream{
		ID:             pool.ID,
		Code:           pool.Name,
		Name:           pool.Name,
		ProviderFamily: upstreamProviderFamilyFromFixedProvider(pool.FixedProviderType),
		AccessMode:     coreupstream.AccessModeOAuthPool,
		BaseURL:        domain.FixedProviderBaseURL(pool.FixedProviderType),
		Notes:          pool.Notes,
		Status:         coreupstream.Status(pool.Status),
		CreatedAt:      pool.CreatedAt,
		UpdatedAt:      pool.UpdatedAt,
	}
}

func directUpstreamToCore(row dbgen.AiUpstreamAccount) coreupstream.Upstream {
	return coreupstream.Upstream{
		ID:               uuidToString(row.ID),
		Code:             row.Name,
		Name:             row.Name,
		ProviderFamily:   coreupstream.ProviderFamilyOther,
		AccessMode:       coreupstream.AccessModeDirect,
		BaseURL:          row.BaseUrl,
		ConcurrencyLimit: int32PtrToIntPtr(akInt4StrPtr(row.ConcurrencyLimit)),
		Status:           coreupstream.Status(row.Status),
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func upstreamProviderFamilyFromFixedProvider(provider domain.FixedProviderType) coreupstream.ProviderFamily {
	switch provider {
	case domain.FixedProviderClaudeOAuth:
		return coreupstream.ProviderFamilyAnthropic
	case domain.FixedProviderGeminiCLI, domain.FixedProviderAntigravity:
		return coreupstream.ProviderFamilyGoogle
	case domain.FixedProviderCodex:
		return coreupstream.ProviderFamilyOpenAICompatible
	default:
		return coreupstream.ProviderFamilyOther
	}
}

func upstreamProviderFamilyFromProtocol(protocol domain.UpstreamProtocol) coreupstream.ProviderFamily {
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		return coreupstream.ProviderFamilyAnthropic
	case domain.ProtocolGeminiGenerate, domain.ProtocolGeminiEmbeddings:
		return coreupstream.ProviderFamilyGoogle
	case domain.ProtocolOpenAIChat,
		domain.ProtocolOpenAIResponses,
		domain.ProtocolOpenAIEmbeddings,
		domain.ProtocolOpenAIImages:
		return coreupstream.ProviderFamilyOpenAICompatible
	default:
		return coreupstream.ProviderFamilyOther
	}
}

func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
