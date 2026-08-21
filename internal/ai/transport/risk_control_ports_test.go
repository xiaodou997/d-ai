package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

var (
	_ RiskControlConfigStore = (*riskcontrol.ConfigService)(nil)
	_ RiskControlDetector    = (*riskcontrol.Checker)(nil)
	_ RiskControlLogReader   = (*riskcontrol.LogService)(nil)
	_ RiskEventManager       = (*riskcontrol.EventService)(nil)
)

type riskControlConfigStoreStub struct {
	config  domain.RiskControlConfig
	updated domain.RiskControlConfig
	gets    int
}

func (s *riskControlConfigStoreStub) Get(context.Context) (domain.RiskControlConfig, error) {
	s.gets++
	return s.config, nil
}

func (s *riskControlConfigStoreStub) Update(_ context.Context, config domain.RiskControlConfig) error {
	s.updated = config
	return nil
}

type riskControlDetectorStub struct {
	config domain.RiskControlConfig
	text   string
	result domain.RiskControlDetection
}

func (s *riskControlDetectorStub) Detect(_ context.Context, config domain.RiskControlConfig, text string) domain.RiskControlDetection {
	s.config, s.text = config, text
	return s.result
}

type riskControlLogReaderStub struct {
	filter domain.ContentModerationLogFilter
	limit  int32
	offset int32
	page   domain.ContentModerationLogPage
}

func (s *riskControlLogReaderStub) List(_ context.Context, filter domain.ContentModerationLogFilter, limit, offset int32) (domain.ContentModerationLogPage, error) {
	s.filter, s.limit, s.offset = filter, limit, offset
	return s.page, nil
}

type riskEventManagerStub struct {
	filter        domain.RiskEventFilter
	limit         int32
	offset        int32
	page          domain.RiskEventPage
	resolveID     string
	resolveStatus string
	resolvedBy    string
	resolveNote   string
	resolved      domain.RiskEvent
}

func (s *riskEventManagerStub) List(_ context.Context, filter domain.RiskEventFilter, limit, offset int32) (domain.RiskEventPage, error) {
	s.filter, s.limit, s.offset = filter, limit, offset
	return s.page, nil
}

func (s *riskEventManagerStub) Resolve(_ context.Context, id, status, resolvedBy, note string) (domain.RiskEvent, error) {
	s.resolveID, s.resolveStatus, s.resolvedBy, s.resolveNote = id, status, resolvedBy, note
	return s.resolved, nil
}

