package clientruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/lifecycle"
)

const authResponseSnapshotLimit = 64 << 10
const sharedRefreshTimeout = 30 * time.Second
const refreshedCredentialReuseTTL = 30 * time.Second

var errRuntimeStopped = errors.New("client runtime is stopped")

type refreshedCredentialEntry struct {
	credential Credential
	expiresAt  time.Time
}

type clientProfile interface {
	revision() string
	supports(domain.UpstreamProtocol) bool
	prepare(Invocation) (*WireRequest, error)
}

type Runtime struct {
	transport Transport
	refresher CredentialRefresher
	profiles  map[domain.FixedProviderType]clientProfile
	refreshes singleflight.Group

	refreshMu         sync.Mutex
	recentlyRefreshed map[string]refreshedCredentialEntry

	lifecycleMu   sync.Mutex
	started       bool
	stopped       bool
	ownerCtx      context.Context
	nextRefreshID uint64
	activeRefresh map[uint64]context.CancelFunc
	refreshWG     sync.WaitGroup
}

var _ lifecycle.Component = (*Runtime)(nil)

func New(transport Transport, refresher CredentialRefresher) *Runtime {
	return &Runtime{
		transport: transport,
		refresher: refresher,
		profiles: map[domain.FixedProviderType]clientProfile{
			domain.FixedProviderCodex:       codexProfileV01441{},
			domain.FixedProviderClaudeOAuth: claudeProfileV21220{},
			domain.FixedProviderGeminiCLI:   geminiCLIProfileV015{},
			domain.FixedProviderAntigravity: antigravityProfileV1016{},
		},
		recentlyRefreshed: make(map[string]refreshedCredentialEntry),
		activeRefresh:     make(map[uint64]context.CancelFunc),
	}
}

// Start marks the fixed-provider runtime as owned by the process lifecycle.
// Invocation remains usable before Start for lightweight callers and tests;
// Start is idempotent and a stopped runtime cannot be restarted.
func (r *Runtime) Start(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.lifecycleMu.Lock()
	if r.started || r.stopped {
		r.lifecycleMu.Unlock()
		return
	}
	r.started = true
	r.ownerCtx = ctx
	r.lifecycleMu.Unlock()
}

// Stop cancels every in-flight credential refresh and waits for shared
// singleflight leaders to leave the refresher path. A later call can continue
// waiting after an earlier deadline expires.
func (r *Runtime) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.lifecycleMu.Lock()
	r.stopped = true
	r.ownerCtx = nil
	cancels := make([]context.CancelFunc, 0, len(r.activeRefresh))
	for _, cancel := range r.activeRefresh {
		cancels = append(cancels, cancel)
	}
	r.lifecycleMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		r.refreshWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Health returns a lock-safe lifecycle snapshot for management probes.
func (r *Runtime) Health() lifecycle.HealthSnapshot {
	if r == nil {
		return lifecycle.HealthSnapshot{}
	}
	r.lifecycleMu.Lock()
	started, stopped := r.started, r.stopped
	r.lifecycleMu.Unlock()
	return lifecycle.HealthSnapshot{Started: started, Stopped: stopped}
}

func (r *Runtime) SupportsInvocation(provider domain.FixedProviderType, protocol domain.UpstreamProtocol) bool {
	if r == nil {
		return false
	}
	profile, ok := r.profiles[provider]
	return ok && profile.supports(protocol)
}

func (r *Runtime) Invoke(ctx context.Context, in Invocation) (*Exchange, error) {
	if r == nil || r.transport == nil {
		return nil, &Error{
			Code:       ErrorRuntimeNotConfigured,
			SafeDetail: "fixed-provider client runtime is not configured",
		}
	}
	profile, ok := r.profiles[in.Provider]
	if !ok {
		return nil, &Error{
			Code:       ErrorUnsupportedProvider,
			SafeDetail: fmt.Sprintf("fixed-provider client profile %q is not registered", in.Provider),
		}
	}
	if in.Credential.ID == "" || in.Credential.AccessToken == "" {
		return nil, &Error{
			Code:            ErrorInvalidInvocation,
			ProfileRevision: profile.revision(),
			SafeDetail:      "selected credential is incomplete",
		}
	}

	request, err := profile.prepare(in)
	if err != nil {
		return nil, &Error{
			Code:            ErrorRequestContract,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider request does not satisfy the selected client profile",
			Cause:           err,
		}
	}
	exchange := &Exchange{
		Trace: Trace{
			ProfileRevision:  profile.revision(),
			RequestURL:       request.URL,
			CredentialID:     in.Credential.ID,
			CredentialEffect: CredentialEffectNone,
		},
	}

	response, err := r.do(ctx, request, &exchange.Trace)
	if err != nil {
		return exchange, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "fixed-provider upstream transport failed",
			Cause:           err,
		}
	}
	if response == nil {
		return exchange, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "fixed-provider transport returned no response",
		}
	}
	exchange.Response = response

	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		exchange.Trace.CredentialEffect = CredentialEffectCooldown
		exchange.Trace.CooldownUntil = responseCooldownUntil(response.Headers, now())
		return exchange, nil
	}
	if response.StatusCode != http.StatusUnauthorized {
		return exchange, nil
	}
	if in.Credential.RefreshToken == "" || r.refresher == nil {
		exchange.Trace.CredentialEffect = CredentialEffectInvalidate
		return exchange, nil
	}

	originalBody := snapshotAndClose(response.Body, authResponseSnapshotLimit)
	refreshed, refreshErr := r.refreshCredential(ctx, in.Credential)
	exchange.Trace.RefreshCalls++
	if refreshErr != nil {
		response.Body = io.NopCloser(bytes.NewReader(originalBody))
		exchange.Trace.CredentialEffect = CredentialEffectRefreshFailed
		return exchange, nil
	}

	in.Credential = refreshed
	retryRequest, err := profile.prepare(in)
	if err != nil {
		return exchange, &Error{
			Code:            ErrorRequestContract,
			ProfileRevision: profile.revision(),
			SafeDetail:      "refreshed credential could not satisfy the client profile",
			Cause:           err,
		}
	}
	retryResponse, err := r.do(ctx, retryRequest, &exchange.Trace)
	if err != nil {
		return exchange, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "fixed-provider retry after credential refresh failed",
			Cause:           err,
		}
	}
	if retryResponse == nil {
		return exchange, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "fixed-provider retry returned no response",
		}
	}
	exchange.Response = retryResponse
	if retryResponse.StatusCode == http.StatusUnauthorized {
		exchange.Trace.CredentialEffect = CredentialEffectInvalidate
	} else if retryResponse.StatusCode == http.StatusForbidden ||
		retryResponse.StatusCode == http.StatusTooManyRequests {
		exchange.Trace.CredentialEffect = CredentialEffectCooldown
		exchange.Trace.CooldownUntil = responseCooldownUntil(retryResponse.Headers, now())
	} else {
		exchange.Trace.CredentialEffect = CredentialEffectRefreshed
	}
	return exchange, nil
}

