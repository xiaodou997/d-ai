package audit

import (
	"context"
	"testing"
)

type noopStore struct{}

func (noopStore) InsertBatch(_ context.Context, _ []*Payload) error { return nil }

func TestWorkerSubmitSummarizesImageResponsesWhenBlobStorageDisabled(t *testing.T) {
	w := NewWorker(noopStore{}, nil, WorkerOptions{StoreImageBlobs: false})
	p := &Payload{
		RequestID:       "req-1",
		ClientProtocol:  "openai_images",
		ResponseMessage: []byte(`{"created":1234,"data":[{"b64_json":"abc","revised_prompt":"a cat"}]}`),
	}
	if !w.Submit(p) {
		t.Fatal("expected payload to be accepted")
	}
	if string(p.ResponseMessage) == `{"created":1234,"data":[{"b64_json":"abc","revised_prompt":"a cat"}]}` {
		t.Fatal("expected response message to be summarized before enqueue")
	}
}