func TestRiskControlRoutesUsePorts(t *testing.T) {
	createdAt := time.Date(2026, time.August, 21, 3, 4, 5, 0, time.UTC)
	score := 0.95
	configStore := &riskControlConfigStoreStub{config: domain.RiskControlConfig{
		Enabled: true, Mode: domain.RiskControlModeObserve, ConfigRevision: 7,
		Provider: domain.RiskControlProviderConfig{APIKeyCiphertext: "existing-secret"},
	}}
	detector := &riskControlDetectorStub{result: domain.RiskControlDetection{
		Flagged: true, MatchedKeyword: "blocked-word", HitLayer: domain.HitLayerKeyword,
		HighestCategory: "violence", HighestScore: &score,
	}}
	logs := &riskControlLogReaderStub{page: domain.ContentModerationLogPage{
		Total: 1,
		Items: []domain.ContentModerationLog{{
			ID: "log-1", TenantID: "tenant-1", Mode: domain.RiskControlModeObserve,
			Action: domain.RiskControlActionBlock, Flagged: true, HitLayer: domain.HitLayerKeyword, CreatedAt: createdAt,
		}},
	}}
	events := &riskEventManagerStub{
		page: domain.RiskEventPage{Total: 1, Items: []domain.RiskEvent{{
			ID: "event-1", EventType: "content_moderation", Severity: "high", Status: "open", CreatedAt: createdAt,
		}}},
		resolved: domain.RiskEvent{
			ID: "event-1", EventType: "content_moderation", Severity: "high",
			Status: domain.RiskEventStatusResolved, ResolvedBy: "admin-1", CreatedAt: createdAt,
		},
	}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerRiskControl(api, AIDeps{OperationsDeps: OperationsDeps{
		RiskControlConfig: configStore, RiskControlDetector: detector,
		RiskControlLogs: logs, RiskEvents: events,
	}})

	configRecorder := performRiskControlRequest(router, http.MethodGet, "/api/v1/risk-control/config", "")
	requireRiskControlStatus(t, configRecorder, http.StatusOK)
	var config riskControlConfigDTO
	decodeRiskControlResponse(t, configRecorder, &config)
	if config.ConfigRevision != 7 || !config.Provider.HasAPIKey {
		t.Fatalf("config response = %#v", config)
	}

	updateBody := `{"enabled":true,"mode":"pre_block","keyword":{"enabled":false,"entries":[],"homoglyph_map_extra":{},"pinyin":{"enabled":false,"entries":[],"include_initials":false}},"provider":{"base_url":"https://moderation.example","model":"moderation-test","timeout_ms":1200},"thresholds":{},"sample_rate":1,"verdict_cache_ttl_seconds":600,"scope_group_ids":[],"violation_window_hours":24,"risk_event_threshold":3,"record_non_hits":false,"block_status_code":403,"block_message":"blocked"}`
	updateRecorder := performRiskControlRequest(router, http.MethodPut, "/api/v1/risk-control/config", updateBody)
	requireRiskControlStatus(t, updateRecorder, http.StatusOK)
	if configStore.updated.Mode != domain.RiskControlModePreBlock || configStore.updated.Provider.APIKeyCiphertext != "existing-secret" {
		t.Fatalf("updated config = %#v", configStore.updated)
	}

	testRecorder := performRiskControlRequest(router, http.MethodPost, "/api/v1/risk-control/test", `{"text":"  unsafe input  "}`)
	requireRiskControlStatus(t, testRecorder, http.StatusOK)
	if detector.text != "unsafe input" || detector.config.ConfigRevision != 7 {
		t.Fatalf("detection input = text %q config %#v", detector.text, detector.config)
	}
	var detection struct {
		Flagged        bool    `json:"flagged"`
		MatchedKeyword *string `json:"matched_keyword"`
		HitLayer       *string `json:"hit_layer"`
	}
	decodeRiskControlResponse(t, testRecorder, &detection)
	if !detection.Flagged || detection.MatchedKeyword == nil || *detection.MatchedKeyword != "blocked-word" || detection.HitLayer == nil || *detection.HitLayer != domain.HitLayerKeyword {
		t.Fatalf("detection response = %#v", detection)
	}

	logPath := "/api/v1/risk-control/logs?tenant_id=tenant-1&user_id=user-1&mode=observe&action=block&flagged=true&hit_layer=keyword&date_from=2026-08-20T00:00:00Z&date_to=2026-08-21T00:00:00Z&limit=99&offset=2"
	logsRecorder := performRiskControlRequest(router, http.MethodGet, logPath, "")
	requireRiskControlStatus(t, logsRecorder, http.StatusOK)
	if logs.filter.TenantID != "tenant-1" || logs.filter.UserID != "user-1" || logs.filter.Flagged == nil || !*logs.filter.Flagged || logs.limit != 99 || logs.offset != 2 {
		t.Fatalf("log query = filter %#v limit %d offset %d", logs.filter, logs.limit, logs.offset)
	}
	assertRiskControlWindow(t, logs.filter.DateFrom, logs.filter.DateTo)
	var logPage struct {
		Items []riskControlLogDTO `json:"items"`
		Total int64               `json:"total"`
	}
	decodeRiskControlResponse(t, logsRecorder, &logPage)
	if logPage.Total != 1 || len(logPage.Items) != 1 || logPage.Items[0].ID != "log-1" {
		t.Fatalf("log response = %#v", logPage)
	}

	eventsRecorder := performRiskControlRequest(router, http.MethodGet, "/api/v1/risk-control/events?status=open&tenant_id=tenant-1&user_id=user-1&limit=88&offset=3", "")
	requireRiskControlStatus(t, eventsRecorder, http.StatusOK)
	if events.filter.Status != "open" || events.filter.TenantID != "tenant-1" || events.filter.UserID != "user-1" || events.limit != 88 || events.offset != 3 {
		t.Fatalf("event query = filter %#v limit %d offset %d", events.filter, events.limit, events.offset)
	}
	var eventPage struct {
		Items []riskEventDTO `json:"items"`
		Total int64          `json:"total"`
	}
	decodeRiskControlResponse(t, eventsRecorder, &eventPage)
	if eventPage.Total != 1 || len(eventPage.Items) != 1 || eventPage.Items[0].ID != "event-1" {
		t.Fatalf("event response = %#v", eventPage)
	}

	claimsHandler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx := context.WithValue(request.Context(), authClaimsContextKey{}, &auth.Claims{UserID: "admin-1"})
		router.ServeHTTP(w, request.WithContext(ctx))
	})
	resolveRecorder := performRiskControlRequest(claimsHandler, http.MethodPost, "/api/v1/risk-control/events/event-1/resolve", `{"status":"resolved","note":"reviewed"}`)
	requireRiskControlStatus(t, resolveRecorder, http.StatusOK)
	if events.resolveID != "event-1" || events.resolveStatus != domain.RiskEventStatusResolved || events.resolvedBy != "admin-1" || events.resolveNote != "reviewed" {
		t.Fatalf("resolve command = id %q status %q actor %q note %q", events.resolveID, events.resolveStatus, events.resolvedBy, events.resolveNote)
	}
}

func TestRiskControlRoutesRequirePorts(t *testing.T) {
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerRiskControl(api, AIDeps{})

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/risk-control/config", ""},
		{http.MethodPost, "/api/v1/risk-control/test", `{"text":"test"}`},
		{http.MethodGet, "/api/v1/risk-control/logs", ""},
		{http.MethodGet, "/api/v1/risk-control/events", ""},
	}
	for _, request := range requests {
		recorder := performRiskControlRequest(router, request.method, request.path, request.body)
		requireRiskControlStatus(t, recorder, http.StatusServiceUnavailable)
	}
}

func performRiskControlRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func requireRiskControlStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, want, recorder.Body.String())
	}
}

func decodeRiskControlResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertRiskControlWindow(t *testing.T, dateFrom, dateTo *time.Time) {
	t.Helper()
	if dateFrom == nil || dateTo == nil || dateFrom.Format(time.RFC3339) != "2026-08-20T00:00:00Z" || dateTo.Format(time.RFC3339) != "2026-08-21T00:00:00Z" {
		t.Fatalf("window = %v to %v", dateFrom, dateTo)
	}
}
