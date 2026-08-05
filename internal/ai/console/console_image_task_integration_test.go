package console

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/testsupport"
)

type stubConsoleImageTaskResolver struct{}

func (stubConsoleImageTaskResolver) prepareConsoleImageTask(
	context.Context,
	coreidentity.Subject,
	[]byte,
	string,
	string,
) (consoleImageResolution, error) {
	return consoleImageResolution{
		Input:     json.RawMessage(`{"prompt":"persisted","operation":"generation"}`),
		ModelCode: "image-model",
	}, nil
}

func (stubConsoleImageTaskResolver) resolveConsoleImageTask(
	_ context.Context,
	subject coreidentity.Subject,
	input consoleImageTaskInputPayload,
	operation string,
) (gateway.ReplayInput, error) {
	body, _ := json.Marshal(map[string]string{
		"operation": operation,
		"prompt":    input.Prompt,
	})
	return gateway.ReplayInput{
		Subject:     subject,
		Capability:  domain.CapabilityImage,
		Protocol:    domain.ProtocolOpenAIImages,
		ClientPath:  "/v1/images/generations",
		Body:        body,
		ContentType: "application/json",
	}, nil
}

type recordingConsoleImageReplayer struct {
	calls chan gateway.ReplayInput
}

func (r *recordingConsoleImageReplayer) Replay(_ context.Context, in gateway.ReplayInput) gateway.ReplayResult {
	r.calls <- in
	return gateway.ReplayResult{
		Request:    &coreruntime.Result{RequestStatus: string(domain.RequestSuccess), CallerChargeMicro: 321},
		Body:       []byte(`{"data":[{"url":"https://images.example/result.png"}]}`),
		StatusCode: 200,
	}
}

func TestConsoleImageTaskRunsThroughGenericEngine(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	engine, err := asynctask.New(asynctask.Config{
		Workers:      1,
		PollInterval: 20 * time.Millisecond,
		LeaseTTL:     3 * time.Second,
		ReapInterval: 20 * time.Millisecond,
	}, asynctask.Deps{
		Pool:   pool,
		Logger: zap.NewNop(),
		Subjects: asynctask.SubjectResolverFunc(func(_ context.Context, ref asynctask.SubjectRef) (coreidentity.Subject, error) {
			return coreidentity.Subject{
				AuthMethod:    ref.AuthMethod,
				RequestSource: coreidentity.RequestSourceWebImage,
				Scope:         coreidentity.ScopeUser,
				TenantID:      ref.TenantID,
				UserID:        ref.UserID,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	replayer := &recordingConsoleImageReplayer{calls: make(chan gateway.ReplayInput, 1)}
	engine.Register(consoleImageGenerationTaskType, &consoleImageTaskHandler{
		resolver:  stubConsoleImageTaskResolver{},
		operation: "generation",
		replayer:  replayer,
	}, asynctask.Options{MaxAttempts: 1})

	runCtx, cancel := context.WithCancel(context.Background())
	engine.Start(runCtx)
	t.Cleanup(func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		engine.Stop(stopCtx)
	})

	subject := coreidentity.Subject{
		AuthMethod:    coreidentity.AuthMethodJWT,
		RequestSource: coreidentity.RequestSourceWebImage,
		Scope:         coreidentity.ScopeUser,
		TenantID:      "tenant-console",
		UserID:        "user-console",
	}
	created, err := engine.Submit(ctx, asynctask.SubmitRequest{
		Subject:     subject,
		Type:        consoleImageGenerationTaskType,
		Body:        []byte(`{"prompt":"inbound"}`),
		ContentType: "application/json",
	})
	if err != nil {
		t.Fatalf("submit console image task: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var view asynctask.TaskView
	for time.Now().Before(deadline) {
		view, err = engine.Get(ctx, subject, created.ID)
		if err == nil && view.Status == domain.TaskCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("get completed task: %v", err)
	}
	if view.Status != domain.TaskCompleted {
		t.Fatalf("task status = %q, want completed", view.Status)
	}
	if view.ModelCode != "image-model" || view.CallerCharge != 321 {
		t.Fatalf("task result = model %q cost %d", view.ModelCode, view.CallerCharge)
	}
	if !strings.HasPrefix(view.RequestID, "atsk_"+created.ID+"_1") {
		t.Fatalf("request id = %q", view.RequestID)
	}
	if len(view.Output) == 0 {
		t.Fatal("completed task has no result payload")
	}

	select {
	case replay := <-replayer.calls:
		if replay.RequestID != view.RequestID {
			t.Fatalf("replay request id = %q, want %q", replay.RequestID, view.RequestID)
		}
		if !strings.Contains(string(replay.Body), "persisted") {
			t.Fatalf("replay body = %s, want persisted input", replay.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("replayer was not called")
	}
}
