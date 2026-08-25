package audit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
)

const (
	workerBatchSize   = 50
	workerPollEvery   = 500 * time.Millisecond
	workerLeaseTTL    = 30 * time.Second
	workerMaxAttempts = 10
	workerRetryBase   = 500 * time.Millisecond
	workerRetryMax    = 5 * time.Minute
	workerMaxPayload  = 4 << 20 // 4 MiB per payload; larger ones are rejected
	workerOpTimeout   = 15 * time.Second
	workerStallAfter  = 2 * time.Minute
)

var (
	pendingInbox = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_ai_audit_inbox_pending",
		Help: "Audit payloads waiting for durable materialization.",
	})
	deadInbox = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_ai_audit_inbox_dead",
		Help: "Audit payloads parked after exhausting delivery attempts.",
	})
	oldestInboxSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dai_ai_audit_inbox_oldest_pending_seconds",
		Help: "Age of the oldest pending audit payload.",
	})
	enqueuedPayloads = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dai_ai_audit_inbox_enqueued_total",
		Help: "Audit payloads accepted into the durable inbox.",
	})
	completedPayloads = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dai_ai_audit_inbox_completed_total",
		Help: "Audit payloads materialized from the durable inbox.",
	})
	failedPayloads = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dai_ai_audit_inbox_failed_total",
		Help: "Audit payload delivery attempts that failed.",
	})
)

// Store is the durable inbox contract. Implementations must make Complete
// idempotent: a process crash after materializing a payload but before deleting
// its inbox row must never create a duplicate audit record.
type Store interface {
	Enqueue(ctx context.Context, payload *Payload) error
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]Delivery, error)
	Complete(ctx context.Context, delivery Delivery) error
	Retry(ctx context.Context, delivery Delivery, availableAt time.Time, cause error, dead bool) error
	Stats(ctx context.Context) (QueueStats, error)
}

// Delivery is one leased inbox item.
type Delivery struct {
	ID       int64
	Payload  *Payload
	Attempts int
	WorkerID string
}

// QueueStats describes the queue health exposed to operators.
type QueueStats struct {
	Pending int64
	Dead    int64
	OldestS float64
}

// BlobPutter persists content-addressed binary blobs.
// Duplicate sha256 keys are silently ignored (ON CONFLICT DO NOTHING semantics).
type BlobPutter interface {
	Put(ctx context.Context, sha256 string, data []byte, contentType string) error
}

