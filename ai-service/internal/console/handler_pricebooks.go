package console

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"xiaodou/unihub/ai-service/internal/domain"
)

// perMillion scales between per-token storage and per-1M display. Price book
// columns store USD per token (LiteLLM-native); the API exposes USD per 1M
// tokens (and per 1M chars for TTS) so admins type human-sized numbers.
const perMillion = 1_000_000.0

// ============================================================================
// DTOs
// ============================================================================

type priceBookDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   *int64 `json:"created_at"`
	UpdatedAt   *int64 `json:"updated_at"`
}

type resolutionUSDDTO struct {
	Resolution string  `json:"resolution"`
	Price      float64 `json:"price"` // USD per image / per second
}

// priceBookEntryDTO exposes all prices in USD. Token/char fields are per 1M;
// audio_stt and resolution tiers are absolute per-unit USD.
type priceBookEntryDTO struct {
	ModelCode          string             `json:"model_code"`
	CapabilityType     string             `json:"capability_type"`
	InputPer1MUSD      float64            `json:"input_per_1m_usd"`
	OutputPer1MUSD     float64            `json:"output_per_1m_usd"`
	CacheWritePer1MUSD float64            `json:"cache_write_per_1m_usd"`
	CacheReadPer1MUSD  float64            `json:"cache_read_per_1m_usd"`
	ReasoningPer1MUSD  float64            `json:"reasoning_per_1m_usd"`
	ImagePrices        []resolutionUSDDTO `json:"image_prices"`
	VideoPrices        []resolutionUSDDTO `json:"video_prices"`
	AudioTTSPer1MChars float64            `json:"audio_tts_per_1m_chars_usd"`
	AudioSTTPerMinute  float64            `json:"audio_stt_per_minute_usd"`
	Source             string             `json:"source"`
	ManuallyEdited     bool               `json:"manually_edited"`
	UpdatedAt          *int64             `json:"updated_at"`
}

type tenantSellBindingDTO struct {
	TenantID            string  `json:"tenant_id"`
	PriceBookID         string  `json:"price_book_id"`
	PriceBookName       string  `json:"price_book_name,omitempty"`
	SellMultiplier      float64 `json:"sell_multiplier"`
	CacheBillingEnabled bool    `json:"cache_billing_enabled"`
	UpdatedAt           *int64  `json:"updated_at"`
}

type userSellBindingDTO struct {
	TenantID            string  `json:"tenant_id"`
	UserMultiplier      float64 `json:"user_multiplier"`
	CacheBillingEnabled bool    `json:"cache_billing_enabled"`
	UpdatedAt           *int64  `json:"updated_at"`
}

// ============================================================================
// Mappers
// ============================================================================

func priceBookToDTO(b domain.PriceBook) priceBookDTO {
	return priceBookDTO{
		ID:          b.ID,
		Name:        b.Name,
		Description: b.Description,
		Status:      b.Status,
		CreatedAt:   timeToMillisPtr(b.CreatedAt),
		UpdatedAt:   timeToMillisPtr(b.UpdatedAt),
	}
}

func resolutionsToDTO(rs []domain.ResolutionUSDPrice) []resolutionUSDDTO {
	out := make([]resolutionUSDDTO, 0, len(rs))
	for _, r := range rs {
		out = append(out, resolutionUSDDTO{Resolution: r.Resolution, Price: r.Price})
	}
	return out
}

func resolutionsFromDTO(rs []resolutionUSDDTO) []domain.ResolutionUSDPrice {
	out := make([]domain.ResolutionUSDPrice, 0, len(rs))
	for _, r := range rs {
		out = append(out, domain.ResolutionUSDPrice{Resolution: strings.TrimSpace(r.Resolution), Price: r.Price})
	}
	return out
}

func priceBookEntryToDTO(e domain.PriceBookEntry) priceBookEntryDTO {
	return priceBookEntryDTO{
		ModelCode:          e.ModelCode,
		CapabilityType:     e.CapabilityType,
		InputPer1MUSD:      e.InputPerToken * perMillion,
		OutputPer1MUSD:     e.OutputPerToken * perMillion,
		CacheWritePer1MUSD: e.CacheWritePerToken * perMillion,
		CacheReadPer1MUSD:  e.CacheReadPerToken * perMillion,
		ReasoningPer1MUSD:  e.ReasoningPerToken * perMillion,
		ImagePrices:        resolutionsToDTO(e.ImagePrices),
		VideoPrices:        resolutionsToDTO(e.VideoPrices),
		AudioTTSPer1MChars: e.AudioTTSPerChar * perMillion,
		AudioSTTPerMinute:  e.AudioSTTPerMinute,
		Source:             e.Source,
		ManuallyEdited:     e.ManuallyEdited,
		UpdatedAt:          timeToMillisPtr(e.UpdatedAt),
	}
}

