package cleanup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func TestServiceStartStopIsIdempotentAndWaitsForWorker(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:postgres@127.0.0.1:1/dai_test?sslmode=disable")
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	defer pool.Close()

	workerCtx, cancel := context.WithCancel(context.Background())
	cancel()
	service := NewService(pool, zap.NewNop())
	service.Start(workerCtx)
	if got := service.Health(); !got.Started || got.Stopped {
		t.Fatalf("running cleanup health = %+v, want started and not stopped", got)
	}

	service.mu.RLock()
	done := service.workerDone
	service.mu.RUnlock()
	if done == nil {
		t.Fatal("Start did not register a worker completion signal")
	}

	service.Start(context.Background())
	service.mu.RLock()
	doneAgain := service.workerDone
	service.mu.RUnlock()
	if doneAgain != done {
		t.Fatal("second Start replaced the original worker")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	service.Stop(stopCtx)
	service.Stop(stopCtx)
	select {
	case <-done:
	default:
		t.Fatal("Stop returned before cleanup worker exited")
	}
	if got := service.Health(); !got.Stopped {
		t.Fatalf("stopped cleanup health = %+v, want stopped", got)
	}
}

func TestServiceDoesNotStartAfterStop(t *testing.T) {
	service := &Service{}
	service.Stop(context.Background())
	service.Start(context.Background())

	service.mu.RLock()
	done := service.workerDone
	service.mu.RUnlock()
	if done != nil {
		t.Fatal("Start created a worker after Stop")
	}
	if got := service.Health(); !got.Stopped || got.Started {
		t.Fatalf("cleanup health after stop-before-start = %+v", got)
	}
}

func TestServiceStopWaitsForManualRun(t *testing.T) {
	service := NewService(nil, zap.NewNop())
	manualCtx, release, err := service.beginManualRun()
	if err != nil {
		t.Fatalf("begin manual run: %v", err)
	}
	started := make(chan struct{})
	releaseRun := make(chan struct{})
	go func() {
		defer release()
		close(started)
		<-manualCtx.Done()
		<-releaseRun
	}()
	<-started

	shortCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	service.Stop(shortCtx)
	select {
	case <-manualCtx.Done():
	default:
		t.Fatal("Stop did not cancel manual cleanup context")
	}

	longCtx, longCancel := context.WithTimeout(context.Background(), time.Second)
	defer longCancel()
	close(releaseRun)
	service.Stop(longCtx)
	if got := service.Health(); !got.Stopped {
		t.Fatalf("stopped cleanup health = %+v, want stopped", got)
	}
}

func TestServiceRejectsManualRunAfterStop(t *testing.T) {
	service := NewService(nil, zap.NewNop())
	service.Stop(context.Background())
	if _, err := service.StartManual([]string{TargetNotifications}, "operator"); !errors.Is(err, ErrServiceStopped) {
		t.Fatalf("StartManual after Stop error = %v, want %v", err, ErrServiceStopped)
	}
}
