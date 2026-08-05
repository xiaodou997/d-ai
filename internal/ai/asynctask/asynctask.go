package asynctask

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Config tunes the engine. Zero values get sensible defaults via withDefaults.
type Config struct {
	// Workers is the number of concurrent executing goroutines on this instance.
	Workers int

	// PollInterval is how often a worker looks for work absent a wake-up signal.
	// It only bounds latency for tasks submitted to *other* instances and for
	// reaped orphans; same-instance submits are woken immediately.
	PollInterval time.Duration

	// LeaseTTL is how long a claim is held before the reaper may take it. The
	// heartbeat renews at LeaseTTL/3, so a task survives two missed beats.
	LeaseTTL time.Duration

	// MaxInFlightPerTenant caps a tenant's pending+running tasks. It is enforced
	// twice: at submit (429) and in the claim query (fairness).
	MaxInFlightPerTenant int

	// Retention is the default task TTL when neither the registration nor the
	// handler overrides it.
	Retention time.Duration

	// ReapInterval is how often orphaned leases and expired rows are swept.
	ReapInterval time.Duration

	// ReapBatch caps rows touched per reap statement.
	ReapBatch int

	// WebhookWorkers is the number of independent delivery goroutines.
	WebhookWorkers int
	// WebhookPollInterval bounds cross-instance delivery latency.
	WebhookPollInterval time.Duration
	// WebhookLeaseTTL bounds recovery after a delivery worker exits mid-send.
	WebhookLeaseTTL time.Duration
}

func (c Config) withDefaults() Config {
	if c.Workers <= 0 {
		c.Workers = 2
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 2 * time.Second
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 60 * time.Second
	}
	if c.MaxInFlightPerTenant <= 0 {
		c.MaxInFlightPerTenant = 16
	}
	if c.Retention <= 0 {
		c.Retention = 24 * time.Hour
	}
	if c.ReapInterval <= 0 {
		c.ReapInterval = 30 * time.Second
	}
	if c.ReapBatch <= 0 {
		c.ReapBatch = 64
	}
	if c.WebhookWorkers <= 0 {
		c.WebhookWorkers = 2
	}
	if c.WebhookPollInterval <= 0 {
		c.WebhookPollInterval = c.PollInterval
	}
	if c.WebhookLeaseTTL < 15*time.Second {
		c.WebhookLeaseTTL = 30 * time.Second
	}
	return c
}

// Deps are the engine's collaborators. Everything AI-specific enters here, so
// the engine itself stays a generic queue.
type Deps struct {
	Pool   *pgxpool.Pool
	Logger *zap.Logger

	// Subjects re-resolves a persisted SubjectRef before each attempt. Required.
	Subjects SubjectResolver

	// RedactDetail is the last-line-of-defense scrub applied to
	// Failure.InternalDetail before it is persisted. Wire
	// serving.RedactInternalErrorDetail here — injected rather than imported so
	// the engine does not depend on the runtime pipeline package. Defaults to a
	// no-op.
	RedactDetail func(string) string

	// WebhookSender defaults to the guarded production HTTPS adapter.
	WebhookSender WebhookSender
}

// Engine owns the queue: submission, scheduling, leases, retries and expiry.
type Engine struct {
	cfg      Config
	deps     Deps
	store    store
	registry *registry
	logger   *zap.Logger

	// workerID identifies this process's claims. Leases are scoped to it, which
	// is what makes multi-replica operation safe.
	workerID string

	// wake is a signal, not a queue. It carries no payload: a worker always
	// re-reads the row from the database, which is what forces submit and
	// crash-recovery down the same code path. A full buffer is not an error —
	// the task is already durable and polling will pick it up.
	wake          chan struct{}
	deliveryWake  chan struct{}
	webhookSender WebhookSender

	// cancels lets the submitting instance stop a running task instantly.
	// Other instances learn via the heartbeat returning zero rows.
	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc

	startOnce sync.Once
	wg        sync.WaitGroup
}

