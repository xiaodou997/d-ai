package scheduler

import (
	"errors"
	"testing"

	"go.uber.org/zap"
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