func tenantSellBindingToDTO(b domain.TenantSellBinding) tenantSellBindingDTO {
	return tenantSellBindingDTO{
		TenantID:            b.TenantID,
		PriceBookID:         b.PriceBookID,
		PriceBookName:       b.PriceBookName,
		SellMultiplier:      b.SellMultiplier,
		CacheBillingEnabled: b.CacheBillingEnabled,
		UpdatedAt:           timeToMillisPtr(b.UpdatedAt),
	}
}

func userSellBindingToDTO(b domain.UserSellBinding) userSellBindingDTO {
	return userSellBindingDTO{
		TenantID:            b.TenantID,
		UserMultiplier:      b.UserMultiplier,
		CacheBillingEnabled: b.CacheBillingEnabled,
		UpdatedAt:           timeToMillisPtr(b.UpdatedAt),
	}
}

// ============================================================================
// Price book CRUD
// ============================================================================

func (s *Console) handleAdminListPriceBooks(w http.ResponseWriter, r *http.Request) {
	books, err := s.priceBookSvc.ListPriceBooks(r.Context())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	out := make([]priceBookDTO, 0, len(books))
	for _, b := range books {
		out = append(out, priceBookToDTO(b))
	}
	writeOK(w, out)
}

type priceBookRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (s *Console) handleAdminCreatePriceBook(w http.ResponseWriter, r *http.Request) {
	var req priceBookRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	b, err := s.priceBookSvc.CreatePriceBook(r.Context(), req.Name, req.Description)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, priceBookToDTO(b))
}

func (s *Console) handleAdminGetPriceBook(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	b, err := s.priceBookSvc.GetPriceBook(r.Context(), chi.URLParam(r, "bookID"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, priceBookToDTO(b))
}

func (s *Console) handleAdminUpdatePriceBook(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	var req priceBookRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	b, err := s.priceBookSvc.UpdatePriceBook(r.Context(), chi.URLParam(r, "bookID"), req.Name, req.Description, req.Status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, priceBookToDTO(b))
}

func (s *Console) handleAdminDeletePriceBook(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	if err := s.priceBookSvc.DeletePriceBook(r.Context(), chi.URLParam(r, "bookID")); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

// ============================================================================
// Price book entries
// ============================================================================

func (s *Console) handleAdminListPriceBookEntries(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	entries, err := s.priceBookSvc.ListEntries(r.Context(), chi.URLParam(r, "bookID"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	out := make([]priceBookEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, priceBookEntryToDTO(e))
	}
	writeOK(w, out)
}

type upsertEntryRequest struct {
	ModelCode          string             `json:"model_code"`
	CapabilityType     string             `json:"capability_type"`
	InputPer1MUSD      float64            `json:"input_per_1m_usd"`
	OutputPer1MUSD     float64            `json:"output_per_1m_usd"`
	CacheWritePer1MUSD float64            `json:"cache_write_per_1m_usd"`
	CacheReadPer1MUSD  float64            `json:"cache_read_per_1m_usd"`
	ReasoningPer1MUSD  float64            `json:"reasoning_per_1m_usd"`
	ImagePrices        []resolutionUSDDTO `json:"image_prices"`
	VideoPrices        []resolutionUSDDTO `json:"video_prices"`
	AudioTTSPer1MChars float64            `json:"audio_tts_per_1m_chars_usd"`
	AudioSTTPerMinute  float64            `json:"audio_stt_per_minute_usd"`
}

func (s *Console) handleAdminUpsertPriceBookEntry(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	var req upsertEntryRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	capability := req.CapabilityType
	if capability == "" {
		capability = "chat"
	}
	entry := domain.PriceBookEntry{
		PriceBookID:        chi.URLParam(r, "bookID"),
		ModelCode:          strings.TrimSpace(req.ModelCode),
		CapabilityType:     capability,
		InputPerToken:      req.InputPer1MUSD / perMillion,
		OutputPerToken:     req.OutputPer1MUSD / perMillion,
		CacheWritePerToken: req.CacheWritePer1MUSD / perMillion,
		CacheReadPerToken:  req.CacheReadPer1MUSD / perMillion,
		ReasoningPerToken:  req.ReasoningPer1MUSD / perMillion,
		ImagePrices:        resolutionsFromDTO(req.ImagePrices),
		VideoPrices:        resolutionsFromDTO(req.VideoPrices),
		AudioTTSPerChar:    req.AudioTTSPer1MChars / perMillion,
		AudioSTTPerMinute:  req.AudioSTTPerMinute,
	}
	saved, err := s.priceBookSvc.UpsertEntry(r.Context(), entry)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, priceBookEntryToDTO(saved))
}

func (s *Console) handleAdminDeletePriceBookEntry(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	modelCode := strings.TrimSpace(r.URL.Query().Get("model_code"))
	if modelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model_code is required")
		return
	}
	if err := s.priceBookSvc.DeleteEntry(r.Context(), chi.URLParam(r, "bookID"), modelCode); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

// GET /price-books/litellm/models?q=&limit= — search the in-memory LiteLLM
// reference catalog for the entry autofill flow (never bulk-imported).
func (s *Console) handleAdminSearchLiteLLM(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	res, err := s.priceBookSvc.SearchLiteLLM(r.Context(), q, limit)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, res)
}

// POST /price-books/{bookID}/sync-common — 拉取一批常用模型的价格（白名单）。
func (s *Console) handleAdminSyncCommonModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	res, err := s.priceBookSvc.SyncCommonModels(r.Context(), chi.URLParam(r, "bookID"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, res)
}

func (s *Console) handleAdminImportLiteLLM(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "bookID"); !ok {
		return
	}
	res, err := s.priceBookSvc.ImportFromLiteLLM(r.Context(), chi.URLParam(r, "bookID"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, res)
}

// ============================================================================
// Global USD→credit rate
// ============================================================================

type creditsPerUSDDTO struct {
	CreditsPerUSD float64 `json:"credits_per_usd"`
}

func (s *Console) handleAdminGetCreditsPerUSD(w http.ResponseWriter, r *http.Request) {
	v, err := s.priceBookSvc.GetCreditsPerUSD(r.Context())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, creditsPerUSDDTO{CreditsPerUSD: v})
}

