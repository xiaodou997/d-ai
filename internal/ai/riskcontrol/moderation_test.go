package riskcontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

// ---- fakes ----

type fakeLogRepo struct {
	logs              []domain.ContentModerationLog
	flaggedSinceCount int64
}

func (r *fakeLogRepo) InsertLog(_ context.Context, log domain.ContentModerationLog) (string, time.Time, error) {
	log.ID = "log-id"
	log.CreatedAt = time.Now()
	r.logs = append(r.logs, log)
	return log.ID, log.CreatedAt, nil
}
func (r *fakeLogRepo) ListLogs(context.Context, domain.ContentModerationLogFilter, int32, int32) ([]domain.ContentModerationLog, error) {
	return r.logs, nil
}
func (r *fakeLogRepo) CountLogs(context.Context, domain.ContentModerationLogFilter) (int64, error) {
	return int64(len(r.logs)), nil
}
func (r *fakeLogRepo) CountFlaggedSince(context.Context, string, time.Time) (int64, error) {
	return r.flaggedSinceCount, nil
}

type fakeEventRepo struct {
	events []domain.RiskEvent
}

func (r *fakeEventRepo) InsertEvent(_ context.Context, ev domain.RiskEvent) (string, error) {
	ev.ID = "event-id"
	r.events = append(r.events, ev)
	return ev.ID, nil
}
func (r *fakeEventRepo) ListEvents(context.Context, domain.RiskEventFilter, int32, int32) ([]domain.RiskEvent, error) {
	return r.events, nil
}
func (r *fakeEventRepo) CountEvents(context.Context, domain.RiskEventFilter) (int64, error) {
	return int64(len(r.events)), nil
}
func (r *fakeEventRepo) ResolveEvent(_ context.Context, id, status, resolvedBy, note string) (domain.RiskEvent, error) {
	for i := range r.events {
		if r.events[i].ID == id {
			r.events[i].Status = status
			r.events[i].ResolvedBy = resolvedBy
			r.events[i].ResolutionNote = note
			return r.events[i], nil
		}
	}
	return domain.RiskEvent{}, domain.ErrNotFound
}

// ---- Detect ----

func TestDetect_EmptyTextIsNoop(t *testing.T) {
	c := &Checker{}
	got := c.Detect(context.Background(), domain.RiskControlConfig{}, "")
	if got.Flagged || got.APIError != "" {
		t.Fatalf("got %#v", got)
	}
}

func TestDetect_KeywordMatchShortCircuitsAPI(t *testing.T) {
	apiCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.Write([]byte(`{"results":[{"flagged":false,"category_scores":{}}]}`))
	}))
	defer server.Close()

	c := &Checker{}
	cfg := domain.RiskControlConfig{
		Keyword: domain.KeywordConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{{Word: "BadWord", Level: domain.KeywordLevelBlock}},
		},
		SampleRate: 1,
		Provider:   domain.RiskControlProviderConfig{BaseURL: server.URL, APIKeyCiphertext: "plain:k"},
	}
	got := c.Detect(context.Background(), cfg, "this has badword in it")
	if !got.Flagged || got.MatchedKeyword != "BadWord" {
		t.Fatalf("got %#v", got)
	}
	if apiCalled {
		t.Fatal("keyword hit should short-circuit the API call")
	}
}

