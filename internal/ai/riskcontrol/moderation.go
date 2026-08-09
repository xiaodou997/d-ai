package riskcontrol

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/secret"
)

const (
	excerptMaxRunes   = 500
	defaultAPITimeout = 5 * time.Second
)

// CheckInput carries the request-scoped fields a moderation check needs,
// independent of how the caller (sync pipeline step vs async worker)
// obtained them.
type CheckInput struct {
	RequestID      string
	TenantID       string
	UserID         string
	APIKeyID       string
	ModelCode      string
	CapabilityType string
	Text           string
}

// DetectResult is the pure detection outcome, before any DB writes.
type DetectResult struct {
	Flagged           bool
	MatchedKeyword    string
	HighestCategory   string
	HighestScore      *float64
	CategoryScores    map[string]float64
	APIError          string
	UpstreamLatencyMs *int32
	HitLayer          string // cache | keyword | pinyin | api
	FromCache         bool   // true when result came from L0 verdict cache
}

// Checker runs keyword + AI-moderation-API detection and records the
// outcome. It never mutates account/tenant status: crossing the violation
// threshold only creates an ai_risk_events row for a human to act on.
type Checker struct {
	Config          *ConfigService
	Logs            *LogService
	Events          *EventService
	HTTPClient      *http.Client
	SecretMasterKey string
	Logger          *zap.Logger

	// Engine management: engine is rebuilt when config_revision changes.
	engineMu  sync.RWMutex
	engine    *KeywordEngine
	engineRev int64

	// L0 verdict cache.
	cacheMu  sync.Mutex
	cache    *VerdictCache
	cacheTTL time.Duration
}

// Detect runs the full detection funnel: L0 cache → L1 keyword engine →
// Provider API. A matched keyword (block level) short-circuits the API
// call (cheap, no external cost). Sampling (cfg.SampleRate) only gates
// the API stage; keyword matching is always run when enabled.
func (c *Checker) Detect(ctx context.Context, cfg domain.RiskControlConfig, text string) DetectResult {
	if text == "" {
		return DetectResult{}
	}

	// Ensure L0 verdict cache exists with the right TTL.
	c.ensureCache(cfg)

	// L0: check verdict cache.
	textHash := sha256Hex(text)
	if c.cache != nil {
		if cached, ok := c.cache.Get(cfg.ConfigRevision, textHash); ok {
			return DetectResult{
				Flagged:         cached.Flagged,
				MatchedKeyword:  cached.MatchedKeyword,
				HitLayer:        domain.HitLayerCache,
				HighestCategory: cached.HighestCategory,
				HighestScore:    cached.HighestScore,
				FromCache:       true,
			}
		}
	}

	result := c.detectFresh(ctx, cfg, text)

	// L0: write verdict to cache (only for non-error results).
	if c.cache != nil && result.APIError == "" {
		c.cache.Put(cfg.ConfigRevision, textHash, CachedVerdict{
			Flagged:         result.Flagged,
			MatchedKeyword:  result.MatchedKeyword,
			HitLayer:        result.HitLayer,
			HighestCategory: result.HighestCategory,
			HighestScore:    result.HighestScore,
		})
	}

	return result
}

// detectFresh runs the detection pipeline without consulting the cache.
func (c *Checker) detectFresh(ctx context.Context, cfg domain.RiskControlConfig, text string) DetectResult {
	// L1: keyword engine (AC automaton + pinyin).
	if cfg.Keyword.Enabled {
		engine := c.getOrBuildEngine(cfg)
		if engine != nil {
			if match := engine.Match(text); match != nil {
				if match.Entry.Level == domain.KeywordLevelBlock {
					return DetectResult{
						Flagged:        true,
						MatchedKeyword: match.Entry.Word,
						HitLayer:       match.HitLayer,
					}
				}
				// Suspect level: log but don't block. HitLayer is set
				// so the log records which layer found it, but Flagged
				// stays false so it doesn't count toward risk events.
				return DetectResult{
					MatchedKeyword: match.Entry.Word,
					HitLayer:       match.HitLayer,
				}
			}
		}
	}

	// Provider API (optional, sampled).
	if cfg.Provider.BaseURL == "" || cfg.SampleRate <= 0 || !shouldSample(text, cfg.SampleRate) {
		return DetectResult{}
	}

	scores, latencyMs, err := c.callModerationAPI(ctx, cfg, text)
	result := DetectResult{UpstreamLatencyMs: &latencyMs, HitLayer: domain.HitLayerAPI}
	if err != nil {
		if c.Logger != nil {
			c.Logger.Warn("risk_control: moderation api call failed", zap.Error(err))
		}
		result.APIError = err.Error()
		return result
	}
	result.CategoryScores = scores
	result.Flagged, result.HighestCategory, result.HighestScore = evaluateScores(scores, cfg.Thresholds)
	return result
}

