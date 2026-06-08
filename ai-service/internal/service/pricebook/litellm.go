package pricebook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"xiaodou/unihub/ai-service/internal/domain"
)

// llmCacheTTL bounds how long the in-memory LiteLLM map is reused before
// re-fetching. The data only matters for the admin search/autofill flow, so a
// long TTL is fine.
const llmCacheTTL = 6 * time.Hour

// DefaultLiteLLMURL is the canonical LiteLLM price source.
const DefaultLiteLLMURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// LiteLLMModel is the subset of one LiteLLM price entry we consume. All cost
// fields are USD per token (LiteLLM's native unit), matching our entry columns.
type LiteLLMModel struct {
	InputCostPerToken           float64 `json:"input_cost_per_token"`
	OutputCostPerToken          float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost     float64 `json:"cache_read_input_token_cost"`
	Mode                        string  `json:"mode"`
}

// LiteLLMFetcher fetches the LiteLLM price map keyed by model name.
type LiteLLMFetcher interface {
	Fetch(ctx context.Context) (map[string]LiteLLMModel, error)
}

// HTTPFetcher fetches the price map over HTTP.
type HTTPFetcher struct {
	URL    string
	client *http.Client
}

// NewHTTPFetcher builds a fetcher. Empty url falls back to DefaultLiteLLMURL.
func NewHTTPFetcher(url string) *HTTPFetcher {
	if url == "" {
		url = DefaultLiteLLMURL
	}
	return &HTTPFetcher{URL: url, client: &http.Client{Timeout: 30 * time.Second}}
}

func (f *HTTPFetcher) Fetch(ctx context.Context) (map[string]LiteLLMModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch litellm prices: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch litellm prices: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB guard
	if err != nil {
		return nil, fmt.Errorf("read litellm prices: %w", err)
	}
	var raw map[string]LiteLLMModel
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse litellm prices: %w", err)
	}
	return raw, nil
}

// ImportResult summarizes one LiteLLM import run.
type ImportResult struct {
	Fetched  int `json:"fetched"`  // total models in the source
	Imported int `json:"imported"` // entries inserted or refreshed (manual rows skipped)
	Skipped  int `json:"skipped"`  // source models we did not import (unsupported mode / no price)
}

// ImportFromLiteLLM fetches the LiteLLM source and fills the target price book,
// inserting new entries and refreshing only non-manually-edited rows. Models
// with an unsupported mode or no usable price are skipped.
func (s *Service) ImportFromLiteLLM(ctx context.Context, priceBookID string) (ImportResult, error) {
	var res ImportResult
	if s.fetcher == nil {
		return res, fmt.Errorf("litellm import: no fetcher configured")
	}
	if _, err := s.repo.GetPriceBook(ctx, priceBookID); err != nil {
		return res, err
	}
	models, err := s.fetcher.Fetch(ctx)
	if err != nil {
		return res, err
	}
	for name, m := range models {
		res.Fetched++
		if name == "sample_spec" {
			res.Skipped++
			continue
		}
		capability, ok := mapLiteLLMMode(m.Mode)
		if !ok {
			res.Skipped++
			continue
		}
		if m.InputCostPerToken <= 0 && m.OutputCostPerToken <= 0 {
			res.Skipped++
			continue
		}
		entry := domain.PriceBookEntry{
			PriceBookID:        priceBookID,
			ModelCode:          name,
			CapabilityType:     capability,
			InputPerToken:      m.InputCostPerToken,
			OutputPerToken:     m.OutputCostPerToken,
			CacheWritePerToken: m.CacheCreationInputTokenCost,
			CacheReadPerToken:  m.CacheReadInputTokenCost,
		}
		if err := s.repo.ImportEntry(ctx, entry); err != nil {
			return res, fmt.Errorf("import entry %q: %w", name, err)
		}
		res.Imported++
	}
	return res, nil
}

// mapLiteLLMMode maps a LiteLLM "mode" to our capability_type. Unknown/empty
// modes default to chat; truly unsupported modes return ok=false.
func mapLiteLLMMode(mode string) (string, bool) {
	switch mode {
	case "", "chat", "completion":
		return "chat", true
	case "embedding":
		return "embedding", true
	case "image_generation":
		return "image", true
	case "audio_transcription":
		return "audio_stt", true
	case "audio_speech":
		return "audio_tts", true
	case "rerank":
		return "rerank", true
	default:
		return "", false
	}
}

// LiteLLMModelInfo is a search hit: one LiteLLM model with per-1M USD prices,
// shaped for the admin entry autofill flow.
type LiteLLMModelInfo struct {
	ModelCode          string  `json:"model_code"`
	CapabilityType     string  `json:"capability_type"`
	InputPer1MUSD      float64 `json:"input_per_1m_usd"`
	OutputPer1MUSD     float64 `json:"output_per_1m_usd"`
	CacheWritePer1MUSD float64 `json:"cache_write_per_1m_usd"`
	CacheReadPer1MUSD  float64 `json:"cache_read_per_1m_usd"`
}

// litellmData returns the LiteLLM map, fetching + caching it in memory on first
// use or after the TTL. Never persisted to the DB.
func (s *Service) litellmData(ctx context.Context) (map[string]LiteLLMModel, error) {
	s.llmMu.Lock()
	defer s.llmMu.Unlock()
	if s.llmCache != nil && time.Now().Before(s.llmExp) {
		return s.llmCache, nil
	}
	if s.fetcher == nil {
		return nil, fmt.Errorf("litellm: no fetcher configured")
	}
	data, err := s.fetcher.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	s.llmCache = data
	s.llmExp = time.Now().Add(llmCacheTTL)
	return data, nil
}

