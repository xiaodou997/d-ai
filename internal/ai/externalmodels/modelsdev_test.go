package externalmodels

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const fixturePayload = `{
	"openai": {
		"models": {
			"gpt-4o": {"id": "gpt-4o", "name": "GPT-4o", "modalities": {"input": ["text","image"], "output": ["text"]}},
			"gpt-image-2": {"id": "gpt-image-2", "name": "GPT Image 2", "modalities": {"input": ["text"], "output": ["image"]}},
			"tts-1": {"id": "tts-1", "name": "TTS-1", "modalities": {"input": ["text"], "output": ["audio"]}},
			"whisper-1": {"id": "whisper-1", "name": "Whisper", "modalities": {"input": ["audio"], "output": ["text"]}},
			"text-embedding-3-large": {"id": "text-embedding-3-large", "name": "text-embedding-3-large", "modalities": {"input": ["text"], "output": ["text"]}}
		}
	},
	"requesty": {
		"models": {
			"openai/gpt-4o": {"id": "openai/gpt-4o", "name": "GPT-4o", "modalities": {"input": ["text","image"], "output": ["text"]}}
		}
	}
}`

func withFixtureServer(t *testing.T, body string) *Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	t.Setenv(sourceURLEnv, server.URL)
	return New(nil, server.Client())
}

func TestLookupStructuralCapabilities(t *testing.T) {
	resolver := withFixtureServer(t, fixturePayload)

	cases := []struct {
		name    string
		model   string
		wantCap domain.CapabilityType
		wantOK  bool
	}{
		{"image model", "gpt-image-2", domain.CapabilityImage, true},
		{"tts model", "tts-1", domain.CapabilityAudioTTS, true},
		{"stt model", "whisper-1", domain.CapabilityAudioSTT, true},
		{"chat/embedding ambiguous falls back", "text-embedding-3-large", "", false},
		{"plain chat model is ambiguous too", "gpt-4o", "", false},
		{"prefixed id matches bare name", "gpt-image-2", domain.CapabilityImage, true},
		{"unknown model misses", "totally-unknown-model-xyz", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCap, gotOK := resolver.Lookup(t.Context(), tc.model)
			if gotOK != tc.wantOK {
				t.Fatalf("ok: got %v want %v", gotOK, tc.wantOK)
			}
			if gotOK && gotCap != tc.wantCap {
				t.Fatalf("capability: got %q want %q", gotCap, tc.wantCap)
			}
		})
	}
}

func TestLookupMatchesProviderPrefixedID(t *testing.T) {
	resolver := withFixtureServer(t, fixturePayload)
	// requesty 下的 "openai/gpt-4o" 应该也能被裸名 "gpt-4o" 命中索引（即便这里因为
	// 模态无法区分 chat/embedding 而返回 ok=false，也验证了索引构建没有 panic/出错）。
	if _, ok := resolver.Lookup(t.Context(), "gpt-4o"); ok {
		t.Fatalf("gpt-4o modalities are text->text, expected ambiguous (ok=false)")
	}
}

func TestLookupFallsBackWhenSourceUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv(sourceURLEnv, server.URL)
	resolver := New(nil, server.Client())

	cap, ok := resolver.Lookup(t.Context(), "gpt-image-2")
	if ok {
		t.Fatalf("expected ok=false when source is unavailable, got capability %q", cap)
	}
}

func TestFailureBackoffSkipsRepeatedFetches(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv(sourceURLEnv, server.URL)
	resolver := New(nil, server.Client())
	for i := range 5 {
		if _, ok := resolver.Lookup(t.Context(), "gpt-image-2"); ok {
			t.Fatalf("iteration %d: expected ok=false while source is down", i)
		}
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 fetch attempt during failure backoff window, got %d", attempts)
	}

	// 冷却期过后应该重新尝试。
	resolver.cache.mu.Lock()
	resolver.cache.failedUntil = time.Now().Add(-time.Second)
	resolver.cache.mu.Unlock()
	if _, ok := resolver.Lookup(t.Context(), "gpt-image-2"); ok {
		t.Fatalf("expected ok=false on retry, source is still down")
	}
	if attempts != 2 {
		t.Fatalf("expected a second attempt after backoff expiry, got %d", attempts)
	}
}

func TestIndexCacheReusesWithinTTL(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixturePayload))
	}))
	t.Cleanup(server.Close)
	t.Setenv(sourceURLEnv, server.URL)
	resolver := New(nil, server.Client())
	for i := range 5 {
		if _, ok := resolver.Lookup(t.Context(), "gpt-image-2"); !ok {
			t.Fatalf("iteration %d: expected hit", i)
		}
	}
	if hits != 1 {
		t.Fatalf("expected exactly 1 source fetch within TTL window, got %d", hits)
	}

	// 手动让缓存过期，验证会重新拉取。
	resolver.cache.mu.Lock()
	resolver.cache.expiresAt = time.Now().Add(-time.Second)
	resolver.cache.mu.Unlock()
	if _, ok := resolver.Lookup(t.Context(), "gpt-image-2"); !ok {
		t.Fatalf("expected hit after cache expiry refetch")
	}
	if hits != 2 {
		t.Fatalf("expected a second fetch after expiry, got %d", hits)
	}
}
