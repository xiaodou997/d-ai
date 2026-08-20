package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	enqueued   []*Payload
	deliveries []Delivery
	completed  []Delivery
	retried    []Delivery
	retryDead  []bool
	retryErr   error
}

func (s *fakeStore) Enqueue(_ context.Context, payload *Payload) error {
	s.enqueued = append(s.enqueued, payload)
	return nil
}

func (s *fakeStore) Claim(_ context.Context, workerID string, _ int, _ time.Duration) ([]Delivery, error) {
	for i := range s.deliveries {
		s.deliveries[i].WorkerID = workerID
		s.deliveries[i].Attempts++
	}
	return s.deliveries, nil
}

func (s *fakeStore) Complete(_ context.Context, delivery Delivery) error {
	s.completed = append(s.completed, delivery)
	return nil
}

func (s *fakeStore) Retry(_ context.Context, delivery Delivery, _ time.Time, _ error, dead bool) error {
	s.retried = append(s.retried, delivery)
	s.retryDead = append(s.retryDead, dead)
	return s.retryErr
}

func (s *fakeStore) Stats(context.Context) (QueueStats, error) { return QueueStats{}, nil }

func TestWorkerSubmitSummarizesImageResponsesWhenBlobStorageDisabled(t *testing.T) {
	store := &fakeStore{}
	w := NewWorker(store, nil, WorkerOptions{StoreImageBlobs: false})
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
	if len(store.enqueued) != 1 || store.enqueued[0] != p {
		t.Fatalf("durable enqueue = %#v", store.enqueued)
	}
}

func TestWorkerDrainCompletesDurableDelivery(t *testing.T) {
	store := &fakeStore{deliveries: []Delivery{{ID: 7, Attempts: 1, Payload: &Payload{RequestID: "req-7"}}}}
	w := NewWorker(store, nil, WorkerOptions{BatchSize: 10})
	count, err := w.DrainOnce(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("drain = %d, err=%v", count, err)
	}
	if len(store.completed) != 1 || len(store.retried) != 0 {
		t.Fatalf("completed=%d retried=%d", len(store.completed), len(store.retried))
	}
}

type failingBlobStore struct{}

func (failingBlobStore) Put(context.Context, string, []byte, string) error {
	return errors.New("blob unavailable")
}

func TestWorkerRetriesFailedDeliveryAndParksAfterMaxAttempts(t *testing.T) {
	store := &fakeStore{deliveries: []Delivery{{ID: 8, Attempts: 2, Payload: &Payload{
		RequestID:       "req-8",
		ClientProtocol:  "openai_chat",
		RequestMessages: []byte(`[{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,ZmFrZQ=="}}]}]`),
	}}}}
	w := NewWorker(store, failingBlobStore{}, WorkerOptions{MaxAttempts: 3, RetryBase: time.Millisecond})
	if _, err := w.DrainOnce(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(store.retried) != 1 || len(store.completed) != 0 {
		t.Fatalf("retried=%d completed=%d", len(store.retried), len(store.completed))
	}
	if len(store.retryDead) != 1 || !store.retryDead[0] {
		t.Fatal("expected third failed attempt to be parked as dead")
	}
}
