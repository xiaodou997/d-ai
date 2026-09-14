package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestDebugScrubsSecretsAndInlineMedia(t *testing.T) {
	var value any
	if err := json.Unmarshal([]byte(`{"authorization":"secret-auth","apiKey":"secret-key","source":{"type":"base64","data":"media-a"},"output":[{"type":"image_generation_call","result":"media-b"}],"content":[{"inlineData":{"data":"media-c"}},{"image_url":"data:image/png;base64,media-d"}],"text":"kept sk-123456789012345678901234", "array":["data:image/png;base64,media-e"]}`), &value); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(scrubDebug(value))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-auth", "secret-key", "media-a", "media-b", "media-c", "media-d", "media-e", "sk-123456789012345678901234"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("sensitive content retained: %s", secret)
		}
	}
	if !strings.Contains(string(raw), "kept") {
		t.Fatal("ordinary text lost")
	}
}

func TestDebugRequiresScopeAndRemainsOptional(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	store := NewRequestStore(pool, nil, nil)
	if _, err = store.StartDebug(ctx, domain.DebugSessionInput{}, "admin"); err == nil {
		t.Fatal("unscoped recording accepted")
	}
	if _, err = store.StartDebug(ctx, domain.DebugSessionInput{TenantID: "t", Hours: 25}, "admin"); err == nil {
		t.Fatal("unbounded recording accepted")
	}
	session, err := store.StartDebug(ctx, domain.DebugSessionInput{TenantID: "t"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := usageCompletionRequest("debug-request", "t", "u")
	if err = store.Execute(ctx, req); err != nil {
		t.Fatal(err)
	}
	defer req.RecordLeaseStop()
	if !req.CaptureBody || req.RecordDebugSessionID != session.ID {
		t.Fatal("matching scope not captured")
	}
	req.AuditResponseMessage = []byte(`{"text":"hello","access_token":"secret"}`)
	if err = store.Log(ctx, req); err != nil {
		t.Fatal(err)
	}
	store.CaptureDebug(ctx, req)
	raw, err := store.DebugPayload(ctx, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || !strings.Contains(string(raw), "hello") {
		t.Fatalf("incorrect debug content: %s", raw)
	}
	other := usageCompletionRequest("debug-other", "other", "u")
	if err = store.Execute(ctx, other); err != nil {
		t.Fatal(err)
	}
	defer other.RecordLeaseStop()
	if other.CaptureBody {
		t.Fatal("scope leaked across tenants")
	}
}
