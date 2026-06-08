package serving

import (
	"context"
	"testing"
	"time"

	"xiaodou/unihub/ai-service/internal/domain"
)

func waitCancel(t *testing.T, dc *deadlineController, want error, within time.Duration) {
	t.Helper()
	select {
	case <-dc.ctx.Done():
		if got := dc.cause(); got != want {
			t.Fatalf("cause = %v, want %v", got, want)
		}
	case <-time.After(within):
		t.Fatalf("context not cancelled within %v", within)
	}
}

func TestDeadlineControllerConnectTimeout(t *testing.T) {
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		Connect: 20 * time.Millisecond, FirstByte: time.Hour, Idle: time.Hour, MaxDuration: time.Hour,
	})
	defer dc.stop()
	waitCancel(t, dc, ErrConnectTimeout, time.Second)
}

func TestDeadlineControllerFirstByteTimeout(t *testing.T) {
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		Connect: time.Hour, FirstByte: 20 * time.Millisecond, Idle: time.Hour, MaxDuration: time.Hour,
	})
	defer dc.stop()
	dc.headersReceived()
	waitCancel(t, dc, ErrFirstByteTimeout, time.Second)
}

func TestDeadlineControllerIdleResetThenTimeout(t *testing.T) {
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		Connect: time.Hour, FirstByte: time.Hour, Idle: 40 * time.Millisecond, MaxDuration: time.Hour,
	})
	defer dc.stop()
	dc.headersReceived()
	dc.firstByte()
	// Each chunk resets the idle timer — the stream must survive well past one
	// idle window as long as chunks keep arriving.
	for i := 0; i < 4; i++ {
		time.Sleep(20 * time.Millisecond)
		dc.chunkReceived()
	}
	if dc.ctx.Err() != nil {
		t.Fatalf("idle timer should have been reset by chunkReceived")
	}
	waitCancel(t, dc, ErrIdleTimeout, time.Second)
}

func TestDeadlineControllerMaxDuration(t *testing.T) {
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		Connect: time.Hour, FirstByte: time.Hour, Idle: time.Hour, MaxDuration: 25 * time.Millisecond,
	})
	defer dc.stop()
	dc.headersReceived()
	dc.firstByte()
	waitCancel(t, dc, ErrMaxDuration, time.Second)
}

func TestDeadlineControllerStopIsClean(t *testing.T) {
	dc := newDeadlineController(context.Background(), domain.RouteTimeouts{
		Connect: time.Hour, FirstByte: time.Hour, Idle: time.Hour, MaxDuration: time.Hour,
	})
	dc.headersReceived()
	dc.firstByte()
	dc.stop()
	if dc.ctx.Err() == nil {
		t.Fatalf("stop should cancel the context")
	}
	if dc.cause() != nil {
		t.Fatalf("stop must not look like a phase timeout, got %v", dc.cause())
	}
}

func TestDeadlineControllerParentCancelIsNotAPhaseTimeout(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	dc := newDeadlineController(parent, domain.RouteTimeouts{
		Connect: time.Hour, FirstByte: time.Hour, Idle: time.Hour, MaxDuration: time.Hour,
	})
	defer dc.stop()
	cancel() // client disconnect
	select {
	case <-dc.ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("parent cancel did not propagate")
	}
	if dc.cause() != nil {
		t.Fatalf("client disconnect must not be classified as a phase timeout, got %v", dc.cause())
	}
}