// getOrBuildEngine returns the current KeywordEngine, rebuilding it if
// the config revision has changed since the last build.
func (c *Checker) getOrBuildEngine(cfg domain.RiskControlConfig) *KeywordEngine {
	c.engineMu.RLock()
	if c.engine != nil && c.engineRev == cfg.ConfigRevision {
		e := c.engine
		c.engineMu.RUnlock()
		return e
	}
	c.engineMu.RUnlock()

	// Rebuild under write lock.
	c.engineMu.Lock()
	defer c.engineMu.Unlock()

	// Double-check after acquiring write lock.
	if c.engine != nil && c.engineRev == cfg.ConfigRevision {
		return c.engine
	}

	engine := NewKeywordEngine(cfg.Keyword)
	c.engine = engine
	c.engineRev = cfg.ConfigRevision
	return engine
}

// ensureCache creates or recreates the L0 verdict cache if the TTL has
// changed or the cache doesn't exist yet.
func (c *Checker) ensureCache(cfg domain.RiskControlConfig) {
	ttl := time.Duration(cfg.VerdictCacheTTLSeconds) * time.Second
	if cfg.VerdictCacheTTLSeconds == 0 {
		ttl = 0 // disabled
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if c.cache == nil || c.cacheTTL != ttl {
		if ttl == 0 {
			c.cache = nil
		} else {
			c.cache = NewVerdictCache(defaultVerdictCacheCapacity, ttl)
		}
		c.cacheTTL = ttl
	}
}

// Record persists the detection outcome (subject to cfg.RecordNonHits for
// non-flagged results) and, when a per-user violation count crosses the
// configured threshold within the rolling window, creates an ai_risk_events
// row. It returns the log action for the caller (e.g. to decide whether to
// block the request in pre_block mode).
//
// Cache hits (FromCache=true) should NOT call Record — the caller is
// responsible for skipping it.
func (c *Checker) Record(ctx context.Context, cfg domain.RiskControlConfig, in CheckInput, det DetectResult, mode string) string {
	action := domain.RiskControlActionAllow
	switch {
	case det.APIError != "":
		action = domain.RiskControlActionError
	case det.Flagged && (det.HitLayer == domain.HitLayerKeyword || det.HitLayer == domain.HitLayerPinyin):
		action = domain.RiskControlActionKeywordBlock
	case det.Flagged:
		action = domain.RiskControlActionBlock
	}

	// Suspect-level keyword hits (not flagged) are always logged so
	// admins can review them, regardless of RecordNonHits.
	isSuspectHit := !det.Flagged && det.MatchedKeyword != "" && det.HitLayer != ""

	if !det.Flagged && det.APIError == "" && !isSuspectHit && !cfg.RecordNonHits {
		return action
	}

	log := domain.ContentModerationLog{
		RequestID:         in.RequestID,
		TenantID:          in.TenantID,
		UserID:            in.UserID,
		APIKeyID:          in.APIKeyID,
		ModelCode:         in.ModelCode,
		CapabilityType:    in.CapabilityType,
		Mode:              mode,
		Action:            action,
		Flagged:           det.Flagged,
		MatchedKeyword:    det.MatchedKeyword,
		HighestCategory:   det.HighestCategory,
		HighestScore:      det.HighestScore,
		InputExcerpt:      excerptText(in.Text),
		UpstreamLatencyMs: det.UpstreamLatencyMs,
		Error:             det.APIError,
		HitLayer:          det.HitLayer,
	}
	if det.CategoryScores != nil {
		if b, err := json.Marshal(det.CategoryScores); err == nil {
			log.CategoryScores = b
		}
	}
	if cfg.Thresholds != nil {
		if b, err := json.Marshal(cfg.Thresholds); err == nil {
			log.ThresholdSnapshot = b
		}
	}

	logID, _, err := c.Logs.Insert(ctx, log)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Error("risk_control: failed to insert moderation log", zap.Error(err))
		}
		return action
	}

	if det.Flagged && in.UserID != "" && cfg.RiskEventThreshold > 0 {
		c.maybeRaiseRiskEvent(ctx, cfg, in, det, logID)
	}
	return action
}

