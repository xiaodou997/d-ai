package billingcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const llmCacheTTL = 6 * time.Hour
const litellmPerMillion = 1_000_000.0
const DefaultLiteLLMURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

var builtInLiteLLMModels = map[string]LiteLLMModel{
	"gpt-5.6-sol": {
		Mode:               "chat",
		InputCostPerToken:  5.0 / litellmPerMillion,
		OutputCostPerToken: 30.0 / litellmPerMillion,
	},
	"gpt-5.6-terra": {
		Mode:               "chat",
		InputCostPerToken:  2.5 / litellmPerMillion,
		OutputCostPerToken: 15.0 / litellmPerMillion,
	},
	"gpt-5.6-luna": {
		Mode:               "chat",
		InputCostPerToken:  1.0 / litellmPerMillion,
		OutputCostPerToken: 6.0 / litellmPerMillion,
	},
	"codex-auto-review": {
		Mode:                        "chat",
		InputCostPerToken:           5.0 / litellmPerMillion,
		OutputCostPerToken:          30.0 / litellmPerMillion,
		CacheCreationInputTokenCost: 5.0 / litellmPerMillion,
		CacheReadInputTokenCost:     0.5 / litellmPerMillion,
		cacheCreationPricePresent:   true,
		cacheReadPricePresent:       true,
	},
}

type LiteLLMModel struct {
	InputCostPerToken                 float64 `json:"input_cost_per_token"`
	OutputCostPerToken                float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost       float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost           float64 `json:"cache_read_input_token_cost"`
	Mode                              string  `json:"mode"`
	LongContextInputTokenThreshold    int     `json:"long_context_input_token_threshold"`
	LongContextInputTenantMultiplier  float64 `json:"long_context_input_tenant_multiplier"`
	LongContextOutputTenantMultiplier float64 `json:"long_context_output_tenant_multiplier"`
	cacheCreationPricePresent         bool
	cacheReadPricePresent             bool
	abovePrices                       map[int]liteLLMAbovePrice
}

type liteLLMAbovePrice struct {
	input, output, cacheWrite, cacheRead *float64
}

var liteLLMAboveField = regexp.MustCompile(`^(input_cost_per_token|output_cost_per_token|cache_creation_input_token_cost|cache_read_input_token_cost)_above_([0-9]+)k_tokens$`)

func (m *LiteLLMModel) UnmarshalJSON(data []byte) error {
	type modelAlias LiteLLMModel
	var base modelAlias
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	*m = LiteLLMModel(base)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	_, m.cacheCreationPricePresent = raw["cache_creation_input_token_cost"]
	_, m.cacheReadPricePresent = raw["cache_read_input_token_cost"]
	for key, value := range raw {
		match := liteLLMAboveField.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		thresholdK, err := strconv.Atoi(match[2])
		if err != nil || thresholdK <= 0 {
			continue
		}
		var price float64
		if err := json.Unmarshal(value, &price); err != nil {
			continue
		}
		if m.abovePrices == nil {
			m.abovePrices = make(map[int]liteLLMAbovePrice)
		}
		threshold := thresholdK * 1000
		item := m.abovePrices[threshold]
		switch match[1] {
		case "input_cost_per_token":
			item.input = float64Ptr(price)
		case "output_cost_per_token":
			item.output = float64Ptr(price)
		case "cache_creation_input_token_cost":
			item.cacheWrite = float64Ptr(price)
		case "cache_read_input_token_cost":
			item.cacheRead = float64Ptr(price)
		}
		m.abovePrices[threshold] = item
	}
	return nil
}

type LiteLLMFetcher interface {
	Fetch(ctx context.Context) (map[string]LiteLLMModel, error)
}

type HTTPFetcher struct {
	URL    string
	client *http.Client
}

func NewHTTPFetcher(url string) *HTTPFetcher {
	if url == "" {
		url = DefaultLiteLLMURL
	}
	return &HTTPFetcher{URL: url, client: &http.Client{Timeout: 120 * time.Second}}
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("read litellm prices: %w", err)
	}
	var raw map[string]LiteLLMModel
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse litellm prices: %w", err)
	}
	return raw, nil
}

