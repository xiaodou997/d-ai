package promptaudit

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type settingRepoStub struct {
	raw json.RawMessage
	err error
}

func (s *settingRepoStub) GetSetting(context.Context, string) (json.RawMessage, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.raw == nil {
		return nil, errors.New("missing")
	}
	return s.raw, nil
}
func (s *settingRepoStub) UpsertSetting(_ context.Context, _ string, raw json.RawMessage) error {
	s.raw = append([]byte(nil), raw...)
	return nil
}

type scannerStub struct {
	mu         sync.Mutex
	calls      []string
	byEndpoint map[string]struct {
		result *Result
		err    error
	}
}

func (s *scannerStub) Scan(_ context.Context, ep Endpoint, _ string, _ string, _ []string) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, ep.ID)
	v := s.byEndpoint[ep.ID]
	return v.result, v.err
}

type eventRepoStub struct{ ch chan Event }

func (r *eventRepoStub) InsertPromptAuditEvent(_ context.Context, event Event) error {
	r.ch <- event
	return nil
}

func testConfig(mode string) Config {
	return Config{Enabled: true, Mode: mode, WorkerCount: 1, QueueCapacity: 4, Scanners: []string{"jailbreak"}, ConfigRevision: 2, Endpoints: []Endpoint{{ID: "one", Name: "one", BaseURL: "https://guard.example", Model: DefaultGuardModel, TimeoutMS: 1000, InputLimit: 4000, Enabled: true}}}
}
func configServiceFor(t *testing.T, cfg Config) *ConfigService {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return NewConfigService(&settingRepoStub{raw: raw})
}
func auditInput() Input {
	return Input{RequestID: "req-1", TenantID: "tenant-1", Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"audit this prompt with enough text to create metadata"}]}`)}
}

func TestEngineBlockingFailsClosedAndDoesNotPersistRawText(t *testing.T) {
	events := &eventRepoStub{ch: make(chan Event, 1)}
	scanner := &scannerStub{byEndpoint: map[string]struct {
		result *Result
		err    error
	}{"one": {result: &Result{Decision: "critical", RiskLevel: "critical", Action: "Block", Safety: "Unsafe", MatchedScanners: []string{"jailbreak"}, ScannerScores: map[string]float64{"jailbreak": 1}}}}}
	engine := NewEngine(configServiceFor(t, testConfig(ModeBlocking)), scanner, events, nil, nil)
	decision := engine.Check(context.Background(), auditInput())
	if decision.Allow || decision.ErrorCode != ErrorBlocked {
		t.Fatalf("decision=%+v", decision)
	}
	event := <-events.ch
	if event.Snapshot.ScanText != "" {
		t.Fatal("raw scan text reached event repository")
	}
	if event.Snapshot.PromptHash == "" || event.Snapshot.RedactedPreview == "" {
		t.Fatalf("event=%+v", event)
	}
}

func TestEngineFailoverOnlyForRetryableErrors(t *testing.T) {
	cfg := testConfig(ModeBlocking)
	cfg.Endpoints = append(cfg.Endpoints, Endpoint{ID: "two", Name: "two", BaseURL: "https://guard2.example", Model: DefaultGuardModel, TimeoutMS: 1000, InputLimit: 4000, Enabled: true})
	scanner := &scannerStub{byEndpoint: map[string]struct {
		result *Result
		err    error
	}{"one": {err: &GuardError{Code: ErrorUnavailable, Retryable: true}}, "two": {result: &Result{Decision: "pass", RiskLevel: "low", Action: "Allow", Safety: "Safe", ScannerScores: map[string]float64{}}}}}
	engine := NewEngine(configServiceFor(t, cfg), scanner, nil, nil, nil)
	decision := engine.Check(context.Background(), auditInput())
	if !decision.Allow {
		t.Fatalf("decision=%+v", decision)
	}
	if len(scanner.calls) != 2 {
		t.Fatalf("calls=%v", scanner.calls)
	}
}

func TestEngineObserveIsAsynchronousAndStoresNoRawText(t *testing.T) {
	events := &eventRepoStub{ch: make(chan Event, 1)}
	cfg := testConfig(ModeObserve)
	scanner := &scannerStub{byEndpoint: map[string]struct {
		result *Result
		err    error
	}{"one": {result: &Result{Decision: "flag", RiskLevel: "medium", Action: "Warn", Safety: "Controversial", ScannerScores: map[string]float64{}}}}}
	engine := NewEngine(configServiceFor(t, cfg), scanner, events, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())
	if decision := engine.Check(context.Background(), auditInput()); !decision.Allow {
		t.Fatalf("decision=%+v", decision)
	}
	select {
	case event := <-events.ch:
		if event.Snapshot.ScanText != "" {
			t.Fatal("raw scan text persisted")
		}
	case <-time.After(time.Second):
		t.Fatal("observe event not processed")
	}
}

func TestEngineKeepsPersistedBlockingIntentFailClosedOnReloadError(t *testing.T) {
	cfg := testConfig(ModeBlocking)
	raw, _ := json.Marshal(cfg)
	repo := &settingRepoStub{raw: raw}
	service := NewConfigService(repo)
	if _, err := service.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.cachedAt = time.Now().Add(-time.Hour)
	service.mu.Unlock()
	repo.err = errors.New("database unavailable")
	engine := NewEngine(service, &scannerStub{}, nil, nil, nil)
	decision := engine.Check(context.Background(), auditInput())
	if decision.Allow || decision.ErrorCode != ErrorUnavailable {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestEngineDefaultOffDoesNotFailClosedWithoutConfig(t *testing.T) {
	repo := &settingRepoStub{err: errors.New("database unavailable")}
	engine := NewEngine(NewConfigService(repo), &scannerStub{}, nil, nil, nil)
	if decision := engine.Check(context.Background(), auditInput()); !decision.Allow {
		t.Fatalf("decision=%+v", decision)
	}
}
