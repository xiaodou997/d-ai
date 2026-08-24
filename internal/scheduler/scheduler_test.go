package scheduler

import (
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
