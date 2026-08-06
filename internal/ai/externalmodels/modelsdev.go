// Package externalmodels 从外部权威模型目录（models.dev）拉取模型模态信息，
// 用于给"上游模型绑定"表单的能力类型提供更准确的默认值建议。
//
// 设计原则：只做只读的推断建议缓存，不落库、不参与提交时校验——真正的合法性
// 红线仍是 domain.DefaultProtocolForCapability 配合 transport 层的
// bindingProtocolSupportsCapability；这里命中与否都不影响用户手动改选的能力。
//
// models.dev 的 modalities 字段能结构化区分 image / audio_tts / audio_stt，
// 但无法区分 chat / embedding / rerank（这三者都是纯 text→text），后两者仍需
// 调用方回退到 domain.InferModelCapabilityAndProtocol 的关键词启发式。
package externalmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/ai/domain"
)

const (
	sourceURLEnv     = "AI_EXTERNAL_MODELS_URL"
	sourceURLDefault = "https://models.dev/api.json"
	redisCacheKey    = "dai:external:models_dev"
	cacheTTL         = 15 * time.Minute
	fetchTimeout     = 60 * time.Second
	// failureBackoff 是拉取失败后的冷却期：冷却期内的调用直接跳过网络请求，
	// 避免 models.dev 真的挂掉时，每一次 infer 请求都各自付出一次 fetchTimeout
	// 的等待（fetchTimeout 拉长到 60s 后，没有这层退避会让故障期间的体验变得很差）。
	failureBackoff = 60 * time.Second
)

type modelEntry struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Family     string `json:"family"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
}

type providerEntry struct {
	Models map[string]modelEntry `json:"models"`
}

type modelIndex map[string]modelEntry

var shared indexCache

// indexCache 是进程内的两级缓存：命中窗口内直接复用已解析好的索引，
// 避免每次调用都重新反序列化 models.dev 那份几 MB 的 JSON。
// Redis 不可用时，这一层本身就充当"内存缓存"，语义上退化但不缺失。
type indexCache struct {
	mu          sync.RWMutex
	index       modelIndex
	expiresAt   time.Time
	failedUntil time.Time // 拉取失败后的冷却期截止时间，冷却期内不再重试网络请求
}

func (c *indexCache) get(ctx context.Context, redisClient *redis.Client, httpClient *http.Client) modelIndex {
	c.mu.RLock()
	if c.index != nil && time.Now().Before(c.expiresAt) {
		idx := c.index
		c.mu.RUnlock()
		return idx
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index != nil && time.Now().Before(c.expiresAt) {
		return c.index
	}
	if time.Now().Before(c.failedUntil) {
		return c.index // 冷却期内直接用旧缓存（可能是 nil），不再发起网络请求
	}

	raw, err := loadRawPayload(ctx, redisClient, httpClient)
	if err != nil {
		c.failedUntil = time.Now().Add(failureBackoff)
		return c.index // 拉取失败：有旧缓存（哪怕已过期）就继续用，没有就返回 nil 交给调用方回退本地启发式
	}
	idx, err := buildIndex(raw)
	if err != nil {
		c.failedUntil = time.Now().Add(failureBackoff)
		return c.index
	}
	c.index = idx
	c.expiresAt = time.Now().Add(cacheTTL)
	c.failedUntil = time.Time{}
	return idx
}

func loadRawPayload(ctx context.Context, redisClient *redis.Client, httpClient *http.Client) ([]byte, error) {
	if redisClient != nil {
		if raw, err := redisClient.Get(ctx, redisCacheKey).Bytes(); err == nil && len(raw) > 0 {
			return raw, nil
		}
	}

	raw, err := fetchFromSource(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	if redisClient != nil {
		_ = redisClient.Set(ctx, redisCacheKey, raw, cacheTTL).Err()
	}
	return raw, nil
}

func fetchFromSource(ctx context.Context, httpClient *http.Client) ([]byte, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("external models source returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func sourceURL() string {
	if v := strings.TrimSpace(os.Getenv(sourceURLEnv)); v != "" {
		return v
	}
	return sourceURLDefault
}

func buildIndex(raw []byte) (modelIndex, error) {
	var providers map[string]providerEntry
	if err := json.Unmarshal(raw, &providers); err != nil {
		return nil, err
	}
	idx := make(modelIndex)
	for _, provider := range providers {
		for modelID, entry := range provider.Models {
			entry.ID = modelID
			for _, key := range indexKeys(modelID) {
				if _, exists := idx[key]; !exists {
					idx[key] = entry
				}
			}
		}
	}
	return idx, nil
}

// indexKeys 返回一个 model id 用于索引的候选 key：完整 id，以及去掉
// "provider/" 前缀后的裸模型名（管理员手填 model_code 通常是裸名）。
func indexKeys(modelID string) []string {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if lower == "" {
		return nil
	}
	if idx := strings.LastIndex(lower, "/"); idx >= 0 && idx+1 < len(lower) {
		return []string{lower, lower[idx+1:]}
	}
	return []string{lower}
}

// capabilityFromModalities 只对模态上结构化可辨识的能力给出结论：
// 生图（output 含 image）、语音合成（纯 text→audio）、语音识别（纯 audio→text）。
// chat/embedding/rerank 在 models.dev 里都是 text→text，无法靠模态区分，返回 ok=false。
func capabilityFromModalities(input, output []string) (domain.CapabilityType, bool) {
	hasImageOut := containsStr(output, "image")
	hasAudioOut := containsStr(output, "audio")
	hasAudioIn := containsStr(input, "audio")
	switch {
	case hasImageOut:
		return domain.CapabilityImage, true
	case hasAudioOut && !hasAudioIn:
		return domain.CapabilityAudioTTS, true
	case hasAudioIn && !hasAudioOut:
		return domain.CapabilityAudioSTT, true
	default:
		return "", false
	}
}

func containsStr(list []string, target string) bool {
	for _, v := range list {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}

// Lookup 按 model_code 查询 models.dev 缓存目录，返回结构化可辨识的能力类型。
// ok=false 表示未命中，或命中了但模态无法区分 chat/embedding/rerank——
// 两种情况调用方都应回退到 domain.InferModelCapabilityAndProtocol 的本地启发式。
func Lookup(ctx context.Context, redisClient *redis.Client, httpClient *http.Client, modelCode string) (domain.CapabilityType, bool) {
	key := strings.ToLower(strings.TrimSpace(modelCode))
	if key == "" {
		return "", false
	}
	idx := shared.get(ctx, redisClient, httpClient)
	if idx == nil {
		return "", false
	}
	entry, found := idx[key]
	if !found {
		return "", false
	}
	return capabilityFromModalities(entry.Modalities.Input, entry.Modalities.Output)
}