// SearchLiteLLM returns up to limit LiteLLM models whose name contains q
// (case-insensitive). Empty q returns the first limit models. Prefix matches
// rank before substring matches; ties broken alphabetically.
func (s *Service) SearchLiteLLM(ctx context.Context, q string, limit int) ([]LiteLLMModelInfo, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	data, err := s.litellmData(ctx)
	if err != nil {
		return nil, err
	}
	ql := strings.ToLower(strings.TrimSpace(q))

	type scored struct {
		info LiteLLMModelInfo
		rank int // 0 = prefix, 1 = substring
	}
	matches := make([]scored, 0, limit*2)
	for name, m := range data {
		if name == "sample_spec" {
			continue
		}
		capability, ok := mapLiteLLMMode(m.Mode)
		if !ok {
			continue
		}
		if m.InputCostPerToken <= 0 && m.OutputCostPerToken <= 0 {
			continue
		}
		nl := strings.ToLower(name)
		rank := -1
		if ql == "" {
			rank = 1
		} else if strings.HasPrefix(nl, ql) {
			rank = 0
		} else if strings.Contains(nl, ql) {
			rank = 1
		}
		if rank < 0 {
			continue
		}
		matches = append(matches, scored{
			rank: rank,
			info: LiteLLMModelInfo{
				ModelCode:          name,
				CapabilityType:     capability,
				InputPer1MUSD:      m.InputCostPerToken * 1_000_000,
				OutputPer1MUSD:     m.OutputCostPerToken * 1_000_000,
				CacheWritePer1MUSD: m.CacheCreationInputTokenCost * 1_000_000,
				CacheReadPer1MUSD:  m.CacheReadInputTokenCost * 1_000_000,
			},
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		return matches[i].info.ModelCode < matches[j].info.ModelCode
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]LiteLLMModelInfo, len(matches))
	for i, m := range matches {
		out[i] = m.info
	}
	return out, nil
}

// commonModels is the curated "common models" starter set, mirroring the spirit
// of sub2api's hardcoded fallback list. Each entry is a list of candidate
// LiteLLM keys (first present wins) so we tolerate LiteLLM naming drift across
// versions. "自动同步" pulls only these from the in-memory LiteLLM catalog
// instead of materializing all ~1500 models.
var commonModels = [][]string{
	// OpenAI
	{"gpt-5.5"},
	{"gpt-5.4"},
	{"gpt-5.4-mini"},
	{"codex-auto-review"},
	// Anthropic Claude
	{"claude-haiku-4-5-20251001"},
	{"claude-opus-4-5-20251101"},
	{"claude-opus-4-6"},
	{"claude-opus-4-7"},
	{"claude-opus-4-8"},
	{"claude-sonnet-4-5-20250929"},
	{"claude-sonnet-4-6"},
	// Google Gemini
	{"gemini-3.1-flash-image-preview"},
	{"gemini-3.1-flash-lite"},
	{"gemini-3.1-flash-lite-preview"},
	{"gemini-3.1-flash-live-preview"},
	{"gemini-3.1-pro-preview"},
	{"gemini-3.1-pro-preview-customtools"},
	{"gemini-3.5-flash"},
}

// SyncResult summarizes one "common models" sync run.
type SyncResult struct {
	Synced  int      `json:"synced"`  // entries written (new or refreshed non-manual)
	Missing []string `json:"missing"` // curated models not found in the LiteLLM catalog
}

// SyncCommonModels fills the target price book with the curated common models'
// prices, pulled from the in-memory LiteLLM catalog. Fill-only: manually edited
// rows are preserved (via ImportEntry). Curated models absent from LiteLLM are
// reported in Missing.
func (s *Service) SyncCommonModels(ctx context.Context, priceBookID string) (SyncResult, error) {
	var res SyncResult
	if _, err := s.repo.GetPriceBook(ctx, priceBookID); err != nil {
		return res, err
	}
	data, err := s.litellmData(ctx)
	if err != nil {
		return res, err
	}
	for _, candidates := range commonModels {
		var hit *LiteLLMModel
		var hitName string
		for _, name := range candidates {
			if m, ok := data[name]; ok && (m.InputCostPerToken > 0 || m.OutputCostPerToken > 0) {
				mm := m
				hit = &mm
				hitName = name
				break
			}
		}
		if hit == nil {
			res.Missing = append(res.Missing, candidates[0])
			continue
		}
		capability, ok := mapLiteLLMMode(hit.Mode)
		if !ok {
			capability = "chat"
		}
		entry := domain.PriceBookEntry{
			PriceBookID:        priceBookID,
			ModelCode:          hitName,
			CapabilityType:     capability,
			InputPerToken:      hit.InputCostPerToken,
			OutputPerToken:     hit.OutputCostPerToken,
			CacheWritePerToken: hit.CacheCreationInputTokenCost,
			CacheReadPerToken:  hit.CacheReadInputTokenCost,
		}
		if err := s.repo.ImportEntry(ctx, entry); err != nil {
			return res, fmt.Errorf("sync %q: %w", hitName, err)
		}
		res.Synced++
	}
	return res, nil
}
