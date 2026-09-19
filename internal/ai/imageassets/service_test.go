package imageassets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/weborigin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestImageBytesDecodesBase64PNG(t *testing.T) {
	data := testPNG(t)
	item := responseImageItem{B64JSON: encodeStdBase64(data)}
	svc := New(Config{}, nil)

	got, contentType, err := svc.imageBytes(context.Background(), item)
	if err != nil {
		t.Fatalf("imageBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("decoded image bytes mismatch")
	}
	if contentType != "image/png" {
		t.Fatalf("contentType = %q, want image/png", contentType)
	}
}

func encodeStdBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func TestImageBytesDownloadsURL(t *testing.T) {
	data := testPNG(t)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.String(); got != "https://images.example/image.png" {
			t.Fatalf("request URL = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(data)),
			Request:    req,
		}, nil
	})}
	svc := New(Config{}, client)
	got, contentType, err := svc.imageBytes(context.Background(), responseImageItem{URL: "https://images.example/image.png"})
	if err != nil {
		t.Fatalf("imageBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded image bytes mismatch")
	}
	if contentType != "image/png" {
		t.Fatalf("contentType = %q, want image/png", contentType)
	}
}

func TestImageBytesDecodesDataURLPlacedInURLField(t *testing.T) {
	data := testPNG(t)
	item := responseImageItem{
		URL: "data:image/png;base64," + encodeStdBase64(data),
	}
	svc := New(Config{}, nil)

	got, contentType, err := svc.imageBytes(context.Background(), item)
	if err != nil {
		t.Fatalf("imageBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("decoded data url bytes mismatch")
	}
	if contentType != "image/png" {
		t.Fatalf("contentType = %q, want image/png", contentType)
	}
}

func TestImageBytesDecodesRawBase64PlacedInURLField(t *testing.T) {
	data := testPNG(t)
	item := responseImageItem{URL: encodeStdBase64(data)}
	svc := New(Config{}, nil)

	got, contentType, err := svc.imageBytes(context.Background(), item)
	if err != nil {
		t.Fatalf("imageBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("decoded base64 bytes mismatch")
	}
	if contentType != "image/png" {
		t.Fatalf("contentType = %q, want image/png", contentType)
	}
}

func TestValidateDownloadURLRejectsDataAndLocalhost(t *testing.T) {
	for _, rawURL := range []string{
		"data:image/png;base64,abc",
		"http://localhost/image.png",
		"http://127.0.0.1/image.png",
		"http://[::1]/image.png",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if err := ValidateDownloadURL(rawURL); err == nil {
				t.Fatal("expected url to be rejected")
			}
		})
	}
}

func TestStoreOpenAIImagesResponseRejectsPrivateProviderURL(t *testing.T) {
	svc := New(Config{StorageDir: t.TempDir()}, nil)
	body := []byte(`{"data":[{"url":"http://127.0.0.1/private.png"}]}`)
	_, err := svc.StoreOpenAIImagesResponse(context.Background(), Owner{TenantID: "tenant-1"}, "task-1", body)
	if err == nil {
		t.Fatal("provider-returned private image URL was accepted")
	}
}

func TestImageAddressGuardRejectsNonPublicNetworks(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1",
		"169.254.10.1", "100.64.0.1", "0.0.0.1", "192.0.2.1",
		"198.18.0.1", "203.0.113.1", "224.0.0.1", "255.255.255.255",
		"::1", "fc00::1", "fec0::1", "fe80::1", "ff02::1", "2001:db8::1",
		"64:ff9b::7f00:1", "2002:7f00:1::", "::ffff:127.0.0.1",
	} {
		if isPublicImageDownloadAddr(netip.MustParseAddr(raw)) {
			t.Errorf("non-public address %s was accepted", raw)
		}
	}
	for _, raw := range []string{"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"} {
		if !isPublicImageDownloadAddr(netip.MustParseAddr(raw)) {
			t.Errorf("public address %s was rejected", raw)
		}
	}
}

