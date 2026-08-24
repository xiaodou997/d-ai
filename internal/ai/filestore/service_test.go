package filestore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/weborigin"
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
	claims, deletedLinks, err := repo.claimExpired(ctx, now, now.Add(time.Minute), 10, "filestore-test-owner")
	if err != nil {
		t.Fatalf("claimExpired: %v", err)
	}
	if len(claims) != 1 || claims[0].storageKey != storageKey || deletedLinks != 0 {
		t.Fatalf("claims = %+v links %d, want [%s] and 0", claims, deletedLinks, storageKey)
	}
	ids := []string{claims[0].id}
	deletedAssets, finalizedLinks, err := repo.finalizeExpired(ctx, ids, "filestore-test-owner")
	if err != nil {
		t.Fatalf("finalizeExpired: %v", err)
	}
	if deletedAssets != 1 || finalizedLinks != 1 {
		t.Fatalf("finalize = assets %d links %d, want 1 and 1", deletedAssets, finalizedLinks)
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

func TestCleanupExpiredSerializesReplicasAndRetainsMetadataOnFileFailure(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("filestore test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	storageDir := t.TempDir()
	first, err := New(pool, Config{StorageDir: storageDir})
	if err != nil {
		t.Fatalf("new first filestore: %v", err)
	}
	second, err := New(pool, Config{StorageDir: storageDir})
	if err != nil {
		t.Fatalf("new second filestore: %v", err)
	}
	const (
		assetID    = "00000000-0000-0000-0000-000000000102"
		storageKey = "replica-cleanup.bin"
	)
	filePath := filepath.Join(storageDir, storageKey)
	if err := os.WriteFile(filePath, []byte("expired"), 0o600); err != nil {
		t.Fatalf("write expired file: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO file_assets (id, storage_key, content_type, size_bytes, created_at, expires_at)
		VALUES ($1::uuid, $2, 'application/octet-stream', 7, now() - interval '2 minutes', now() - interval '1 minute')
	`, assetID, storageKey); err != nil {
		t.Fatalf("seed expired asset: %v", err)
	}

	start := make(chan struct{})
	results := make(chan CleanupResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range []*Service{first, second} {
		wg.Add(1)
		go func(store *Service) {
			defer wg.Done()
			<-start
			result, err := store.CleanupExpired(ctx, 10)
			results <- result
			errs <- err
		}(store)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	deletedAssets, deletedFiles := 0, 0
	for result := range results {
		deletedAssets += result.DeletedAssets
		deletedFiles += result.DeletedFiles
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent file cleanup: %v", err)
		}
	}
	if deletedAssets != 1 || deletedFiles != 1 {
		t.Fatalf("concurrent cleanup totals = assets:%d files:%d, want 1 and 1", deletedAssets, deletedFiles)
	}
	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired file stat error = %v, want not exist", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM file_assets WHERE id = $1::uuid`, assetID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("expired asset rows = %d, want 0", remaining)
	}

	const failedAssetID = "00000000-0000-0000-0000-000000000103"
	blockedPath := filepath.Join(storageDir, "blocked")
	if err := os.Mkdir(blockedPath, 0o700); err != nil {
		t.Fatalf("create blocked cleanup directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockedPath, "keep"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("seed blocked directory: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO file_assets (id, storage_key, content_type, size_bytes, created_at, expires_at)
		VALUES ($1::uuid, 'blocked', 'application/octet-stream', 4, now() - interval '2 minutes', now() - interval '1 minute')
	`, failedAssetID); err != nil {
		t.Fatalf("seed blocked asset: %v", err)
	}
	if _, err := first.CleanupExpired(ctx, 10); err == nil {
		t.Fatal("cleanup of non-empty directory unexpectedly succeeded")
	}
	var owner string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(cleanup_owner, '') FROM file_assets WHERE id = $1::uuid`, failedAssetID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "" {
		t.Fatalf("failed cleanup owner = %q, want released", owner)
	}
	if err := os.Remove(filepath.Join(blockedPath, "keep")); err != nil {
		t.Fatalf("remove blocked fixture file: %v", err)
	}
	if err := os.Remove(blockedPath); err != nil {
		t.Fatalf("remove blocked fixture directory: %v", err)
	}
	if result, err := first.CleanupExpired(ctx, 10); err != nil || result.DeletedAssets != 1 {
		t.Fatalf("retry blocked cleanup = %+v err:%v, want one asset", result, err)
	}
}

func TestPublishIssueURLAndServeContent(t *testing.T) {
	repo := newMemoryRepository()
	store, err := newService(repo, Config{
		StorageDir: t.TempDir(),
		AssetTTL:   24 * time.Hour,
		URLTTL:     2 * time.Hour,
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
	if !strings.HasPrefix(link.URL, "/v1/files/content/") {
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

func TestIssueURLUsesRequestOriginWhenAvailable(t *testing.T) {
	store, err := newService(newMemoryRepository(), Config{StorageDir: t.TempDir()})
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	asset, err := store.PublishBytes(context.Background(), []byte("content"), "text/plain")
	if err != nil {
		t.Fatalf("PublishBytes: %v", err)
	}
	link, err := store.IssueURL(weborigin.WithOrigin(context.Background(), "https://uadmin.example.test"), asset.Ref)
	if err != nil {
		t.Fatalf("IssueURL: %v", err)
	}
	if !strings.HasPrefix(link.URL, "https://uadmin.example.test/v1/files/content/") {
		t.Fatalf("link URL = %q", link.URL)
	}
}

func TestNewServiceDefaultsAssetAndURLTTLTo24Hours(t *testing.T) {
	store, err := newService(newMemoryRepository(), Config{
		StorageDir: t.TempDir(),
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
		StorageDir: t.TempDir(),
		AssetTTL:   time.Hour,
		URLTTL:     2 * time.Hour,
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
	if !strings.Contains(text, "/v1/files/content/") {
		t.Fatalf("inline image was not published: %s", text)
	}
	if !strings.Contains(text, `"asset_ref":"media://`) {
		t.Fatalf("published image has no asset reference: %s", text)
	}
}

func TestCleanupExpiresAssetAndItsLinks(t *testing.T) {
	repo := newMemoryRepository()
	store, err := newService(repo, Config{
		StorageDir: t.TempDir(),
		AssetTTL:   time.Nanosecond,
		URLTTL:     time.Hour,
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

func (r *memoryRepository) claimExpired(_ context.Context, now, leaseUntil time.Time, limit int, owner string) ([]storedAsset, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	deletedLinks := 0
	for hash, link := range r.links {
		if !link.expiresAt.After(now) {
			delete(r.links, hash)
			deletedLinks++
		}
	}
	claims := make([]storedAsset, 0, limit)
	for id, asset := range r.assets {
		if len(claims) == limit || asset.expiresAt.After(now) {
			continue
		}
		if asset.cleanupOwner != "" && asset.cleanupLeaseUntil.After(now) {
			continue
		}
		asset.cleanupOwner = owner
		asset.cleanupLeaseUntil = leaseUntil
		r.assets[id] = asset
		claims = append(claims, asset)
	}
	return claims, deletedLinks, nil
}

func (r *memoryRepository) renewExpired(_ context.Context, ids []string, owner string, leaseUntil time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for _, id := range ids {
		asset, ok := r.assets[id]
		if !ok || asset.cleanupOwner != owner || !asset.cleanupLeaseUntil.After(now) {
			return false, nil
		}
	}
	for _, id := range ids {
		asset := r.assets[id]
		asset.cleanupLeaseUntil = leaseUntil
		r.assets[id] = asset
	}
	return true, nil
}

func (r *memoryRepository) releaseExpired(_ context.Context, ids []string, owner string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		asset, ok := r.assets[id]
		if ok && asset.cleanupOwner == owner {
			asset.cleanupOwner = ""
			asset.cleanupLeaseUntil = time.Time{}
			r.assets[id] = asset
		}
	}
	return nil
}

func (r *memoryRepository) finalizeExpired(_ context.Context, ids []string, owner string) (int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	assetsDeleted, linksDeleted := 0, 0
	for _, id := range ids {
		asset, ok := r.assets[id]
		if !ok || asset.cleanupOwner != owner {
			continue
		}
		delete(r.assets, id)
		assetsDeleted++
		for hash, link := range r.links {
			if link.assetID == id {
				delete(r.links, hash)
				linksDeleted++
			}
		}
	}
	return assetsDeleted, linksDeleted, nil
}
