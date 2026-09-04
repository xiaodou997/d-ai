package promptaudit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type EventRepository interface {
	InsertPromptAuditEvent(context.Context, Event) error
}
type DecryptFunc func(string) (string, error)

type Engine struct {
	Config  *ConfigService
	Scanner Scanner
	Events  EventRepository
	Decrypt DecryptFunc
	Logger  *zap.Logger

	mu         sync.Mutex
	queue      chan queuedTask
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	metrics    engineMetrics
	guardSlots chan struct{}
	nodeMu     sync.Mutex
	nodeSlots  map[string]chan struct{}
}

type engineMetrics struct{ submitted, dropped, processed, failed, allowed, flagged, blocked, unavailable, invalid atomic.Int64 }

type queuedTask struct {
	Config   Config
	Snapshot Snapshot
}

const maxWorkerCount = 32

func NewEngine(config *ConfigService, scanner Scanner, events EventRepository, decrypt DecryptFunc, logger *zap.Logger) *Engine {
	return &Engine{Config: config, Scanner: scanner, Events: events, Decrypt: decrypt, Logger: logger, guardSlots: make(chan struct{}, 64), nodeSlots: map[string]chan struct{}{}}
}

func (e *Engine) Probe(ctx context.Context, endpoint Endpoint, apiKey string) (*Result, error) {
	normalizeConfig(&Config{Mode: ModeOff, WorkerCount: 1, QueueCapacity: 1, Scanners: ScannerIDs, Endpoints: []Endpoint{endpoint}, ConfigRevision: 1})
	if endpoint.Model == "" {
		endpoint.Model = DefaultGuardModel
	}
	if endpoint.TimeoutMS == 0 {
		endpoint.TimeoutMS = 3000
	}
	if endpoint.InputLimit == 0 {
		endpoint.InputLimit = 4000
	}
	if _, err := NormalizeBaseURL(endpoint.BaseURL); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(endpoint.TimeoutMS)*time.Millisecond)
	defer cancel()
	return e.Scanner.Scan(callCtx, endpoint, apiKey, "Hello", ScannerIDs)
}

func (e *Engine) Start(ctx context.Context) {
	if e == nil || e.Config == nil {
		return
	}
	e.mu.Lock()
	if e.cancel != nil {
		e.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	// The physical channel stays at the validated maximum. submit enforces the
	// active configuration's smaller logical capacity, so capacity changes do
	// not require replacing a channel while producers are using it.
	e.queue = make(chan queuedTask, 100000)
	e.mu.Unlock()
	for workerID := range maxWorkerCount {
		e.wg.Add(1)
		go e.worker(runCtx, workerID)
	}
}

func (e *Engine) Stop(ctx context.Context) error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	cancel := e.cancel
	e.cancel = nil
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) Check(ctx context.Context, in Input) Decision {
	if e == nil || e.Config == nil || e.Scanner == nil {
		return Decision{Allow: true}
	}
	cfg, err := e.Config.Get(ctx)
	if err != nil {
		if cfg.Enabled && cfg.Mode == ModeBlocking {
			return Decision{Allow: false, ErrorCode: ErrorUnavailable}
		}
		return Decision{Allow: true}
	}
	if !cfg.Enabled || cfg.Mode == ModeOff || !cfg.IncludesTenant(in.TenantID) {
		return Decision{Allow: true}
	}
	snapshot, err := ExtractSnapshot(in, cfg.LatestTurnOnly && cfg.Mode == ModeBlocking)
	if errors.Is(err, ErrNoPrompt) {
		return Decision{Allow: true}
	}
	if err != nil {
		return Decision{Allow: cfg.Mode != ModeBlocking, ErrorCode: ErrorInvalidResponse}
	}
	if cfg.Mode == ModeObserve {
		e.submit(queuedTask{Config: cfg, Snapshot: snapshot})
		return Decision{Allow: true}
	}
	result, code := e.evaluate(ctx, cfg, snapshot)
	if code != "" {
		e.observe(nil, code)
		e.record(ctx, cfg, snapshot, nil, code)
		return Decision{Allow: false, ErrorCode: code}
	}
	e.record(ctx, cfg, snapshot, result, "")
	e.observe(result, "")
	if result.Action == "Block" {
		return Decision{Allow: false, ErrorCode: ErrorBlocked, Result: result}
	}
	return Decision{Allow: true, Result: result}
}