type ImportResult struct {
	Fetched  int `json:"fetched"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

func (s *Service) ImportFromLiteLLM(ctx context.Context, priceBookID string) (ImportResult, error) {
	return s.importFromLiteLLM(ctx, domain.PriceBookOwnerPlatform, "", priceBookID)
}

func (s *Service) ImportTenantFromLiteLLM(ctx context.Context, tenantID, priceBookID string) (ImportResult, error) {
	return s.importFromLiteLLM(ctx, domain.PriceBookOwnerTenant, tenantID, priceBookID)
}

func (s *Service) importFromLiteLLM(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID, priceBookID string) (ImportResult, error) {
	var res ImportResult
	if err := s.requireOwner(ctx, priceBookID, ownerType, ownerTenantID); err != nil {
		return res, err
	}
	models := s.llmSource.Snapshot()
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]domain.PriceBookEntry, 0, len(names))
	for _, name := range names {
		m := models[name]
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
		if capability == "image" || capability == "video" {
			res.Skipped++
			continue
		}
		if m.InputCostPerToken <= 0 && m.OutputCostPerToken <= 0 {
			res.Skipped++
			continue
		}
		entry := domain.PriceBookEntry{
			PriceBookID:     priceBookID,
			ModelCode:       name,
			CapabilityType:  capability,
			TokenPriceTiers: tokenPriceTiersFromLiteLLM(m),
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return res, nil
	}
	changed, err := s.repo.ImportEntries(ctx, priceBookID, entries)
	if err != nil {
		return res, fmt.Errorf("import LiteLLM entries: %w", err)
	}
	res.Imported = changed
	res.Skipped += len(entries) - changed
	return res, nil
}

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

type LiteLLMModelInfo struct {
	ModelCode       string                     `json:"model_code"`
	CapabilityType  string                     `json:"capability_type"`
	TokenPriceTiers []LiteLLMTokenPriceTierDTO `json:"token_price_tiers"`
}

type LiteLLMTokenPriceTierDTO struct {
	UpToInputTokens    *int    `json:"up_to_input_tokens"`
	InputPer1MUSD      float64 `json:"input_per_1m_usd"`
	OutputPer1MUSD     float64 `json:"output_per_1m_usd"`
	CacheWritePer1MUSD float64 `json:"cache_write_per_1m_usd"`
	CacheReadPer1MUSD  float64 `json:"cache_read_per_1m_usd"`
}

func (s *Service) SearchLiteLLM(_ context.Context, q string, limit int) ([]LiteLLMModelInfo, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	data := s.llmSource.Snapshot()
	ql := strings.ToLower(strings.TrimSpace(q))

	type scored struct {
		info LiteLLMModelInfo
		rank int
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
			info: LiteLLMModelInfo{ModelCode: name, CapabilityType: capability, TokenPriceTiers: liteLLMTiersToDTO(tokenPriceTiersFromLiteLLM(m))},
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

var commonModels = [][]string{
	{"gpt-5.6-sol"},
	{"gpt-5.6-terra"},
	{"gpt-5.6-luna"},
	{"gpt-5.5"},
	{"gpt-5.4"},
	{"gpt-5.4-mini"},
	{"codex-auto-review"},
	{"claude-haiku-4-5-20251001"},
	{"claude-opus-4-5-20251101"},
	{"claude-opus-4-6"},
	{"claude-opus-4-7"},
	{"claude-opus-4-8"},
	{"claude-sonnet-5"},
	{"claude-fable-5"},
	{"claude-sonnet-4-6"},
	{"gemini-3.1-flash-lite"},
	{"gemini-3.1-flash-lite-preview"},
	{"gemini-3.1-flash-live-preview"},
	{"gemini-3.1-pro-preview"},
	{"gemini-3.1-pro-preview-customtools"},
	{"gemini-3.5-flash"},
}

type SyncResult struct {
	Synced  int      `json:"synced"`
	Missing []string `json:"missing"`
}

func (s *Service) SyncCommonModels(ctx context.Context, priceBookID string) (SyncResult, error) {
	return s.syncCommonModels(ctx, domain.PriceBookOwnerPlatform, "", priceBookID)
}

func (s *Service) SyncTenantCommonModels(ctx context.Context, tenantID, priceBookID string) (SyncResult, error) {
	return s.syncCommonModels(ctx, domain.PriceBookOwnerTenant, tenantID, priceBookID)
}

func (s *Service) syncCommonModels(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID, priceBookID string) (SyncResult, error) {
	var res SyncResult
	if err := s.requireOwner(ctx, priceBookID, ownerType, ownerTenantID); err != nil {
		return res, err
	}
	data := s.llmSource.Snapshot()
	entries := make([]domain.PriceBookEntry, 0, len(commonModels))
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
			PriceBookID:     priceBookID,
			ModelCode:       hitName,
			CapabilityType:  capability,
			TokenPriceTiers: tokenPriceTiersFromLiteLLM(*hit),
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return res, nil
	}
	// Keep the ordered common-model selection semantics, but apply the whole
	// snapshot in one repository transaction. The repository's conditional
	// upsert makes retries and concurrent replicas no-ops when the same LiteLLM
	// values are already present.
	changed, err := s.repo.ImportEntries(ctx, priceBookID, entries)
	if err != nil {
		return res, fmt.Errorf("sync LiteLLM entries: %w", err)
	}
	res.Synced = changed
	return res, nil
}

func normalizeLiteLLMPerMillionPrice(v float64) float64 {
	return domain.RoundUpCurrency2(v)
}

func normalizeLiteLLMPerTokenPrice(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return normalizeLiteLLMPerMillionPrice(v*litellmPerMillion) / litellmPerMillion
}

func tokenPriceTiersFromLiteLLM(model LiteLLMModel) []domain.TokenPriceTier {
	input := normalizeLiteLLMPerTokenPrice(model.InputCostPerToken)
	output := normalizeLiteLLMPerTokenPrice(model.OutputCostPerToken)
	current := domain.TokenPriceTier{
		InputPerToken:      input,
		OutputPerToken:     output,
		CacheWritePerToken: normalizedCachePrice(model.CacheCreationInputTokenCost, model.cacheCreationPricePresent, input),
		CacheReadPerToken:  normalizedCachePrice(model.CacheReadInputTokenCost, model.cacheReadPricePresent, input),
	}

	thresholds := make([]int, 0, len(model.abovePrices))
	for threshold := range model.abovePrices {
		thresholds = append(thresholds, threshold)
	}
	sort.Ints(thresholds)
	if len(thresholds) == 0 && model.LongContextInputTokenThreshold > 0 &&
		(model.LongContextInputTenantMultiplier > 0 || model.LongContextOutputTenantMultiplier > 0) {
		threshold := model.LongContextInputTokenThreshold
		thresholds = append(thresholds, threshold)
		inputMultiplier := model.LongContextInputTenantMultiplier
		if inputMultiplier <= 0 {
			inputMultiplier = 1
		}
		outputMultiplier := model.LongContextOutputTenantMultiplier
		if outputMultiplier <= 0 {
			outputMultiplier = 1
		}
		longInput := input * inputMultiplier
		longOutput := output * outputMultiplier
		model.abovePrices = map[int]liteLLMAbovePrice{
			threshold: {input: &longInput, output: &longOutput},
		}
	}

	tiers := make([]domain.TokenPriceTier, 0, len(thresholds)+1)
	for _, threshold := range thresholds {
		limit := threshold
		current.UpToInputTokens = &limit
		tiers = append(tiers, current)
		above := model.abovePrices[threshold]
		nextInput := current.InputPerToken
		if above.input != nil {
			nextInput = normalizeLiteLLMPerTokenPrice(*above.input)
		}
		nextOutput := current.OutputPerToken
		if above.output != nil {
			nextOutput = normalizeLiteLLMPerTokenPrice(*above.output)
		}
		current = domain.TokenPriceTier{
			InputPerToken:      nextInput,
			OutputPerToken:     nextOutput,
			CacheWritePerToken: normalizedOptionalCachePrice(above.cacheWrite, nextInput),
			CacheReadPerToken:  normalizedOptionalCachePrice(above.cacheRead, nextInput),
		}
	}
	current.UpToInputTokens = nil
	return append(tiers, current)
}

func normalizedCachePrice(price float64, present bool, input float64) float64 {
	if !present {
		return input
	}
	return normalizeLiteLLMPerTokenPrice(price)
}

func normalizedOptionalCachePrice(price *float64, input float64) float64 {
	if price == nil {
		return input
	}
	return normalizeLiteLLMPerTokenPrice(*price)
}

func liteLLMTiersToDTO(tiers []domain.TokenPriceTier) []LiteLLMTokenPriceTierDTO {
	out := make([]LiteLLMTokenPriceTierDTO, 0, len(tiers))
	for _, tier := range tiers {
		out = append(out, LiteLLMTokenPriceTierDTO{
			UpToInputTokens:    tier.UpToInputTokens,
			InputPer1MUSD:      normalizeLiteLLMPerMillionPrice(tier.InputPerToken * litellmPerMillion),
			OutputPer1MUSD:     normalizeLiteLLMPerMillionPrice(tier.OutputPerToken * litellmPerMillion),
			CacheWritePer1MUSD: normalizeLiteLLMPerMillionPrice(tier.CacheWritePerToken * litellmPerMillion),
			CacheReadPer1MUSD:  normalizeLiteLLMPerMillionPrice(tier.CacheReadPerToken * litellmPerMillion),
		})
	}
	return out
}

func float64Ptr(value float64) *float64 { return &value }

func mergedLiteLLMData(remote map[string]LiteLLMModel) map[string]LiteLLMModel {
	merged := make(map[string]LiteLLMModel, len(remote)+len(builtInLiteLLMModels))
	for name, model := range remote {
		merged[name] = model
	}
	for name, model := range builtInLiteLLMModels {
		if _, exists := merged[name]; !exists {
			merged[name] = model
		}
	}
	return merged
}