// Worker drains a PostgreSQL-backed durable inbox. There is deliberately no
// process-local channel: accepted payloads survive restarts and each item is
// leased with FOR UPDATE SKIP LOCKED so multiple instances can drain safely.
type Worker struct {
	store       Store
	blobStore   BlobPutter // optional; nil = skip media extraction
	opts        WorkerOptions
	workerID    string
	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type WorkerOptions struct {
	StoreImageBlobs bool
	PollInterval    time.Duration
	BatchSize       int
	LeaseTTL        time.Duration
	MaxAttempts     int
	RetryBase       time.Duration
}

// NewWorker creates a Worker backed by a durable store. Zero-valued timing
// options use conservative production defaults; tests may shorten them.
func NewWorker(store Store, blobStore BlobPutter, opts WorkerOptions) *Worker {
	if opts.PollInterval <= 0 {
		opts.PollInterval = workerPollEvery
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = workerBatchSize
	}
	if opts.LeaseTTL <= 0 {
		opts.LeaseTTL = workerLeaseTTL
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = workerMaxAttempts
	}
	if opts.RetryBase <= 0 {
		opts.RetryBase = workerRetryBase
	}
	return &Worker{
		store:     store,
		blobStore: blobStore,
		opts:      opts,
		workerID:  fmt.Sprintf("audit-%d", time.Now().UnixNano()),
	}
}

// Start begins draining the durable inbox in a background goroutine.
func (w *Worker) Start(ctx context.Context) {
	if w == nil || w.store == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	if w.started || w.stopped {
		w.lifecycleMu.Unlock()
		return
	}
	w.started = true
	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.wg.Add(1)
	w.lifecycleMu.Unlock()
	go func() { defer w.wg.Done(); w.run(workerCtx) }()
}

// Stop cancels polling and waits for any in-flight delivery to complete or
// schedule its durable retry decision before shutdown continues.
func (w *Worker) Stop(ctx context.Context) {
	if w == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	if w.stopped {
		started := w.started
		w.lifecycleMu.Unlock()
		if started {
			w.wait(ctx)
		}
		return
	}
	w.stopped = true
	started, cancel := w.started, w.cancel
	w.lifecycleMu.Unlock()
	if !started {
		return
	}
	cancel()
	w.wait(ctx)
}

func (w *Worker) wait(ctx context.Context) {
	done := make(chan struct{})
	go func() { w.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (w *Worker) run(ctx context.Context) {
	ticker := time.NewTicker(w.opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processed, err := w.DrainOnce(ctx)
			if err != nil && ctx.Err() == nil {
				zap.L().Error("audit inbox drain failed", zap.Error(err))
			}
			if processed == w.opts.BatchSize && err == nil {
				for processed == w.opts.BatchSize && ctx.Err() == nil {
					processed, err = w.DrainOnce(ctx)
					if err != nil && ctx.Err() == nil {
						zap.L().Error("audit inbox drain failed", zap.Error(err))
					}
				}
			}
			w.publishHealth(ctx)
		}
	}
}

// DrainOnce leases and processes at most one batch. It is exported so tests
// and operators can drain deterministically without starting a goroutine.
func (w *Worker) DrainOnce(ctx context.Context) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	deliveries, err := w.store.Claim(ctx, w.workerID, w.opts.BatchSize, w.opts.LeaseTTL)
	if err != nil {
		return 0, fmt.Errorf("claim audit inbox: %w", err)
	}
	for _, delivery := range deliveries {
		w.process(ctx, delivery)
	}
	return len(deliveries), nil
}

func (w *Worker) process(parent context.Context, delivery Delivery) {
	workCtx, cancel := context.WithTimeout(parent, workerOpTimeout)
	err := w.processPayload(workCtx, delivery.Payload)
	if err == nil {
		err = w.store.Complete(workCtx, delivery)
	}
	cancel()
	if err == nil {
		completedPayloads.Inc()
		return
	}

	failedPayloads.Inc()
	dead := delivery.Attempts >= w.opts.MaxAttempts
	next := time.Now().UTC().Add(retryDelay(w.opts.RetryBase, delivery.Attempts))
	if dead {
		next = time.Now().UTC()
	}
	retryCtx, retryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	retryErr := w.store.Retry(retryCtx, delivery, next, err, dead)
	retryCancel()
	fields := []zap.Field{
		zap.Int64("inbox_id", delivery.ID),
		zap.String("request_id", payloadRequestID(delivery.Payload)),
		zap.Int("attempts", delivery.Attempts),
		zap.Error(err),
	}
	if retryErr != nil {
		fields = append(fields, zap.NamedError("retry_error", retryErr))
	}
	if dead {
		zap.L().Error("audit payload parked in dead inbox", fields...)
	} else {
		zap.L().Warn("audit payload delivery failed; scheduled retry", fields...)
	}
}

func (w *Worker) processPayload(ctx context.Context, payload *Payload) error {
	if payload == nil {
		return errors.New("nil audit payload")
	}
	if err := w.extractBlobs(ctx, payload); err != nil {
		return err
	}
	return nil
}

func (w *Worker) extractBlobs(ctx context.Context, p *Payload) error {
	if w.blobStore == nil {
		return nil
	}
	protocol := domain.UpstreamProtocol(p.ClientProtocol)
	var msgBlobs []MediaBlob
	if len(p.RequestMessages) > 0 {
		p.RequestMessages, msgBlobs = ExtractFromMessages(p.RequestMessages, protocol)
	}
	var respBlobs []MediaBlob
	if len(p.ResponseMessage) > 0 && protocol == domain.ProtocolOpenAIImages && w.opts.StoreImageBlobs {
		p.ResponseMessage, respBlobs = ExtractFromImagesResponse(p.ResponseMessage)
	}
	allBlobs := append(msgBlobs, respBlobs...)
	for _, blob := range allBlobs {
		if err := w.blobStore.Put(ctx, blob.SHA256, blob.Data, blob.ContentType); err != nil {
			return fmt.Errorf("put audit blob %s: %w", blob.SHA256, err)
		}
	}
	if len(allBlobs) > 0 {
		p.MediaRefs = BuildMediaRefs(allBlobs)
	}
	return nil
}

// Submit keeps the original submitter contract for callers that do not carry
// a request context. Durable callers should prefer SubmitContext.
func (w *Worker) Submit(p *Payload) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.SubmitContext(ctx, p)
}