func (c *Checker) maybeRaiseRiskEvent(ctx context.Context, cfg domain.RiskControlConfig, in CheckInput, det DetectResult, logID string) {
	windowHours := cfg.ViolationWindowHours
	if windowHours <= 0 {
		windowHours = 24
	}
	since := time.Now().Add(-time.Duration(windowHours) * time.Hour)
	count, err := c.Logs.CountFlaggedSince(ctx, in.UserID, since)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Error("risk_control: failed to count flagged logs for risk event", zap.Error(err))
		}
		return
	}
	// Fire every Nth violation within the window (3, 6, 9, ...) rather than
	// only once, so repeat offenders keep surfacing new events even after an
	// earlier one is resolved.
	threshold := int64(cfg.RiskEventThreshold)
	if count == 0 || count%threshold != 0 {
		return
	}

	severity := domain.RiskEventSeverityMedium
	if count >= threshold*3 {
		severity = domain.RiskEventSeverityHigh
	}
	summary := fmt.Sprintf("用户在 %d 小时内累计触发 %d 次内容风控命中", windowHours, count)
	detail, _ := json.Marshal(map[string]any{
		"matched_keyword":  det.MatchedKeyword,
		"highest_category": det.HighestCategory,
		"highest_score":    det.HighestScore,
		"hit_layer":        det.HitLayer,
		"window_hours":     windowHours,
		"violation_count":  count,
	})

	if _, err := c.Events.Create(ctx, domain.RiskEvent{
		EventType:   "content_violation",
		Severity:    severity,
		TenantID:    in.TenantID,
		UserID:      in.UserID,
		SourceLogID: logID,
		Summary:     summary,
		Detail:      detail,
	}); err != nil && c.Logger != nil {
		c.Logger.Error("risk_control: failed to create risk event", zap.Error(err))
	}
}

// shouldSample deterministically hashes text so repeated identical inputs
// sample consistently rather than flapping between runs.
func shouldSample(text string, rate float64) bool {
	if rate >= 1 {
		return true
	}
	if rate <= 0 {
		return false
	}
	sum := sha256.Sum256([]byte(text))
	n := binary.BigEndian.Uint32(sum[:4])
	return float64(n%10000) < rate*10000
}

func evaluateScores(scores, thresholds map[string]float64) (flagged bool, highestCategory string, highestScore *float64) {
	var maxScore float64
	found := false
	for category, score := range scores {
		if !found || score > maxScore {
			maxScore = score
			highestCategory = category
			found = true
		}
		if th, ok := thresholds[category]; ok && score >= th {
			flagged = true
		}
	}
	if !found {
		return false, "", nil
	}
	v := maxScore
	return flagged, highestCategory, &v
}

func excerptText(s string) string {
	r := []rune(s)
	if len(r) <= excerptMaxRunes {
		return s
	}
	return string(r[:excerptMaxRunes]) + "…"
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type moderationAPIRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type moderationAPIResponse struct {
	Results []struct {
		Flagged        bool               `json:"flagged"`
		CategoryScores map[string]float64 `json:"category_scores"`
	} `json:"results"`
}

func (c *Checker) callModerationAPI(ctx context.Context, cfg domain.RiskControlConfig, text string) (map[string]float64, int32, error) {
	start := time.Now()
	apiKey, err := secret.DecryptProviderKey(c.SecretMasterKey, cfg.Provider.APIKeyCiphertext)
	if err != nil {
		return nil, 0, fmt.Errorf("decrypt moderation api key: %w", err)
	}

	body, err := json.Marshal(moderationAPIRequest{Model: cfg.Provider.Model, Input: text})
	if err != nil {
		return nil, 0, err
	}

	timeout := time.Duration(cfg.Provider.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultAPITimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimRight(cfg.Provider.BaseURL, "/") + "/v1/moderations"
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	latencyMs := int32(time.Since(start).Milliseconds())
	if err != nil {
		return nil, latencyMs, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, latencyMs, fmt.Errorf("moderation api returned status %d", resp.StatusCode)
	}
	var parsed moderationAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, latencyMs, err
	}
	if len(parsed.Results) == 0 {
		return nil, latencyMs, errors.New("moderation api: empty results")
	}
	return parsed.Results[0].CategoryScores, latencyMs, nil
}