func (s *Console) handleAdminSetCreditsPerUSD(w http.ResponseWriter, r *http.Request) {
	var req creditsPerUSDDTO
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if err := s.priceBookSvc.SetCreditsPerUSD(r.Context(), req.CreditsPerUSD); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, req)
}

// ============================================================================
// Sell bindings
// ============================================================================

func (s *Console) handleAdminListTenantSellBindings(w http.ResponseWriter, r *http.Request) {
	bindings, err := s.priceBookSvc.ListTenantSellBindings(r.Context())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	out := make([]tenantSellBindingDTO, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, tenantSellBindingToDTO(b))
	}
	writeOK(w, out)
}

func (s *Console) handleAdminGetTenantSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
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

type upsertTenantSellBindingRequest struct {
	PriceBookID         string  `json:"price_book_id"`
	SellMultiplier      float64 `json:"sell_multiplier"`
	CacheBillingEnabled bool    `json:"cache_billing_enabled"`
}

func (s *Console) handleAdminUpsertTenantSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	var req upsertTenantSellBindingRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	b, err := s.priceBookSvc.UpsertTenantSellBinding(r.Context(), domain.TenantSellBinding{
		TenantID:            tenantID,
		PriceBookID:         req.PriceBookID,
		SellMultiplier:      req.SellMultiplier,
		CacheBillingEnabled: req.CacheBillingEnabled,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, tenantSellBindingToDTO(b))
}

func (s *Console) handleAdminDeleteTenantSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	if err := s.priceBookSvc.DeleteTenantSellBinding(r.Context(), tenantID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

func (s *Console) handleAdminGetUserSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
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

type upsertUserSellBindingRequest struct {
	UserMultiplier      float64 `json:"user_multiplier"`
	CacheBillingEnabled bool    `json:"cache_billing_enabled"`
}

func (s *Console) handleAdminUpsertUserSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
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

func (s *Console) handleAdminDeleteUserSellBinding(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return
	}
	if err := s.priceBookSvc.DeleteUserSellBinding(r.Context(), tenantID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}
