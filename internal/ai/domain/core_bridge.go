package domain

import (
	"encoding/json"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/billing"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/upstream"
)

// ToCore converts a legacy capability to the rebuilt catalog capability.
func (c CapabilityType) ToCore() catalog.Capability {
	switch c {
	case CapabilityChat:
		return catalog.CapabilityChat
	case CapabilityEmbedding:
		return catalog.CapabilityEmbedding
	case CapabilityAudioTTS:
		return catalog.CapabilityAudioTTS
	case CapabilityAudioSTT:
		return catalog.CapabilityAudioSTT
	case CapabilityVideo:
		return catalog.CapabilityVideoGeneration
	case CapabilityRerank:
		return catalog.CapabilityWorkflow
	case CapabilityImage:
		return catalog.CapabilityImageGeneration
	default:
		return catalog.CapabilityChat
	}
}

// ToCore converts a legacy owner type to the rebuilt identity scope.
func (o OwnerType) ToCore() identity.Scope {
	switch o {
	case OwnerTenant:
		return identity.ScopeTenant
	case OwnerUser:
		return identity.ScopeUser
	default:
		return identity.ScopeTenant
	}
}

// ToCore converts a management API key to the rebuilt identity model.
func (k APIKey) ToCore() identity.APIKey {
	return identity.APIKey{
		ID:              k.ID,
		OwnerScope:      k.OwnerType.ToCore(),
		TenantID:        k.TenantID,
		UserID:          k.UserID,
		GroupID:         k.GroupID,
		LastFour:        k.LastFour,
		Name:            k.Name,
		QuotaLimitMicro: k.QuotaLimitMicro,
		QuotaUsedMicro:  k.QuotaUsedMicro,
		AllowedModelIDs: append([]string(nil), k.AllowedModels...),
		Status:          k.Status,
		ExpiresAt:       k.ExpiresAt,
		LastUsedAt:      k.LastUsedAt,
		CreatedBy:       k.CreatedBy,
		CreatedAt:       k.CreatedAt,
		UpdatedAt:       k.UpdatedAt,
	}
}

// ToCore converts a legacy upstream account to the rebuilt upstream resource.
func (a UpstreamAccount) ToCore() upstream.Upstream {
	mode := upstream.AccessModeDirect
	family := upstream.ProviderFamilyOpenAICompatible
	switch EndpointProtocol(a.DefaultProtocol) {
	case EndpointProtocolAnthropic:
		family = upstream.ProviderFamilyAnthropic
	case EndpointProtocolGemini:
		family = upstream.ProviderFamilyGoogle
	case EndpointProtocolOpenAICompatible:
		family = upstream.ProviderFamilyOpenAICompatible
	default:
		family = upstream.ProviderFamilyOther
	}
	return upstream.Upstream{
		ID:             a.ID,
		Code:           a.Name,
		Name:           a.Name,
		ProviderFamily: family,
		AccessMode:     mode,
		BaseURL:        a.BaseURL,
		Status:         upstream.Status(a.Status),
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

// ToCore converts a legacy price book to the rebuilt billing price book.
func (b PriceBook) ToCore() billing.PriceBook {
	return billing.PriceBook{
		ID:          b.ID,
		Code:        b.Name,
		Name:        b.Name,
		Description: b.Description,
		Status:      billing.Status(b.Status),
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
	}
}

// ToCore converts a legacy price book entry to the rebuilt billing entry.
func (e PriceBookEntry) ToCore() billing.PriceBookEntry {
	imagePrices, _ := json.Marshal(e.ImagePrices)
	videoPrices, _ := json.Marshal(e.VideoPrices)
	return billing.PriceBookEntry{
		ID:                e.ID,
		PriceBookID:       e.PriceBookID,
		ModelCode:         e.ModelCode,
		Capability:        CapabilityType(e.CapabilityType).ToCore(),
		TokenPriceTiers:   append([]billing.TokenPriceTier(nil), e.TokenPriceTiers...),
		ImageDefaultPrice: e.ImageDefaultPrice,
		VideoDefaultPrice: e.VideoDefaultPrice,
		ImagePricesJSON:   imagePrices,
		VideoPricesJSON:   videoPrices,
		AudioTTSPerChar:   e.AudioTTSPerChar,
		AudioSTTPerMinute: e.AudioSTTPerMinute,
		Source:            e.Source,
		ManuallyEdited:    e.ManuallyEdited,
		CreatedAt:         e.CreatedAt,
		UpdatedAt:         e.UpdatedAt,
	}
}

// ToCore converts a legacy limit policy to the rebuilt commercial policy.
func (p RuntimeLimitPolicy) ToCore() commercial.LimitPolicy {
	var conc *int
	if p.ConcurrencyLimit != nil {
		v := int(*p.ConcurrencyLimit)
		conc = &v
	}
	return commercial.LimitPolicy{
		ID:               p.ID,
		ScopeType:        commercial.LimitScope(p.ScopeType),
		ScopeID:          p.ScopeID,
		ConcurrencyLimit: conc,
		Status:           commercial.Status(p.Status),
		CreatedBy:        p.CreatedBy,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}