func (e *Engine) submit(task queuedTask) {
	e.mu.Lock()
	q := e.queue
	e.mu.Unlock()
	if q == nil {
		return
	}
	capacity := task.Config.QueueCapacity
	if capacity < 1 {
		capacity = 4096
	}
	if len(q) >= capacity {
		e.metrics.dropped.Add(1)
		if e.Logger != nil {
			e.Logger.Warn("prompt_audit.enqueue_dropped", zap.String("error_code", "queue_full"))
		}
		return
	}
	select {
	case q <- task:
		e.metrics.submitted.Add(1)
	default:
		e.metrics.dropped.Add(1)
		if e.Logger != nil {
			e.Logger.Warn("prompt_audit.enqueue_dropped", zap.String("error_code", "queue_full"))
		}
	}
}
func (e *Engine) worker(ctx context.Context, workerID int) {
	defer e.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		cfg, _ := e.Config.Get(ctx)
		if workerID >= cfg.WorkerCount {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		select {
		case <-ctx.Done():
			return
		case task := <-e.queue:
			result, code := e.evaluate(ctx, task.Config, task.Snapshot)
			e.record(ctx, task.Config, task.Snapshot, result, code)
			e.observe(result, code)
			if code == "" {
				e.metrics.processed.Add(1)
			} else {
				e.metrics.failed.Add(1)
			}
		case <-ticker.C:
		}
	}
}

func (e *Engine) observe(result *Result, code string) {
	if code == ErrorInvalidResponse {
		e.metrics.invalid.Add(1)
		return
	}
	if code != "" {
		e.metrics.unavailable.Add(1)
		return
	}
	if result == nil {
		return
	}
	switch result.Action {
	case "Block":
		e.metrics.blocked.Add(1)
	case "Warn":
		e.metrics.flagged.Add(1)
	default:
		e.metrics.allowed.Add(1)
	}
}

func (e *Engine) Runtime(ctx context.Context) Runtime {
	cfg, _ := e.Config.Get(ctx)
	e.mu.Lock()
	q := e.queue
	e.mu.Unlock()
	depth := 0
	if q != nil {
		depth = len(q)
	}
	return Runtime{Mode: cfg.Mode, QueueDepth: depth, QueueCapacity: cfg.QueueCapacity, Submitted: e.metrics.submitted.Load(), Dropped: e.metrics.dropped.Load(), Processed: e.metrics.processed.Load(), Failed: e.metrics.failed.Load(), Allowed: e.metrics.allowed.Load(), Flagged: e.metrics.flagged.Load(), Blocked: e.metrics.blocked.Load(), Unavailable: e.metrics.unavailable.Load(), Invalid: e.metrics.invalid.Load()}
}

