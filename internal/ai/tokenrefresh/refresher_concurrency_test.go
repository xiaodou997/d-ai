package tokenrefresh

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestRefreshEntryPointsAndProcessesShareOneRotation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("refresh database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	store := pgadapter.NewOAuthCredentialStore(pool, "0123456789abcdef0123456789abcdef")
	poolID, err := store.CreatePool(ctx, domain.CredentialPoolCreate{Name: "refresh-race", FixedProviderType: domain.FixedProviderCodex, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.Create(ctx, poolID, domain.OAuthCredentialCreate{Name: "account", ProviderType: domain.FixedProviderCodex, AccessToken: "access-v1", RefreshToken: "refresh-v1"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"access-v2","refresh_token":"refresh-v2","expires_in":3600}`))}, nil
	})}
	first, second := New(store, nil), New(store, nil)
	first.client = client
	second.client = client
	t.Cleanup(func() { _ = first.Stop(context.Background()); _ = second.Stop(context.Background()) })
	manual := make(chan error, 1)
	go func() { manual <- first.RefreshByID(ctx, id) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	background := make(chan struct{})
	go func() { second.refreshOne(ctx, *row); close(background) }()
	// The second service has entered the shared worker and is waiting on the DB lock.
	for {
		second.lifecycleMu.Lock()
		n := len(second.active)
		second.lifecycleMu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	close(release)
	if err = <-manual; err != nil {
		t.Fatal(err)
	}
	select {
	case <-background:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if calls.Load() != 1 {
		t.Fatalf("provider rotations=%d, want 1", calls.Load())
	}
	current, err := store.GetDecryptedByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if current.TokenVersion != 2 || current.AccessToken != "access-v2" || current.RefreshToken != "refresh-v2" {
		t.Fatal("latest rotated tokens were not preserved")
	}
	// A late failure from an older snapshot must not suspend the fresh account.
	if err = store.MarkInvalid(ctx, id, "late invalid_grant", 1); err == nil {
		t.Fatal("stale failure accepted")
	}
	summary, err := store.GetSummaryByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if summary.InvalidReason != "" || summary.CooldownUntil != nil {
		t.Fatal("late failure cooled a refreshed account")
	}
}
