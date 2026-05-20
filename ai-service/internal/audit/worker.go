package audit

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const workerBufferSize = 1000

// Store persists audit payloads to durable storage.
type Store interface {
	InsertPayload(ctx context.Context, p *Payload) error
}

// Worker drains a buffered channel and calls Store.InsertPayload for each
// entry. A full channel drops the payload with a warning — the main request
// path is never blocked.
type Worker struct {
	ch    chan *Payload
	store Store
}

// NewWorker creates a Worker backed by store.
func NewWorker(store Store) *Worker {
	return &Worker{
		ch:    make(chan *Payload, workerBufferSize),
		store: store,
	}
}

// Start begins draining the channel in a background goroutine. Exits when
// ctx is cancelled (typically on server shutdown).
func (w *Worker) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case p := <-w.ch:
				insertCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := w.store.InsertPayload(insertCtx, p); err != nil {
					zap.L().Warn("audit: insert failed",
						zap.Error(err),
						zap.String("request_id", p.RequestID),
					)
				}
				cancel()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Submit enqueues a payload for async persistence. Returns false when the
// channel is full; the payload is dropped and a warning is logged.
func (w *Worker) Submit(p *Payload) bool {
	select {
	case w.ch <- p:
		return true
	default:
		zap.L().Warn("audit: channel full, dropping payload",
			zap.String("request_id", p.RequestID),
		)
		return false
	}
}