func TestDetect_APICallFlagged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"flagged":true,"category_scores":{"violence":0.95,"sexual":0.1}}]}`))
	}))
	defer server.Close()

	c := &Checker{}
	cfg := domain.RiskControlConfig{
		Keyword:    domain.KeywordConfig{Enabled: false}, // no keyword, only API
		SampleRate: 1,
		Provider: domain.RiskControlProviderConfig{
			BaseURL: server.URL, Model: "test-model", APIKeyCiphertext: "plain:sk-test", TimeoutMs: 2000,
		},
		Thresholds: map[string]float64{"violence": 0.8, "sexual": 0.8},
	}
	got := c.Detect(context.Background(), cfg, "some text")
	if !got.Flagged || got.HighestCategory != "violence" || got.HighestScore == nil || *got.HighestScore != 0.95 {
		t.Fatalf("got %#v", got)
	}
	if got.UpstreamLatencyMs == nil {
		t.Fatal("expected latency to be recorded")
	}
	if got.HitLayer != domain.HitLayerAPI {
		t.Fatalf("expected hit_layer=api, got %s", got.HitLayer)
	}
}

func TestDetect_SampleRateZeroSkipsAPI(t *testing.T) {
	apiCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
	}))
	defer server.Close()

	c := &Checker{}
	cfg := domain.RiskControlConfig{
		Keyword:    domain.KeywordConfig{Enabled: false},
		SampleRate: 0,
		Provider:   domain.RiskControlProviderConfig{BaseURL: server.URL, APIKeyCiphertext: "plain:k"},
	}
	got := c.Detect(context.Background(), cfg, "some text")
	if got.Flagged || apiCalled {
		t.Fatalf("expected sample_rate=0 to skip the API call entirely, got %#v (apiCalled=%v)", got, apiCalled)
	}
}

func TestDetect_APIErrorReturnsErrorNotFlagged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &Checker{}
	cfg := domain.RiskControlConfig{
		Keyword:    domain.KeywordConfig{Enabled: false},
		SampleRate: 1,
		Provider:   domain.RiskControlProviderConfig{BaseURL: server.URL, APIKeyCiphertext: "plain:k"},
	}
	got := c.Detect(context.Background(), cfg, "some text")
	if got.Flagged {
		t.Fatal("api error must not be treated as flagged")
	}
	if got.APIError == "" {
		t.Fatal("expected a non-empty APIError")
	}
}

func TestDetect_CacheHitSkipsDetection(t *testing.T) {
	c := &Checker{}
	cfg := domain.RiskControlConfig{
		Keyword: domain.KeywordConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{{Word: "badword", Level: domain.KeywordLevelBlock}},
		},
		ConfigRevision:         1,
		VerdictCacheTTLSeconds: 600,
	}

	// First call: actual detection, should flag.
	got1 := c.Detect(context.Background(), cfg, "this has badword in it")
	if !got1.Flagged || got1.MatchedKeyword != "badword" {
		t.Fatalf("first call: got %#v", got1)
	}
	if got1.FromCache {
		t.Fatal("first call should not be from cache")
	}

	// Second call: same text, should hit cache.
	got2 := c.Detect(context.Background(), cfg, "this has badword in it")
	if !got2.Flagged || got2.MatchedKeyword != "badword" {
		t.Fatalf("second call: got %#v", got2)
	}
	if !got2.FromCache {
		t.Fatal("second call should be from cache")
	}
	if got2.HitLayer != domain.HitLayerCache {
		t.Fatalf("expected hit_layer=cache, got %s", got2.HitLayer)
	}
}

// ---- Record / risk events ----

func TestRecord_NonHitOnlyLoggedWhenConfigured(t *testing.T) {
	logs := &fakeLogRepo{}
	c := &Checker{Logs: NewLogService(logs)}

	c.Record(context.Background(), domain.RiskControlConfig{RecordNonHits: false}, CheckInput{Text: "hi"}, DetectResult{}, domain.RiskControlModeObserve)
	if len(logs.logs) != 0 {
		t.Fatalf("expected no log, got %d", len(logs.logs))
	}

	c.Record(context.Background(), domain.RiskControlConfig{RecordNonHits: true}, CheckInput{Text: "hi"}, DetectResult{}, domain.RiskControlModeObserve)
	if len(logs.logs) != 1 || logs.logs[0].Action != domain.RiskControlActionAllow {
		t.Fatalf("expected one allow log, got %#v", logs.logs)
	}
}

func TestRecord_RaisesRiskEventAtThresholdMultiples(t *testing.T) {
	cases := []struct {
		count     int64
		wantEvent bool
	}{
		{count: 1, wantEvent: false},
		{count: 2, wantEvent: false},
		{count: 3, wantEvent: true},
		{count: 4, wantEvent: false},
		{count: 6, wantEvent: true},
	}
	for _, tc := range cases {
		logs := &fakeLogRepo{flaggedSinceCount: tc.count}
		events := &fakeEventRepo{}
		c := &Checker{Logs: NewLogService(logs), Events: NewEventService(events)}
		cfg := domain.RiskControlConfig{ViolationWindowHours: 24, RiskEventThreshold: 3}
		det := DetectResult{Flagged: true, MatchedKeyword: "bad", HitLayer: domain.HitLayerKeyword}

		action := c.Record(context.Background(), cfg, CheckInput{UserID: "user-1", Text: "bad text"}, det, domain.RiskControlModePreBlock)
		if action != domain.RiskControlActionKeywordBlock {
			t.Fatalf("count=%d: action=%q", tc.count, action)
		}
		if got := len(events.events) > 0; got != tc.wantEvent {
			t.Fatalf("count=%d: gotEvent=%v want=%v", tc.count, got, tc.wantEvent)
		}
	}
}

func TestRecord_NoRiskEventWithoutUserID(t *testing.T) {
	logs := &fakeLogRepo{flaggedSinceCount: 3}
	events := &fakeEventRepo{}
	c := &Checker{Logs: NewLogService(logs), Events: NewEventService(events)}
	cfg := domain.RiskControlConfig{ViolationWindowHours: 24, RiskEventThreshold: 3}
	det := DetectResult{Flagged: true, MatchedKeyword: "bad", HitLayer: domain.HitLayerKeyword}

	c.Record(context.Background(), cfg, CheckInput{Text: "bad text"}, det, domain.RiskControlModePreBlock)
	if len(events.events) != 0 {
		t.Fatalf("expected no risk event without a user id, got %#v", events.events)
	}
}

// ---- pure helpers ----

func TestEvaluateScores(t *testing.T) {
	scores := map[string]float64{"violence": 0.9, "sexual": 0.1}
	thresholds := map[string]float64{"violence": 0.8, "sexual": 0.8}
	flagged, category, score := evaluateScores(scores, thresholds)
	if !flagged || category != "violence" || score == nil || *score != 0.9 {
		t.Fatalf("flagged=%v category=%q score=%v", flagged, category, score)
	}

	flagged, _, _ = evaluateScores(map[string]float64{"violence": 0.1}, thresholds)
	if flagged {
		t.Fatal("expected not flagged below threshold")
	}
}

func TestShouldSample_Deterministic(t *testing.T) {
	text := "some fixed text"
	first := shouldSample(text, 0.5)
	for range 5 {
		if shouldSample(text, 0.5) != first {
			t.Fatal("shouldSample must be deterministic for identical input")
		}
	}
	if !shouldSample(text, 1) {
		t.Fatal("rate=1 must always sample")
	}
	if shouldSample(text, 0) {
		t.Fatal("rate=0 must never sample")
	}
}