func TestGuardedImageDialContextPinsResolvedAddress(t *testing.T) {
	var dialed string
	stop := errors.New("stop after address capture")
	dial := guardedImageDialContext(
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = address
			return nil, stop
		},
	)
	_, err := dial(context.Background(), "tcp", "images.example:443")
	if !errors.Is(err, stop) {
		t.Fatalf("dial error = %v, want sentinel", err)
	}
	if dialed != "93.184.216.34:443" {
		t.Fatalf("dialed address = %q, want vetted IP", dialed)
	}
}

func TestGuardedImageDialContextRejectsUnsafeDNSAnswerBeforeDial(t *testing.T) {
	dialCalled := false
	dial := guardedImageDialContext(
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("127.0.0.1"),
			}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("unexpected dial")
		},
	)
	if _, err := dial(context.Background(), "tcp", "images.example:443"); err == nil {
		t.Fatal("mixed public/private DNS answer was accepted")
	}
	if dialCalled {
		t.Fatal("dial happened before all resolved addresses were validated")
	}
}

func TestNewDefaults(t *testing.T) {
	svc := New(Config{}, nil)
	if svc.storageDir != "images" {
		t.Fatalf("storageDir = %q, want images", svc.storageDir)
	}
	if svc.retention != 24*time.Hour {
		t.Fatalf("retention = %s, want 24h", svc.retention)
	}
	if svc.publicBasePath != "/runtime/v1/images/tasks" {
		t.Fatalf("publicBasePath = %q", svc.publicBasePath)
	}
}

func TestSanitizeAssetURLRejectsInlineImagePayload(t *testing.T) {
	data := testPNG(t)
	values := []string{
		"data:image/png;base64," + encodeStdBase64(data),
		encodeStdBase64(data),
	}
	for _, value := range values {
		if got := SanitizeAssetURL(value); got != "" {
			t.Fatalf("SanitizeAssetURL(%q) = %q, want empty", value[:minLen(len(value), 32)], got)
		}
	}
	if got := SanitizeAssetURL("/runtime/v1/images/tasks/task-1/assets/0/preview?key=test"); got == "" {
		t.Fatal("want runtime asset path to be preserved")
	}
}

