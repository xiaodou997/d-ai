package riskcontrol

import (
	"testing"
	"time"
)

func TestVerdictCache_GetMiss(t *testing.T) {
	cache := NewVerdictCache(100, 10*time.Minute)
	_, ok := cache.Get(1, "nonexistent")
	if ok {
		t.Fatal("expected cache miss for non-existent key")
	}
}

func TestVerdictCache_PutThenGet(t *testing.T) {
	cache := NewVerdictCache(100, 10*time.Minute)
	verdict := CachedVerdict{Flagged: true, MatchedKeyword: "bad", HitLayer: "keyword"}
	cache.Put(1, "hash1", verdict)

	got, ok := cache.Get(1, "hash1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !got.Flagged || got.MatchedKeyword != "bad" || got.HitLayer != "keyword" {
		t.Fatalf("unexpected verdict: %#v", got)
	}
}

func TestVerdictCache_RevisionMismatch(t *testing.T) {
	cache := NewVerdictCache(100, 10*time.Minute)
	cache.Put(1, "hash1", CachedVerdict{Flagged: true})

	// Same hash but different revision should miss.
	_, ok := cache.Get(2, "hash1")
	if ok {
		t.Fatal("expected cache miss for revision mismatch")
	}
}

func TestVerdictCache_TTLExpiry(t *testing.T) {
	cache := NewVerdictCache(100, 50*time.Millisecond)
	cache.Put(1, "hash1", CachedVerdict{Flagged: true})

	// Should hit immediately.
	_, ok := cache.Get(1, "hash1")
	if !ok {
		t.Fatal("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire.
	time.Sleep(60 * time.Millisecond)

	// Should miss after TTL.
	_, ok = cache.Get(1, "hash1")
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestVerdictCache_LRUEviction(t *testing.T) {
	cache := NewVerdictCache(2, 10*time.Minute)

	cache.Put(1, "hash1", CachedVerdict{Flagged: true, MatchedKeyword: "first"})
	cache.Put(1, "hash2", CachedVerdict{Flagged: true, MatchedKeyword: "second"})

	// Access hash1 to make it most-recently-used.
	cache.Get(1, "hash1")

	// Insert hash3, should evict hash2 (least recently used).
	cache.Put(1, "hash3", CachedVerdict{Flagged: true, MatchedKeyword: "third"})

	// hash1 should still be present.
	_, ok := cache.Get(1, "hash1")
	if !ok {
		t.Fatal("expected hash1 to survive eviction")
	}

	// hash2 should have been evicted.
	_, ok = cache.Get(1, "hash2")
	if ok {
		t.Fatal("expected hash2 to be evicted")
	}

	// hash3 should be present.
	got, ok := cache.Get(1, "hash3")
	if !ok || got.MatchedKeyword != "third" {
		t.Fatalf("expected hash3 to be present, got %#v", got)
	}
}

func TestVerdictCache_UpdateExisting(t *testing.T) {
	cache := NewVerdictCache(100, 10*time.Minute)
	cache.Put(1, "hash1", CachedVerdict{Flagged: false})

	// Update with new verdict.
	cache.Put(1, "hash1", CachedVerdict{Flagged: true, MatchedKeyword: "updated"})

	got, ok := cache.Get(1, "hash1")
	if !ok {
		t.Fatal("expected cache hit after update")
	}
	if !got.Flagged || got.MatchedKeyword != "updated" {
		t.Fatalf("expected updated verdict, got %#v", got)
	}
}

func TestVerdictCache_DisabledWhenCapacityZero(t *testing.T) {
	cache := NewVerdictCache(0, 10*time.Minute)
	cache.Put(1, "hash1", CachedVerdict{Flagged: true})

	_, ok := cache.Get(1, "hash1")
	if ok {
		t.Fatal("expected no caching when capacity=0")
	}
}

func TestVerdictCache_Invalidate(t *testing.T) {
	cache := NewVerdictCache(100, 10*time.Minute)
	cache.Put(1, "hash1", CachedVerdict{Flagged: true})
	cache.Put(1, "hash2", CachedVerdict{Flagged: true})

	cache.Invalidate()

	_, ok := cache.Get(1, "hash1")
	if ok {
		t.Fatal("expected miss after invalidate")
	}
	_, ok = cache.Get(1, "hash2")
	if ok {
		t.Fatal("expected miss after invalidate")
	}
}