func (e *Engine) evaluate(ctx context.Context, cfg Config, snapshot Snapshot) (*Result, string) {
	select {
	case e.guardSlots <- struct{}{}:
		defer func() { <-e.guardSlots }()
	default:
		return nil, ErrorUnavailable
	}
	endpoints := cfg.EnabledEndpoints()
	if len(endpoints) == 0 {
		return nil, ErrorUnavailable
	}
	limit := endpoints[0].InputLimit
	for _, ep := range endpoints[1:] {
		if ep.InputLimit < limit {
			limit = ep.InputLimit
		}
	}
	chunks := splitRunes(snapshot.ScanText, limit)
	started := time.Now()
	results := []*Result{}
	for _, chunk := range chunks {
		var result *Result
		var last error
		for _, ep := range endpoints {
			nodeSlot := e.nodeSlot(ep.ID)
			select {
			case nodeSlot <- struct{}{}:
			default:
				last = &GuardError{Code: ErrorUnavailable, Retryable: true}
				continue
			}
			key := ""
			if ep.APIKeyCiphertext != "" {
				if e.Decrypt == nil {
					<-nodeSlot
					last = &GuardError{Code: ErrorUnavailable, Retryable: true}
					continue
				}
				var err error
				key, err = e.Decrypt(ep.APIKeyCiphertext)
				if err != nil {
					<-nodeSlot
					last = &GuardError{Code: ErrorUnavailable, Retryable: true, Cause: err}
					continue
				}
			}
			callCtx, cancel := context.WithTimeout(ctx, time.Duration(ep.TimeoutMS)*time.Millisecond)
			result, last = safeScan(e.Scanner, callCtx, ep, key, chunk, cfg.Scanners)
			cancel()
			<-nodeSlot
			if last == nil && result != nil {
				break
			}
			var ge *GuardError
			if !errors.As(last, &ge) || !ge.Retryable {
				break
			}
		}
		if last != nil || result == nil {
			var ge *GuardError
			if errors.As(last, &ge) && ge.Code == ErrorInvalidResponse {
				return nil, ErrorInvalidResponse
			}
			return nil, ErrorUnavailable
		}
		results = append(results, result)
		if result.Action == "Block" {
			break
		}
	}
	agg := aggregate(results)
	if agg == nil {
		return nil, ErrorInvalidResponse
	}
	agg.ChunkTotal = len(chunks)
	agg.LatencyMS = int(time.Since(started).Milliseconds())
	return agg, ""
}

func (e *Engine) nodeSlot(id string) chan struct{} {
	e.nodeMu.Lock()
	defer e.nodeMu.Unlock()
	if slot := e.nodeSlots[id]; slot != nil {
		return slot
	}
	slot := make(chan struct{}, 16)
	e.nodeSlots[id] = slot
	return slot
}

func safeScan(scanner Scanner, ctx context.Context, endpoint Endpoint, apiKey, chunk string, scanners []string) (result *Result, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = &GuardError{Code: ErrorUnavailable}
		}
	}()
	return scanner.Scan(ctx, endpoint, apiKey, chunk, scanners)
}

func aggregate(results []*Result) *Result {
	if len(results) == 0 {
		return nil
	}
	out := *results[0]
	cats := map[string]struct{}{}
	matched := map[string]struct{}{}
	unknown := map[string]struct{}{}
	out.ScannerScores = map[string]float64{}
	severity := func(d string) int {
		switch d {
		case "critical":
			return 3
		case "flag":
			return 2
		}
		return 1
	}
	for _, r := range results {
		if r == nil {
			return nil
		}
		if severity(r.Decision) > severity(out.Decision) {
			out.Decision, out.RiskLevel, out.Action, out.Safety, out.EndpointID, out.ScannerVersion = r.Decision, r.RiskLevel, r.Action, r.Safety, r.EndpointID, r.ScannerVersion
		}
		for _, v := range r.Categories {
			cats[v] = struct{}{}
		}
		for _, v := range r.MatchedScanners {
			matched[v] = struct{}{}
		}
		for _, v := range r.UnknownCategories {
			unknown[v] = struct{}{}
		}
		for k, v := range r.ScannerScores {
			if v > out.ScannerScores[k] {
				out.ScannerScores[k] = v
			}
		}
	}
	out.Categories = orderedKeys(cats)
	out.MatchedScanners = orderedKeys(matched)
	out.UnknownCategories = sortedKeys(unknown)
	return &out
}

func (e *Engine) record(ctx context.Context, cfg Config, snapshot Snapshot, result *Result, code string) {
	if e.Events == nil {
		return
	}
	if result != nil && result.Decision == "pass" && !cfg.StorePassEvents {
		return
	}
	safe := snapshot
	safe.ScanText = ""
	event := Event{Snapshot: safe, ConfigRevision: cfg.ConfigRevision, ErrorCode: code, CreatedAt: time.Now().UTC()}
	if result != nil {
		event.Result = *result
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := e.Events.InsertPromptAuditEvent(recordCtx, event); err != nil && e.Logger != nil {
		e.Logger.Warn("prompt_audit.record_failed", zap.Error(err))
	}
}