func TestNormalizeOpenAIResponseReturnsBase64ForInlineDataInURLField(t *testing.T) {
	data := testPNG(t)
	svc := New(Config{StorageDir: t.TempDir()}, nil)
	body := []byte(`{"data":[{"url":"` + encodeStdBase64(data) + `"}]}`)

	normalized, err := svc.NormalizeOpenAIResponse(context.Background(), body, "b64_json")
	if err != nil {
		t.Fatalf("NormalizeOpenAIResponse: %v", err)
	}
	var response struct {
		Data []responseImageItem `json:"data"`
	}
	if err := json.Unmarshal(normalized, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != encodeStdBase64(data) || response.Data[0].URL != "" {
		t.Fatalf("unexpected normalized data: %+v", response.Data)
	}
}

func TestNormalizeOpenAIResponseStoresURLForInlineDataInURLField(t *testing.T) {
	data := testPNG(t)
	svc := New(Config{StorageDir: t.TempDir(), PublicBasePath: "/runtime/v1/images/assets"}, nil)
	body := []byte(`{"data":[{"url":"` + encodeStdBase64(data) + `"}]}`)

	normalized, err := svc.NormalizeOpenAIResponse(context.Background(), body, "url")
	if err != nil {
		t.Fatalf("NormalizeOpenAIResponse: %v", err)
	}
	var response struct {
		Data []responseImageItem `json:"data"`
	}
	if err := json.Unmarshal(normalized, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != "" || !strings.HasPrefix(response.Data[0].URL, "/runtime/v1/images/assets/") {
		t.Fatalf("normalized data = %+v", response.Data)
	}
}

func TestNormalizeOpenAIResponseCreatesPlatformURLForBase64(t *testing.T) {
	data := testPNG(t)
	svc := New(Config{StorageDir: t.TempDir()}, nil)
	body := []byte(`{"data":[{"b64_json":"` + encodeStdBase64(data) + `"}]}`)

	normalized, err := svc.NormalizeOpenAIResponse(context.Background(), body, "url")
	if err != nil {
		t.Fatalf("NormalizeOpenAIResponse: %v", err)
	}
	var response struct {
		Data []responseImageItem `json:"data"`
	}
	if err := json.Unmarshal(normalized, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != "" || !strings.Contains(response.Data[0].URL, "/assets/") {
		t.Fatalf("unexpected normalized data: %+v", response.Data)
	}
}

func TestNormalizeOpenAIResponseUsesTrustedPublicOrigin(t *testing.T) {
	data := testPNG(t)
	svc := New(Config{StorageDir: t.TempDir()}, nil)
	ctx := weborigin.WithOrigin(context.Background(), "https://portal.example.test")
	body := []byte(`{"data":[{"b64_json":"` + encodeStdBase64(data) + `"}]}`)

	normalized, err := svc.NormalizeOpenAIResponse(ctx, body, "url")
	if err != nil {
		t.Fatalf("NormalizeOpenAIResponse: %v", err)
	}
	var response struct {
		Data []responseImageItem `json:"data"`
	}
	if err := json.Unmarshal(normalized, &response); err != nil || len(response.Data) != 1 {
		t.Fatalf("normalized response = %s, err = %v", normalized, err)
	}
	if !strings.HasPrefix(response.Data[0].URL, "https://portal.example.test/runtime/v1/images/assets/") {
		t.Fatalf("asset URL = %q", response.Data[0].URL)
	}
}

func TestNormalizeOpenAIResponseRejectsMalformedImagePayload(t *testing.T) {
	svc := New(Config{StorageDir: t.TempDir()}, nil)
	for _, body := range [][]byte{
		[]byte(`{"data":[{"url":"not-an-image"}]}`),
		[]byte(`{"data":[]}`),
		[]byte(`{"unexpected":true}`),
	} {
		if _, err := svc.NormalizeOpenAIResponse(context.Background(), body, "url"); err == nil {
			t.Fatalf("NormalizeOpenAIResponse(%s) unexpectedly succeeded", body)
		}
	}
}

func TestEphemeralAssetRequiresMatchingCapabilityKey(t *testing.T) {
	data := testPNG(t)
	svc := New(Config{StorageDir: t.TempDir()}, nil)
	rawURL, err := svc.storeEphemeralImage(context.Background(), data, "image/png")
	if err != nil {
		t.Fatalf("storeEphemeralImage: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	asset, err := svc.EphemeralAsset(path.Base(parsed.Path), parsed.Query().Get("key"))
	if err != nil {
		t.Fatalf("EphemeralAsset: %v", err)
	}
	got, err := os.ReadFile(asset.Path)
	if err != nil {
		t.Fatalf("read asset: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("ephemeral asset bytes mismatch")
	}
	if _, err := svc.EphemeralAsset(path.Base(parsed.Path), "wrong"); err == nil {
		t.Fatal("expected mismatched key to be rejected")
	}
}

func TestCleanupExpiredLeavesTaskAssetsForEngineExpirer(t *testing.T) {
	storageDir := t.TempDir()
	svc := New(Config{StorageDir: storageDir, Retention: time.Hour}, nil)
	taskAsset := filepath.Join(storageDir, "tasks", "task-1", "0_preview.webp")
	temporary := filepath.Join(storageDir, "_tmp", "upload.png")
	ephemeral := filepath.Join(storageDir, "ephemeral", "asset.webp")
	for _, path := range []string{taskAsset, temporary, ephemeral} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create asset directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write asset fixture: %v", err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, path := range []string{taskAsset, temporary, ephemeral} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("age asset fixture: %v", err)
		}
	}

	result, err := svc.CleanupExpired(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if result.DeletedFiles != 2 {
		t.Fatalf("deleted files = %d, want 2 temporary assets", result.DeletedFiles)
	}
	if _, err := os.Stat(taskAsset); err != nil {
		t.Fatalf("task asset should be owned by the engine expirer: %v", err)
	}

	deleted, err := svc.DeleteTaskAssets("task-1")
	if err != nil {
		t.Fatalf("DeleteTaskAssets: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted task files = %d, want 1", deleted)
	}
	if _, err := os.Stat(taskAsset); !os.IsNotExist(err) {
		t.Fatalf("task asset still exists after DeleteTaskAssets: %v", err)
	}
}

func TestCleanupExpiredCreatesMissingStorageDirectory(t *testing.T) {
	storageDir := filepath.Join(t.TempDir(), "images")
	svc := New(Config{StorageDir: storageDir}, nil)

	if _, err := svc.CleanupExpired(context.Background()); err != nil {
		t.Fatalf("CleanupExpired with missing storage directory: %v", err)
	}
	if info, err := os.Stat(storageDir); err != nil || !info.IsDir() {
		t.Fatalf("storage directory was not created: info=%v err=%v", info, err)
	}
}

func TestCleanupExpiredScansOrphanTaskAssetsOnlyWithRetainer(t *testing.T) {
	storageDir := t.TempDir()
	svc := New(Config{StorageDir: storageDir, Retention: time.Hour}, nil)
	svc.SetTaskRetainer(func(_ context.Context, taskID string) (bool, error) {
		return taskID == "live-task", nil
	})
	paths := []string{
		filepath.Join(storageDir, "tasks", "live-task", "0_preview.webp"),
		filepath.Join(storageDir, "tasks", "orphan-task", "0_preview.webp"),
		filepath.Join(storageDir, "_tmp", "old-upload.png"),
		filepath.Join(storageDir, "ephemeral", "old-asset.webp"),
	}
	for _, filePath := range paths {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("create image fixture directory: %v", err)
		}
		if err := os.WriteFile(filePath, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write image fixture: %v", err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, filePath := range paths {
		if err := os.Chtimes(filePath, old, old); err != nil {
			t.Fatalf("age image fixture: %v", err)
		}
	}
	result, err := svc.CleanupExpired(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if result.DeletedFiles != 3 {
		t.Fatalf("deleted files = %d, want 3 (temp, ephemeral, orphan task)", result.DeletedFiles)
	}
	if _, err := os.Stat(filepath.Join(storageDir, "tasks", "live-task", "0_preview.webp")); err != nil {
		t.Fatalf("retained task asset removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageDir, "tasks", "orphan-task")); !os.IsNotExist(err) {
		t.Fatalf("orphan task directory still exists: %v", err)
	}
}

func TestCleanupExpiredUsesReclaimableFilesystemLease(t *testing.T) {
	storageDir := t.TempDir()
	first := New(Config{StorageDir: storageDir}, nil)
	second := New(Config{StorageDir: storageDir}, nil)
	release, err := first.acquireCleanupLease(time.Now().UTC())
	if err != nil {
		t.Fatalf("acquire first cleanup lease: %v", err)
	}
	defer release()
	if _, err := second.CleanupExpired(context.Background()); !errors.Is(err, ErrCleanupAlreadyRunning) {
		t.Fatalf("second cleanup error = %v, want ErrCleanupAlreadyRunning", err)
	}

	leasePath := filepath.Join(storageDir, imageCleanupLeaseName)
	if err := os.WriteFile(leasePath, []byte(`{"owner":"dead-worker","expires_at":"2000-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("write expired lease: %v", err)
	}
	if _, err := first.acquireCleanupLease(time.Now().UTC()); err != nil {
		t.Fatalf("reclaim expired cleanup lease: %v", err)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