// New builds an Engine. Register handlers on it, then call Start.
func New(cfg Config, deps Deps) (*Engine, error) {
	if deps.Pool == nil {
		return nil, fmt.Errorf("asynctask: Deps.Pool is required")
	}
	if deps.Subjects == nil {
		return nil, fmt.Errorf("asynctask: Deps.Subjects is required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	if deps.RedactDetail == nil {
		deps.RedactDetail = func(s string) string { return s }
	}
	if deps.WebhookSender == nil {
		deps.WebhookSender = NewWebhookSender()
	}
	cfg = cfg.withDefaults()

	host, _ := os.Hostname()
	if host == "" {
		host = "ai-service"
	}

	return &Engine{
		cfg:           cfg,
		deps:          deps,
		store:         newPostgresStore(deps.Pool),
		registry:      newRegistry(),
		logger:        logger,
		workerID:      fmt.Sprintf("%s-%s", host, uuid.NewString()[:8]),
		wake:          make(chan struct{}, cfg.Workers),
		deliveryWake:  make(chan struct{}, cfg.WebhookWorkers),
		webhookSender: deps.WebhookSender,
		cancels:       map[string]context.CancelFunc{},
	}, nil
}

// Register binds a handler to a task type. See registry.register for the rules;
// misuse panics at wiring time rather than failing quietly at runtime.
func (e *Engine) Register(taskType string, h Handler, opts Options) {
	e.registry.register(taskType, h, opts)
}

// Start freezes the registry and launches workers plus the reaper.
//
// Deliberately absent: any startup reconciliation. The old console engine reset
// every running row to pending on boot, with no owner filter, so a second
// instance would seize tasks the first was still executing. Orphans are now
// found only by an expired lease, which cannot mistake a live task for a dead
// one.
func (e *Engine) Start(ctx context.Context) {
	e.startOnce.Do(func() {
		e.registry.freeze()
		types := e.registry.types()
		if len(types) == 0 {
			e.logger.Info("async task engine started with no registered types; task workers not launched")
		} else {
			e.logger.Info("async task engine started",
				zap.String("worker_id", e.workerID),
				zap.Int("workers", e.cfg.Workers),
				zap.Strings("task_types", types),
				zap.Duration("lease_ttl", e.cfg.LeaseTTL),
			)
			for range e.cfg.Workers {
				e.wg.Go(func() { e.workerLoop(ctx) })
			}
		}
		for range e.cfg.WebhookWorkers {
			e.wg.Go(func() { e.webhookWorkerLoop(ctx) })
		}
		e.wg.Go(func() { e.reaperLoop(ctx) })
	})
}

// Stop waits for workers to finish, then hands their in-progress tasks back so
// another instance can pick them up without waiting out the lease.
//
// Callers should cancel the Start context first; Stop's own context only bounds
// the release query.
func (e *Engine) Stop(ctx context.Context) {
	e.wg.Wait()
	released, err := e.store.releaseWorker(ctx, e.workerID)
	if err != nil {
		e.logger.Warn("async task engine: releasing worker tasks failed",
			zap.String("worker_id", e.workerID), zap.Error(err))
	} else if released > 0 {
		e.logger.Info("async task engine: returned in-progress tasks to the queue",
			zap.String("worker_id", e.workerID), zap.Int64("count", released))
	}
	deliveries, err := e.store.releaseDeliveries(ctx, e.workerID)
	if err != nil {
		e.logger.Warn("async task engine: releasing webhook deliveries failed",
			zap.String("worker_id", e.workerID), zap.Error(err))
		return
	}
	if deliveries > 0 {
		e.logger.Info("async task engine: returned webhook deliveries to the queue",
			zap.String("worker_id", e.workerID), zap.Int64("count", deliveries))
	}
}

// signal nudges a local worker. Non-blocking: a full buffer means workers are
// already busy and will re-poll, and the task is durable regardless.
func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) signalDelivery() {
	select {
	case e.deliveryWake <- struct{}{}:
	default:
	}
}

func (e *Engine) registerCancel(taskID string, cancel context.CancelFunc) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	e.cancels[taskID] = cancel
}

func (e *Engine) unregisterCancel(taskID string) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	delete(e.cancels, taskID)
}

func (e *Engine) cancelLocal(taskID string) {
	e.cancelMu.Lock()
	cancel := e.cancels[taskID]
	e.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
