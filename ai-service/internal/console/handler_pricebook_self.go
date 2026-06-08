package console

import (
	"errors"
	"net/http"

	"xiaodou/unihub/ai-service/internal/domain"
)

// ============================================================================
// 租户/用户 自助定价（/tenants/me/*, /users/me/*）
// ============================================================================

// scopedTenantID resolves the tenant whose pricing the caller may act on:
// tenant role → own tenant; platform role → ?tenant_id=. Returns ok=false and
// writes the error response when unresolved.
func (s *Console) scopedTenantID(w http.ResponseWriter, r *http.Request) (string, bool) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return "", false
	}
	switch ac.Role {
	case apiRoleTenant:
		return ac.TenantID, true
	case apiRolePlatform:
		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenant_id required for platform")
			return "", false
		}
		return tenantID, true
	default:
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return "", false
	}
}

// GET /tenants/me/sell-binding — 租户查看平台给自己的售价绑定（只读）。
func (s *Console) handleTenantsMeSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := s.scopedTenantID(w, r)
	if !ok {
		return
	}
	b, err := s.priceBookSvc.GetTenantSellBinding(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeOK(w, nil)
			return
		}
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, tenantSellBindingToDTO(b))
}

// GET /tenants/me/user-sell-binding — 租户查看自己给用户的售价绑定。
func (s *Console) handleTenantsMeUserSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := s.scopedTenantID(w, r)
	if !ok {
		return
	}
	b, err := s.priceBookSvc.GetUserSellBinding(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeOK(w, nil)
			return
		}
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, userSellBindingToDTO(b))
}

// PUT /tenants/me/user-sell-binding — 租户设置自己给用户的售价（级联倍率+缓存开关）。
func (s *Console) handleTenantsMeUserSellBindingUpsert(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := s.scopedTenantID(w, r)
	if !ok {
		return
	}
	var req upsertUserSellBindingRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	b, err := s.priceBookSvc.UpsertUserSellBinding(r.Context(), domain.UserSellBinding{
		TenantID:            tenantID,
		UserMultiplier:      req.UserMultiplier,
		CacheBillingEnabled: req.CacheBillingEnabled,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, userSellBindingToDTO(b))
}

// DELETE /tenants/me/user-sell-binding
func (s *Console) handleTenantsMeUserSellBindingDelete(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := s.scopedTenantID(w, r)
	if !ok {
		return
	}
	if err := s.priceBookSvc.DeleteUserSellBinding(r.Context(), tenantID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

// GET /tenants/me/effective-prices?scope=tenant|user
// scope=tenant（默认）：平台给该租户的积分单价；scope=user：该租户给其用户的级联积分单价。
func (s *Console) handleTenantsMeEffectivePrices(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := s.scopedTenantID(w, r)
	if !ok {
		return
	}
	includeUser := r.URL.Query().Get("scope") == "user"
	s.writeEffectivePrices(w, r, tenantID, includeUser)
}

// GET /users/me/effective-prices — 终端用户查看自己被计费的积分单价（级联结果）。
func (s *Console) handleUsersMeEffectivePrices(w http.ResponseWriter, r *http.Request) {
	ac, ok := apiContextFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, BizErrMissingCtx, "missing context")
		return
	}
	var tenantID string
	switch ac.Role {
	case apiRoleUser:
		tenantID = ac.TenantID
	case apiRolePlatform:
		tenantID = r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenant_id required for platform")
			return
		}
	default:
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return
	}
	s.writeEffectivePrices(w, r, tenantID, true)
}

func (s *Console) writeEffectivePrices(w http.ResponseWriter, r *http.Request, tenantID string, includeUser bool) {
	prices, err := s.priceBookSvc.EffectivePrices(r.Context(), tenantID, includeUser)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeOK(w, []effectivePriceDTO{})
			return
		}
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, effectivePricesToDTO(prices))
}

// ---- DTOs ----

type effectiveResolutionDTO struct {
	Resolution string  `json:"resolution"`
	Price      float64 `json:"price"`
}

type effectivePriceDTO struct {
	ModelCode                 string                   `json:"model_code"`
	CapabilityType            string                   `json:"capability_type"`
	InputPer1MCredits         float64                  `json:"input_per_1m_credits"`
	OutputPer1MCredits        float64                  `json:"output_per_1m_credits"`
	CacheWritePer1MCredits    float64                  `json:"cache_write_per_1m_credits"`
	CacheReadPer1MCredits     float64                  `json:"cache_read_per_1m_credits"`
	ReasoningPer1MCredits     float64                  `json:"reasoning_per_1m_credits"`
	ImagePrices               []effectiveResolutionDTO `json:"image_prices"`
	VideoPrices               []effectiveResolutionDTO `json:"video_prices"`
	AudioTTSPer1MCharsCredits float64                  `json:"audio_tts_per_1m_chars_credits"`
	AudioSTTPerMinuteCredits  float64                  `json:"audio_stt_per_minute_credits"`
}

func effectivePricesToDTO(prices []domain.EffectiveModelPrice) []effectivePriceDTO {
	out := make([]effectivePriceDTO, 0, len(prices))
	for _, p := range prices {
		out = append(out, effectivePriceDTO{
			ModelCode:                 p.ModelCode,
			CapabilityType:            p.CapabilityType,
			InputPer1MCredits:         p.InputPer1MCredits,
			OutputPer1MCredits:        p.OutputPer1MCredits,
			CacheWritePer1MCredits:    p.CacheWritePer1MCredits,
			CacheReadPer1MCredits:     p.CacheReadPer1MCredits,
			ReasoningPer1MCredits:     p.ReasoningPer1MCredits,
			ImagePrices:               effectiveResolutionsToDTO(p.ImagePrices),
			VideoPrices:               effectiveResolutionsToDTO(p.VideoPrices),
			AudioTTSPer1MCharsCredits: p.AudioTTSPer1MCharsCredits,
			AudioSTTPerMinuteCredits:  p.AudioSTTPerMinuteCredits,
		})
	}
	return out
}

func effectiveResolutionsToDTO(rs []domain.ResolutionCreditPriceF) []effectiveResolutionDTO {
	out := make([]effectiveResolutionDTO, 0, len(rs))
	for _, r := range rs {
		out = append(out, effectiveResolutionDTO{Resolution: r.Resolution, Price: r.Price})
	}
	return out
}