// SubmitContext durably enqueues a payload. It intentionally performs the
// small inbox insert synchronously: returning success means PostgreSQL owns
// the payload, not that an in-memory buffer currently has room.
func (w *Worker) SubmitContext(ctx context.Context, p *Payload) bool {
	if w == nil || w.store == nil || p == nil {
		return false
	}
	if !w.opts.StoreImageBlobs && p.ClientProtocol == string(domain.ProtocolOpenAIImages) && len(p.ResponseMessage) > 0 {
		if summarized := SummarizeImagesResponse(p.ResponseMessage); len(summarized) > 0 {
			p.ResponseMessage = summarized
		} else {
			p.ResponseMessage = nil
		}
	}
	sz := payloadSize(p)
	if sz > workerMaxPayload {
		failedPayloads.Inc()
		zap.L().Error("audit: payload too large, rejecting", zap.String("request_id", p.RequestID), zap.Int("size_bytes", sz))
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := w.store.Enqueue(ctx, p); err != nil {
		failedPayloads.Inc()
		zap.L().Error("audit: durable enqueue failed", zap.String("request_id", p.RequestID), zap.Error(err))
		return false
	}
	enqueuedPayloads.Inc()
	return true
}

func (w *Worker) publishHealth(ctx context.Context) {
	stats, err := w.store.Stats(ctx)
	if err != nil {
		if ctx.Err() == nil {
			zap.L().Warn("audit inbox stats unavailable", zap.Error(err))
		}
		return
	}
	pendingInbox.Set(float64(stats.Pending))
	deadInbox.Set(float64(stats.Dead))
	oldestInboxSeconds.Set(stats.OldestS)
	if stats.OldestS >= workerStallAfter.Seconds() {
		zap.L().Error("audit inbox is falling behind",
			zap.Int64("pending", stats.Pending),
			zap.Int64("dead", stats.Dead),
			zap.Float64("oldest_pending_seconds", stats.OldestS))
	}
	if stats.Dead > 0 {
		zap.L().Warn("audit payloads are parked in dead inbox", zap.Int64("dead", stats.Dead))
	}
}

func retryDelay(base time.Duration, attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := base
	for i := 1; i < attempts && delay < workerRetryMax; i++ {
		delay *= 2
	}
	if delay > workerRetryMax {
		return workerRetryMax
	}
	return delay
}

func payloadRequestID(p *Payload) string {
	if p == nil {
		return ""
	}
	return p.RequestID
}

// payloadSize is a rough byte estimate used only to reject pathological input.
func payloadSize(p *Payload) int {
	if p == nil {
		return 0
	}
	return len(p.RequestMessages) + len(p.RequestParams) +
		len(p.ResponseMessage) + len(p.MediaRefs) +
		len(p.RequestID) + len(p.RequestPath) +
		len(p.InternalErrorDetail) + len(p.AttemptsDetail) + 256
}
