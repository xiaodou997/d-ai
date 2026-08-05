package riskcontrol

import (
	"container/list"
	"sync"
	"time"
)

// CachedVerdict is the cached result of a full detection pass. It contains
// only the fields needed to short-circuit subsequent identical requests
// — not the full DetectResult (which includes transient fields like
// UpstreamLatencyMs that are meaningless on a cache hit).
type CachedVerdict struct {
	Flagged         bool
	MatchedKeyword  string
	HitLayer        string
	HighestCategory string
	HighestScore    *float64
}

// cacheEntry is one slot in the LRU cache.
type cacheEntry struct {
	key       string
	revision  int64
	verdict   CachedVerdict
	createdAt time.Time
}

// VerdictCache is a bounded LRU cache keyed by (configRevision, textHash).
// Its purpose is to skip redundant full detection passes when the same
// text is submitted repeatedly (retry, spam, scripted loops). Cache hits
// do NOT write moderation logs and do NOT increment the violation count —
// repeated submission of the same text is one behavior, not N.
type VerdictCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element
	lru      *list.List
	capacity int
	ttl      time.Duration
}

// NewVerdictCache creates a cache with the given capacity and TTL.
// capacity=0 disables caching (all Get calls return false).
func NewVerdictCache(capacity int, ttl time.Duration) *VerdictCache {
	if capacity <= 0 {
		return &VerdictCache{}
	}
	return &VerdictCache{
		items:    make(map[string]*list.Element, capacity),
		lru:      list.New(),
		capacity: capacity,
		ttl:      ttl,
	}
}

// cacheKey produces the composite key from revision and text hash.
func cacheKey(revision int64, textHash string) string {
	// Fast string concat without fmt.Sprintf for hot-path performance.
	return string(append(append([]byte{}, intToBytes(revision)...), ':')) + textHash
}

// intToBytes converts an int64 to its decimal string bytes.
func intToBytes(n int64) []byte {
	if n == 0 {
		return []byte{'0'}
	}
	var buf [20]byte
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return buf[pos:]
}

// Get returns the cached verdict for the given revision and text hash.
// Returns false if the cache is disabled, the entry is missing, the
// revision doesn't match, or the entry has expired.
func (c *VerdictCache) Get(revision int64, textHash string) (CachedVerdict, bool) {
	if c == nil || c.capacity <= 0 || textHash == "" {
		return CachedVerdict{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(revision, textHash)
	elem, ok := c.items[key]
	if !ok {
		return CachedVerdict{}, false
	}

	entry := elem.Value.(*cacheEntry)

	// Revision mismatch → stale entry from an older config; treat as miss.
	if entry.revision != revision {
		c.removeElement(elem)
		return CachedVerdict{}, false
	}

	// TTL expiry → stale entry; treat as miss.
	if c.ttl > 0 && time.Since(entry.createdAt) > c.ttl {
		c.removeElement(elem)
		return CachedVerdict{}, false
	}

	// Move to front (most recently used).
	c.lru.MoveToFront(elem)
	return entry.verdict, true
}

// Put stores a verdict in the cache. If the cache is at capacity, the
// least recently used entry is evicted first. No-op if caching is disabled.
func (c *VerdictCache) Put(revision int64, textHash string, verdict CachedVerdict) {
	if c == nil || c.capacity <= 0 || textHash == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(revision, textHash)

	// Update existing entry.
	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.verdict = verdict
		entry.revision = revision
		entry.createdAt = time.Now()
		c.lru.MoveToFront(elem)
		return
	}

	// Evict LRU if at capacity.
	if c.lru.Len() >= c.capacity {
		oldest := c.lru.Back()
		if oldest != nil {
			c.removeElement(oldest)
		}
	}

	entry := &cacheEntry{
		key:       key,
		revision:  revision,
		verdict:   verdict,
		createdAt: time.Now(),
	}
	elem := c.lru.PushFront(entry)
	c.items[key] = elem
}

// removeElement removes a list element from both the list and the map.
func (c *VerdictCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
	c.lru.Remove(elem)
}

// Invalidate clears all cached entries. Called when config changes that
// don't bump the revision (shouldn't happen in practice, but available
// for manual invalidation).
func (c *VerdictCache) Invalidate() {
	if c == nil || c.capacity <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.capacity)
	c.lru.Init()
}
