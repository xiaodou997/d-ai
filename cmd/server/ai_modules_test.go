package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/gateway"
)

type aiModulesUnauthorizedTransport struct{}

func (aiModulesUnauthorizedTransport) Do(context.Context, *clientruntime.WireRequest) (*clientruntime.WireResponse, error) {
	return &clientruntime.WireResponse{
		StatusCode: http.StatusUnauthorized,
		Headers:    make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

type aiModulesBlockingRefresher struct {
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (r *aiModulesBlockingRefresher) Refresh(context.Context, string) (clientruntime.Credential, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return clientruntime.Credential{AccessToken: "new-token", RefreshToken: "new-refresh"}, nil
}

func TestAIModulesLifecycleIsSafeForEmptyBundle(t *testing.T) {
	modules := &aiModules{}
	modules.Start(context.Background())
	modules.Start(context.Background())
	modules.Stop(context.Background())
	modules.Stop(context.Background())
}

func TestAIModulesCannotStartAfterStop(t *testing.T) {
	runtime := clientruntime.New(nil, nil)
	modules := &aiModules{clientRuntime: runtime}

	modules.Stop(context.Background())
	modules.Start(context.Background())
	if health := runtime.Health(); health.Started {
		t.Fatalf("runtime started after aggregate stop-before-start = %+v", health)
	}
	modules.lifecycleMu.Lock()
	started, stopped := modules.started, modules.stopped
	modules.lifecycleMu.Unlock()
	if started || !stopped {
		t.Fatalf("aggregate lifecycle after stop-before-start = started:%v stopped:%v", started, stopped)
	}
}

func TestAIModulesOwnsRuntimeGatewayTelemetryLifecycle(t *testing.T) {
	runtimeGateway := gateway.New(gateway.Deps{Logger: zap.NewNop()})
	modules := &aiModules{RuntimeGateway: runtimeGateway}

	modules.Start(context.Background())
	if health := runtimeGateway.Health(); !health.Started || health.Stopped {
		t.Fatalf("runtime gateway health after Start = %+v", health)
	}
	modules.Stop(context.Background())
	if health := runtimeGateway.Health(); !health.Started || !health.Stopped {
		t.Fatalf("runtime gateway health after Stop = %+v", health)
	}
}

func TestAIModulesOwnsClientCatalogLifecycle(t *testing.T) {
	catalog := clientcatalog.New(nil, nil, zap.NewNop())
	modules := &aiModules{clientCatalog: catalog}

	modules.Start(context.Background())
	if health := catalog.Health(); !health.Started || health.Stopped {
		t.Fatalf("client catalog health after Start = %+v", health)
	}
	modules.Stop(context.Background())
	if health := catalog.Health(); !health.Started || !health.Stopped {
		t.Fatalf("client catalog health after Stop = %+v", health)
	}
}

func TestAIModulesOwnsClientRuntimeLifecycle(t *testing.T) {
	runtime := clientruntime.New(nil, nil)
	modules := &aiModules{clientRuntime: runtime}

	modules.Start(context.Background())
	if health := runtime.Health(); !health.Started || health.Stopped {
		t.Fatalf("client runtime health after Start = %+v", health)
	}
	modules.Stop(context.Background())
	if health := runtime.Health(); !health.Started || !health.Stopped {
		t.Fatalf("client runtime health after Stop = %+v", health)
	}
}

func TestAIModulesStopCanRetryAfterDeadline(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRefresh := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseRefresh()

	refresher := &aiModulesBlockingRefresher{started: make(chan struct{}), release: release}
	runtime := clientruntime.New(aiModulesUnauthorizedTransport{}, refresher)
	modules := &aiModules{clientRuntime: runtime}
	modules.Start(context.Background())

	invokeDone := make(chan struct{})
	go func() {
		exchange, _ := runtime.Invoke(context.Background(), clientruntime.Invocation{
			Provider:  domain.FixedProviderCodex,
			Operation: clientruntime.OperationResponses,
			Protocol:  domain.ProtocolOpenAIResponses,
			Model:     "gpt-5.6-sol",
			Body:      []byte(`{"input":"hello"}`),
			Credential: clientruntime.Credential{
				ID:           "credential-stop-retry",
				AccessToken:  "old-token",
				RefreshToken: "refresh-token",
			},
		})
		if exchange != nil && exchange.Response != nil && exchange.Response.Body != nil {
			_ = exchange.Response.Body.Close()
		}
		close(invokeDone)
	}()
	select {
	case <-refresher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("credential refresh did not start")
	}

	shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	modules.Stop(shortCtx)
	cancel()
	select {
	case <-invokeDone:
		t.Fatal("invoke completed while refresh was still blocked")
	default:
	}

	longStopDone := make(chan struct{})
	go func() {
		modules.Stop(context.Background())
		close(longStopDone)
	}()
	select {
	case <-longStopDone:
		t.Fatal("retry Stop returned before the in-flight refresh completed")
	case <-time.After(50 * time.Millisecond):
	}

	releaseRefresh()
	select {
	case <-longStopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("retry Stop did not finish after the refresh was released")
	}
	select {
	case <-invokeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("invoke did not finish after the refresh was released")
	}
}
