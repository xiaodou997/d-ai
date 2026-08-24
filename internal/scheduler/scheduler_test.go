package scheduler

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/dbtest"
)

func TestSchedulerStartStopIsIdempotent(t *testing.T) {
	s := NewScheduler(nil, nil, nil, zap.NewNop())
	s.Start()
	s.Start()
	s.Stop()
	s.Stop()
}

func TestSchedulerHealthTracksFailureAndCrossReplicaSkip(t *testing.T) {
	s := NewScheduler(nil, nil, nil, zap.NewNop())
	s.runTask("test_task", func() error { return errors.New("provider unavailable") })
	s.runTask("test_task", func() error { return &taskSkippedError{reason: "advisory_lock_held"} })

	health := s.Health()
	task, ok := health.Tasks["test_task"]
	if !ok {
		t.Fatal("test task health was not recorded")
	}
	if task.Running || task.ConsecutiveFailures != 1 || task.LastError != "provider unavailable" || task.SkipCount != 1 || task.LastSkipReason != "advisory_lock_held" {
		t.Fatalf("task health = %+v", task)
	}
}

func TestSchedulerAdvisoryLockIsReleasedOnSameConnection(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("scheduler test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	s := NewScheduler(pool, nil, nil, zap.NewNop())
	runs := 0
	for range 2 {
		err := s.withAdvisoryLock(ctx, "dai_scheduler_test_lock", "lock_held", func(context.Context) error {
			runs++
			return nil
		})
		if err != nil {
			t.Fatalf("run scheduler task under advisory lock: %v", err)
		}
	}
	if runs != 2 {
		t.Fatalf("task runs = %d, want 2 after lock release", runs)
	}
}
