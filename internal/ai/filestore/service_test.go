package filestore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/ai/testsupport"
)

func TestPostgresDeleteExpiredRemovesUnexpiredAssetLinks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("filestore test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		assetID    = "00000000-0000-0000-0000-000000000101"
		storageKey = "expired/asset.bin"
	)
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO file_assets (id, storage_key, content_type, size_bytes, created_at, expires_at)
		VALUES ($1::uuid, $2, 'application/octet-stream', 1, $3::timestamptz, $3::timestamptz - interval '1 minute')
	`, assetID, storageKey, now); err != nil {
		t.Fatalf("seed expired asset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO file_access_links (token_hash, asset_id, expires_at)
		VALUES (decode('01', 'hex'), $1::uuid, $2::timestamptz + interval '1 hour')
	`, assetID, now); err != nil {
		t.Fatalf("seed unexpired asset link: %v", err)
	}

	repo := &postgresRepository{pool: pool}
	keys, deletedLinks, err := repo.deleteExpired(ctx, now, 10)
	if err != nil {
		t.Fatalf("deleteExpired: %v", err)
	}
	if len(keys) != 1 || keys[0] != storageKey || deletedLinks != 1 {
		t.Fatalf("cleanup = keys %v links %d, want [%s] and 1", keys, deletedLinks, storageKey)
	}

	var assetsAfter, linksAfter int
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM file_assets WHERE id = $1::uuid),
		  (SELECT count(*) FROM file_access_links WHERE asset_id = $1::uuid)
	`, assetID).Scan(&assetsAfter, &linksAfter); err != nil {
		t.Fatalf("count rows after cleanup: %v", err)
	}
	if assetsAfter != 0 || linksAfter != 0 {
		t.Fatalf("rows after cleanup = assets %d links %d, want both zero", assetsAfter, linksAfter)
	}
}

func TestPublishIssueURLAndServeContent(t *testing.T) {
	repo := newMemoryRepository()
	store, err := newService(repo, Config{
		StorageDir:    t.TempDir(),
		AssetTTL:      24 * time.Hour,
		URLTTL:        2 * time.Hour,
		PublicBaseURL: "https://api.example.test",
	})
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	asset, err := store.PublishBytes(context.Background(), []byte("image bytes"), "image/png")
	if err != nil {
		t.Fatalf("PublishBytes: %v", err)
	}
	if !strings.HasPrefix(asset.Ref, "media://") {
		t.Fatalf("asset ref = %q", asset.Ref)
	}
	link, err := store.IssueURL(context.Background(), asset.Ref)
	if err != nil {
		t.Fatalf("IssueURL: %v", err)
	}
	if !strings.HasPrefix(link.URL, "https://api.example.test/v1/files/content/") {
		t.Fatalf("link URL = %q", link.URL)
	}

	router := chi.NewRouter()
	store.Routes(router)
	request := httptest.NewRequest(http.MethodGet, link.URL, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("content type = %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache control = %q", recorder.Header().Get("Cache-Control"))
	}
	if recorder.Body.String() != "image bytes" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestNewServiceDefaultsAssetAndURLTTLTo24Hours(t *testing.T) {
	store, err := newService(newMemoryRepository(), Config{
		StorageDir:    t.TempDir(),
		PublicBaseURL: "https://api.example.test",
	})
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	if store.assetTTL != 24*time.Hour || store.urlTTL != 24*time.Hour {
		t.Fatalf("TTLs = asset %s, url %s; want both 24h", store.assetTTL, store.urlTTL)
	}
}

func TestNormalizeImageResponseKeepsUpstreamURLAndPublishesInlineData(t *testing.T) {
	repo := newMemoryRepository()
	store, err := newService(repo, Config{
		StorageDir:    t.TempDir(),
		AssetTTL:      time.Hour,
		URLTTL:        2 * time.Hour,
		PublicBaseURL: "https://api.example.test",
	})
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	body := []byte(`{"data":[{"url":"https://upstream.example.test/image.png"},{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/lAAAAABJRU5ErkJggg=="}]}`)
	normalized, err := store.NormalizeImageResponse(context.Background(), body, "url")
	if err != nil {
		t.Fatalf("NormalizeImageResponse: %v", err)
	}
	text := string(normalized)
	if !strings.Contains(text, "https://upstream.example.test/image.png") {
		t.Fatalf("upstream URL was not preserved: %s", text)
	}
	if !strings.Contains(text, "https://api.example.test/v1/files/content/") {
		t.Fatalf("inline image was not published: %s", text)
	}
	if !strings.Contains(text, `"asset_ref":"media://`) {
		t.Fatalf("published image has no asset reference: %s", text)
	}
}

func TestCleanupExpiresAssetAndItsLinks(t *testing.T) {
	repo := newMemoryRepository()
	store, err := newService(repo, Config{
		StorageDir:    t.TempDir(),
		AssetTTL:      time.Nanosecond,
		URLTTL:        time.Hour,
		PublicBaseURL: "https://api.example.test",
	})
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	asset, err := store.PublishBytes(context.Background(), []byte("content"), "text/plain")
	if err != nil {
		t.Fatalf("PublishBytes: %v", err)
	}
	time.Sleep(time.Millisecond)
	result, err := store.CleanupExpired(context.Background(), 10)
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if result.DeletedAssets != 1 || result.DeletedFiles != 1 {
		t.Fatalf("cleanup result = %+v", result)
	}
	if _, err := store.IssueURL(context.Background(), asset.Ref); !errors.Is(err, errMemoryNotFound) {
		t.Fatalf("IssueURL after cleanup error = %v", err)
	}
}

var errMemoryNotFound = errors.New("not found")

type memoryRepository struct {
	mu     sync.Mutex
	assets map[string]storedAsset
	links  map[string]memoryLink
}

type memoryLink struct {
	assetID   string
	expiresAt time.Time
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{assets: map[string]storedAsset{}, links: map[string]memoryLink{}}
}

func (r *memoryRepository) createAsset(_ context.Context, asset storedAsset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[asset.id] = asset
	return nil
}

func (r *memoryRepository) assetByID(_ context.Context, id string) (storedAsset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	asset, ok := r.assets[id]
	if !ok {
		return storedAsset{}, errMemoryNotFound
	}
	return asset, nil
}

func (r *memoryRepository) createLink(_ context.Context, hash []byte, assetID string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.links[string(hash)] = memoryLink{assetID: assetID, expiresAt: expiresAt}
	return nil
}

func (r *memoryRepository) assetByLink(_ context.Context, hash []byte, now time.Time) (storedAsset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	link, ok := r.links[string(hash)]
	if !ok || !link.expiresAt.After(now) {
		return storedAsset{}, errMemoryNotFound
	}
	asset, ok := r.assets[link.assetID]
	if !ok || !asset.expiresAt.After(now) {
		return storedAsset{}, errMemoryNotFound
	}
	return asset, nil
}

func (r *memoryRepository) deleteExpired(_ context.Context, now time.Time, limit int) ([]string, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	deletedLinks := 0
	for hash, link := range r.links {
		if !link.expiresAt.After(now) {
			delete(r.links, hash)
			deletedLinks++
		}
	}
	keys := make([]string, 0, limit)
	for id, asset := range r.assets {
		if len(keys) == limit || asset.expiresAt.After(now) {
			continue
		}
		keys = append(keys, asset.storageKey)
		delete(r.assets, id)
		for hash, link := range r.links {
			if link.assetID == id {
				delete(r.links, hash)
			}
		}
	}
	return keys, deletedLinks, nil
}
