package cleanup

import (
	"context"
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
}
