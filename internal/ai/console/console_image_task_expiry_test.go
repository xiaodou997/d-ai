package console

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/asynctask"
)

type recordingConsoleTaskAssets struct {
	taskID string
	err    error
}

func (a *recordingConsoleTaskAssets) DeleteTaskAssets(taskID string) (int, error) {
	a.taskID = taskID
	return 2, a.err
}

func TestConsoleImageTaskHandlerRemovesAssetsOnExpire(t *testing.T) {
	t.Parallel()

	assets := &recordingConsoleTaskAssets{}
	handler := &consoleImageTaskHandler{assets: assets}
	if err := handler.OnExpire(context.Background(), asynctask.Task{ID: "task-1"}); err != nil {
		t.Fatalf("OnExpire: %v", err)
	}
	if assets.taskID != "task-1" {
		t.Fatalf("deleted task id = %q, want task-1", assets.taskID)
	}
}

func TestConsoleImageTaskHandlerPropagatesAssetCleanupFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("storage unavailable")
	handler := &consoleImageTaskHandler{assets: &recordingConsoleTaskAssets{err: want}}
	if err := handler.OnExpire(context.Background(), asynctask.Task{ID: "task-1"}); !errors.Is(err, want) {
		t.Fatalf("OnExpire error = %v, want %v", err, want)
	}
}
