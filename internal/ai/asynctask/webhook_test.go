package asynctask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestNormalizeWebhookURLRequiresAbsoluteHTTPS(t *testing.T) {
	got, err := normalizeWebhookURL("  https://hooks.example.com/task-events  ")
	if err != nil {
		t.Fatalf("valid webhook URL rejected: %v", err)
	}
	if got != "https://hooks.example.com/task-events" {
		t.Fatalf("normalized URL = %q", got)
	}

	for _, raw := range []string{
		"http://hooks.example.com/task-events",
		"/task-events",
		"https://user:pass@hooks.example.com/task-events",
		"https://hooks.example.com/task-events#fragment",
	} {
		if _, err := normalizeWebhookURL(raw); err == nil {
			t.Errorf("unsafe webhook URL %q was accepted", raw)
		}
	}
}

func TestWebhookAddressGuardRejectsNonPublicNetworks(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1",
		"169.254.10.1", "100.64.0.1", "0.0.0.1", "192.0.2.1",
		"198.18.0.1", "203.0.113.1", "224.0.0.1", "255.255.255.255",
		"::1", "fc00::1", "fec0::1", "fe80::1", "ff02::1", "2001:db8::1",
		"64:ff9b::7f00:1", "2002:7f00:1::", "::ffff:127.0.0.1",
	} {
		if isPublicWebhookAddr(netip.MustParseAddr(raw)) {
			t.Errorf("non-public address %s was accepted", raw)
		}
	}
	for _, raw := range []string{"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"} {
		if !isPublicWebhookAddr(netip.MustParseAddr(raw)) {
			t.Errorf("public address %s was rejected", raw)
		}
	}
}

func TestHTTPWebhookSenderIdentifiesUniHubAndPreservesBody(t *testing.T) {
	const taskID = "00000000-0000-0000-0000-000000000001"
	payload := []byte(`{"source":"UniHub","event":"task.completed","task_id":"` + taskID + `"}`)
	var gotBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read webhook body: %v", err)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "UniHub-Webhook/1.0" {
			t.Errorf("user agent = %q", got)
		}
		if got := r.Header.Get("X-UniHub-Signature"); got != "" {
			t.Errorf("unexpected signature header = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sender := newHTTPWebhookSender(server.Client())
	status, err := sender.Send(context.Background(), WebhookRequest{
		URL: server.URL, Payload: payload,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", status)
	}
	if string(gotBody) != string(payload) {
		t.Fatalf("body = %q, want %q", gotBody, payload)
	}
}

func TestProductionWebhookSenderRejectsPrivateTargets(t *testing.T) {
	sender := NewWebhookSender()
	_, err := sender.Send(context.Background(), WebhookRequest{
		URL: "https://127.0.0.1/hooks", Payload: []byte(`{}`),
	})
	if !errors.Is(err, ErrUnsafeWebhookTarget) {
		t.Fatalf("private target error = %v, want ErrUnsafeWebhookTarget", err)
	}
}

func TestWebhookRetryDelayMatchesContract(t *testing.T) {
	want := []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	for attempt, delay := range want {
		got, retry := webhookRetryDelay(attempt + 1)
		if !retry || got != delay {
			t.Errorf("attempt %d: delay=%s retry=%v, want %s true", attempt+1, got, retry, delay)
		}
	}
	if got, retry := webhookRetryDelay(6); retry || got != 0 {
		t.Fatalf("attempt 6: delay=%s retry=%v, want terminal", got, retry)
	}
}

func TestEngineDeliversTerminalWebhookEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 6})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	received := make(chan *http.Request, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read webhook: %v", err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		received <- r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	engine, err := New(Config{
		Workers: 1, PollInterval: 20 * time.Millisecond,
		WebhookWorkers: 1, WebhookPollInterval: 20 * time.Millisecond, WebhookLeaseTTL: time.Second,
	}, Deps{
		Pool: pool, Logger: zap.NewNop(),
		Subjects: SubjectResolverFunc(func(_ context.Context, ref SubjectRef) (identity.Subject, error) {
			return identity.Subject{AuthMethod: ref.AuthMethod, TenantID: ref.TenantID, APIKeyID: ref.APIKeyID}, nil
		}),
		WebhookSender: newHTTPWebhookSender(server.Client()),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	engine.Register(probeType, stubHandler{
		execute: func(context.Context, Task) (Result, error) {
			return Result{Status: domain.TaskCompleted, Output: []byte(`{"ok":true}`)}, nil
		},
	}, Options{})

	runCtx, cancel := context.WithCancel(context.Background())
	engine.Start(runCtx)
	t.Cleanup(func() {
		cancel()
		stopCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		engine.Stop(stopCtx)
	})

	created, err := engine.Submit(ctx, SubmitRequest{
		Subject: testSubject(), Type: probeType, Body: []byte(`{"prompt":"a cat"}`),
		WebhookURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case request := <-received:
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read captured body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload) != 3 || payload["source"] != "UniHub" ||
			payload["event"] != "task.completed" || payload["task_id"] != created.ID {
			t.Fatalf("payload = %#v", payload)
		}
		if request.Header.Get("User-Agent") != "UniHub-Webhook/1.0" {
			t.Fatalf("user agent = %q", request.Header.Get("User-Agent"))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook was not delivered")
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		var status string
		if err := pool.QueryRow(ctx,
			`SELECT status FROM ai_async_task_deliveries WHERE task_id = $1::uuid`, created.ID).Scan(&status); err != nil {
			t.Fatalf("read delivery status: %v", err)
		}
		if status == "delivered" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivery status = %q, want delivered", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWebhookHTTP500SchedulesTheNextAttempt(t *testing.T) {
	got := classifyWebhookResult(1, 6, http.StatusInternalServerError, nil)
	if got.Status != "pending" || got.StatusCode != http.StatusInternalServerError || got.RetryAfter != 10*time.Second {
		t.Fatalf("outcome = %+v, want pending after 10s", got)
	}
	if got.LastError != "webhook returned HTTP 500" {
		t.Fatalf("last error = %q", got.LastError)
	}
}

func TestWebhookPermanentResultsDoNotRetry(t *testing.T) {
	for _, tt := range []struct {
		name       string
		attempt    int
		statusCode int
		err        error
		wantStatus string
	}{
		{name: "success", attempt: 1, statusCode: http.StatusNoContent, wantStatus: "delivered"},
		{name: "gone", attempt: 1, statusCode: http.StatusGone, wantStatus: "failed"},
		{name: "unsafe target", attempt: 1, err: ErrUnsafeWebhookTarget, wantStatus: "failed"},
		{name: "attempts exhausted", attempt: 6, statusCode: http.StatusInternalServerError, wantStatus: "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyWebhookResult(tt.attempt, 6, tt.statusCode, tt.err)
			if got.Status != tt.wantStatus || got.RetryAfter != 0 {
				t.Fatalf("outcome = %+v, want status %s without retry", got, tt.wantStatus)
			}
		})
	}
}

func TestHTTPWebhookSenderDoesNotFollowRedirects(t *testing.T) {
	var finalHit atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			finalHit.Store(true)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()

	sender := newHTTPWebhookSender(server.Client())
	status, err := sender.Send(context.Background(), WebhookRequest{
		URL: server.URL + "/redirect", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if status != http.StatusFound || finalHit.Load() {
		t.Fatalf("status=%d finalHit=%v, redirect must not be followed", status, finalHit.Load())
	}
}