func (r *Runtime) do(ctx context.Context, request *WireRequest, trace *Trace) (*WireResponse, error) {
	startedAt := now()
	response, err := r.transport.Do(ctx, request)
	finishedAt := now()
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	trace.ProviderCalls++
	trace.WireAttempts = append(trace.WireAttempts, WireAttempt{
		StatusCode: status,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	})
	return response, err
}

func (r *Runtime) refreshCredential(ctx context.Context, original Credential) (Credential, error) {
	if refreshed, ok := r.recentRefresh(original); ok {
		return refreshed, nil
	}
	ch := r.refreshes.DoChan(original.ID, func() (any, error) {
		if refreshed, ok := r.recentRefresh(original); ok {
			return refreshed, nil
		}
		refreshCtx, release, ok := r.beginRefresh(ctx)
		if !ok {
			return Credential{}, errRuntimeStopped
		}
		defer release()
		refreshed, err := r.refresher.Refresh(refreshCtx, original.ID)
		if err == nil {
			r.storeRecentRefresh(refreshed)
		}
		return refreshed, err
	})
	select {
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	case result := <-ch:
		if errors.Is(result.Err, errRuntimeStopped) {
			return Credential{}, context.Canceled
		}
		if result.Err != nil {
			return Credential{}, result.Err
		}
		credential, ok := result.Val.(Credential)
		if !ok {
			return Credential{}, fmt.Errorf("credential refresher returned %T", result.Val)
		}
		return credential, nil
	}
}

func (r *Runtime) beginRefresh(callerCtx context.Context) (context.Context, func(), bool) {
	base := context.WithoutCancel(callerCtx)
	refreshCtx, cancel := context.WithTimeout(base, sharedRefreshTimeout)
	r.lifecycleMu.Lock()
	if r.stopped {
		r.lifecycleMu.Unlock()
		cancel()
		return nil, nil, false
	}
	if r.activeRefresh == nil {
		r.activeRefresh = make(map[uint64]context.CancelFunc)
	}
	ownerCtx := r.ownerCtx
	r.nextRefreshID++
	id := r.nextRefreshID
	r.activeRefresh[id] = cancel
	r.refreshWG.Add(1)
	r.lifecycleMu.Unlock()

	stopOwner := func() {}
	if ownerCtx != nil {
		stop := context.AfterFunc(ownerCtx, cancel)
		stopOwner = func() { stop() }
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			stopOwner()
			cancel()
			r.lifecycleMu.Lock()
			delete(r.activeRefresh, id)
			r.lifecycleMu.Unlock()
			r.refreshWG.Done()
		})
	}
	return refreshCtx, release, true
}

func (r *Runtime) recentRefresh(original Credential) (Credential, bool) {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	entry, ok := r.recentlyRefreshed[original.ID]
	if !ok {
		return Credential{}, false
	}
	if !now().Before(entry.expiresAt) {
		delete(r.recentlyRefreshed, original.ID)
		return Credential{}, false
	}
	if entry.credential.TokenVersion > original.TokenVersion ||
		entry.credential.AccessToken != original.AccessToken {
		return entry.credential, true
	}
	return Credential{}, false
}

func (r *Runtime) storeRecentRefresh(credential Credential) {
	if credential.ID == "" || credential.AccessToken == "" {
		return
	}
	r.refreshMu.Lock()
	r.recentlyRefreshed[credential.ID] = refreshedCredentialEntry{
		credential: credential,
		expiresAt:  now().Add(refreshedCredentialReuseTTL),
	}
	r.refreshMu.Unlock()
}

func snapshotAndClose(body io.ReadCloser, limit int64) []byte {
	if body == nil {
		return nil
	}
	defer body.Close()
	snapshot, _ := io.ReadAll(io.LimitReader(body, limit))
	return snapshot
}

var now = func() time.Time {
	return time.Now()
}
