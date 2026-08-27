package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestShutdownStackClosesInReverseOrderAndOnlyOnce(t *testing.T) {
	var got []string
	stack := shutdownStack{}
	stack.Add("postgres", func(context.Context) error {
		got = append(got, "postgres")
		return nil
	})
	stack.Add("redis", func(context.Context) error {
		got = append(got, "redis")
		return nil
	})

	if err := stack.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stack.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if want := []string{"redis", "postgres"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("close order = %v, want %v", got, want)
	}
}

func TestShutdownStackJoinsErrorsAndRejectsLateResources(t *testing.T) {
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	var got []string
	stack := shutdownStack{}
	stack.Add("first", func(context.Context) error {
		got = append(got, "first")
		return firstErr
	})
	stack.Add("second", func(context.Context) error {
		got = append(got, "second")
		return secondErr
	})
	if err := stack.Close(context.Background()); !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("Close() error = %v, want both close errors", err)
	}
	stack.Add("late", func(context.Context) error {
		got = append(got, "late")
		return nil
	})
	if want := []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("close calls = %v, want %v", got, want)
	}
}

func TestLifecycleHealthSnapshotIsolatedAndIdempotent(t *testing.T) {
	lifecycle := newLifecycleHealth()
	lifecycle.MarkStarted("worker")
	lifecycle.MarkStarted("worker")

	snapshot := lifecycle.Snapshot()
	if got, want := snapshot["worker"], (componentHealth{Started: true}); got != want {
		t.Fatalf("started component = %+v, want %+v", got, want)
	}

	// Mutating a returned snapshot must not race with or alter the registry.
	snapshot["worker"] = componentHealth{Stopped: true}
	if got := lifecycle.Snapshot()["worker"]; got != (componentHealth{Started: true}) {
		t.Fatalf("registry was mutated through snapshot: %+v", got)
	}

	lifecycle.MarkStopped("worker")
	lifecycle.MarkStopped("worker")
	if got, want := lifecycle.Snapshot()["worker"], (componentHealth{Started: true, Stopped: true}); got != want {
		t.Fatalf("stopped component = %+v, want %+v", got, want)
	}
}

func TestHealthHandlerProjectsComponentsAndKeepsSchedulerCompatibility(t *testing.T) {
	lifecycle := newLifecycleHealth()
	lifecycle.MarkStarted(healthPostgres)
	lifecycle.MarkStarted(healthAIModules)

	handler := newHealthHandler("test-version", func() any {
		return map[string]any{"started": true, "stopped": false, "tasks": map[string]any{"sweep": map[string]any{"running": true}}}
	}, lifecycle)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}
	var response struct {
		Status     string                     `json:"status"`
		Version    string                     `json:"version"`
		Scheduler  map[string]any             `json:"scheduler"`
		Components map[string]componentHealth `json:"components"`
	}
	if err := json.NewDecoder(bytes.NewReader(recorder.Body.Bytes())).Decode(&response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Status != "ok" || response.Version != "test-version" {
		t.Fatalf("response identity = status %q version %q", response.Status, response.Version)
	}
	if response.Scheduler["started"] != true {
		t.Fatalf("scheduler compatibility field = %#v", response.Scheduler)
	}
	if got := response.Components[healthPostgres]; got != (componentHealth{Started: true}) {
		t.Fatalf("postgres health = %+v", got)
	}
	if got := response.Components[healthAIModules]; got != (componentHealth{Started: true}) {
		t.Fatalf("AI health = %+v", got)
	}
}

func TestPeriodicWorkerStopsIdempotently(t *testing.T) {
	started := make(chan struct{})
	worker := startHourlyCleanup(context.Background(), func(context.Context) {
		select {
		case <-started:
		default:
			close(started)
		}
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("periodic worker did not run its initial cleanup")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestPeriodicWorkerStopWaitsForInFlightCleanup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	worker := startHourlyCleanup(context.Background(), func(context.Context) {
		close(started)
		<-release
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("periodic worker did not enter cleanup")
	}

	shortCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := worker.Stop(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("short Stop() error = %v, want deadline exceeded", err)
	}
	close(release)
	longCtx, longCancel := context.WithTimeout(context.Background(), time.Second)
	defer longCancel()
	if err := worker.Stop(longCtx); err != nil {
		t.Fatalf("long Stop() error = %v", err)
	}
}
