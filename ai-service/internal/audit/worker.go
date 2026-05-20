package audit

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	workerBatchSize    = 50              // max rows per INSERT batch
	workerFlushEvery   = 5 * time.Second // ticker: flush pending batch
	workerByteBudget   = 64 << 20        // 64 MiB channel byte budget
	workerMaxPayload   = 4 << 20         // 4 MiB per payload; larger ones are dropped
)

// Store persists audit payloads to durable storage.
type Store interface {
	InsertBatch(ctx context.Context, payloads []*Payload) error
}

// Worker drains a buffered channel and batch-inserts into the audit store.
// The channel is capped by a byte budget rather than a fixed entry count;
// oversized payloads are dropped immediately. On shutdown, the worker drains
// any remaining entries before returning.
type Worker struct {
	ch        chan *Payload
	store     Store
	byteUsed  int64 // approximate in-flight byte count (not precise — race is fine)
}

// NewWorker creates a Worker backed by store.
func NewWorker(store Store) *Worker {
	// Pre-allocate a generous entry buffer; byte budget enforces real back-pressure.
	const entryCap = 4096
	return &Worker{
		ch:    make(chan *Payload, entryCap),
		store: store,
	}
}

// Start begins draining the channel in a background goroutine.
// Returns when ctx is cancelled; drains remaining entries before returning.
func (w *Worker) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *Worker) run(ctx context.Context) {
	ticker := time.NewTicker(workerFlushEvery)
	defer ticker.Stop()

	batch := make([]*Payload, 0, workerBatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := w.store.InsertBatch(flushCtx, batch); err != nil {
			zap.L().Warn("audit: batch insert failed",
				zap.Error(err),
				zap.Int("batch_size", len(batch)),
			)
		}
		for _, p := range batch {
			w.byteUsed -= int64(payloadSize(p))
		}
		batch = batch[:0]
	}

	for {
		select {
		case p := <-w.ch:
			batch = append(batch, p)
			if len(batch) >= workerBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			// Drain remaining channel entries then flush.
			for {
				select {
				case p := <-w.ch:
					batch = append(batch, p)
					if len(batch) >= workerBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// Submit enqueues a payload for async persistence.
// Returns false when the payload exceeds workerMaxPayload or the byte budget
// is exhausted; the payload is dropped and a warning is logged.
func (w *Worker) Submit(p *Payload) bool {
	sz := payloadSize(p)
	if sz > workerMaxPayload {
		zap.L().Warn("audit: payload too large, dropping",
			zap.String("request_id", p.RequestID),
			zap.Int("size_bytes", sz),
		)
		return false
	}
	if w.byteUsed+int64(sz) > workerByteBudget {
		zap.L().Warn("audit: byte budget exhausted, dropping payload",
			zap.String("request_id", p.RequestID),
		)
		return false
	}
	select {
	case w.ch <- p:
		w.byteUsed += int64(sz)
		return true
	default:
		zap.L().Warn("audit: channel full, dropping payload",
			zap.String("request_id", p.RequestID),
		)
		return false
	}
}

// payloadSize is a rough byte estimate for back-pressure accounting.
func payloadSize(p *Payload) int {
	return len(p.RequestMessages) + len(p.RequestParams) +
		len(p.ResponseMessage) + len(p.MediaRefs) +
		len(p.RequestID) + len(p.RequestPath) + 256 // fixed overhead estimate
}
