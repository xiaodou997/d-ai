package serving

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/riskcontrol"
)

// ---- fakes shared by the tests in this file ----

type fakeSettingRepo struct {
	mu    sync.Mutex
	value json.RawMessage
}

func newFakeSettingRepo(cfg domain.RiskControlConfig) *fakeSettingRepo {
	b, _ := json.Marshal(cfg)
	return &fakeSettingRepo{value: b}
}

func (r *fakeSettingRepo) GetSetting(_ context.Context, _ string) (json.RawMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value, nil
}

func (r *fakeSettingRepo) UpsertSetting(_ context.Context, _ string, value json.RawMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = value
	return nil
}

type fakeModerationLogRepo struct {
	mu   sync.Mutex
	logs []domain.ContentModerationLog
}

func (r *fakeModerationLogRepo) InsertLog(_ context.Context, log domain.ContentModerationLog) (string, time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	log.CreatedAt = time.Now()
	r.logs = append(r.logs, log)
	return "log-id", log.CreatedAt, nil
}

func (r *fakeModerationLogRepo) ListLogs(context.Context, domain.ContentModerationLogFilter, int32, int32) ([]domain.ContentModerationLog, error) {
	return nil, nil
}

func (r *fakeModerationLogRepo) CountLogs(context.Context, domain.ContentModerationLogFilter) (int64, error) {
	return 0, nil
}

func (r *fakeModerationLogRepo) CountFlaggedSince(context.Context, string, time.Time) (int64, error) {
	return 0, nil
}

func (r *fakeModerationLogRepo) snapshot() []domain.ContentModerationLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.ContentModerationLog, len(r.logs))
	copy(out, r.logs)
	return out
}

type fakeRiskEventRepo struct{}

func (fakeRiskEventRepo) InsertEvent(context.Context, domain.RiskEvent) (string, error) {
	return "event-id", nil
}
func (fakeRiskEventRepo) ListEvents(context.Context, domain.RiskEventFilter, int32, int32) ([]domain.RiskEvent, error) {
	return nil, nil
}
func (fakeRiskEventRepo) CountEvents(context.Context, domain.RiskEventFilter) (int64, error) {
	return 0, nil
}
func (fakeRiskEventRepo) ResolveEvent(context.Context, string, string, string, string) (domain.RiskEvent, error) {
	return domain.RiskEvent{}, nil
}

func newTestChecker(t *testing.T, cfg domain.RiskControlConfig) (*riskcontrol.Checker, *fakeModerationLogRepo) {
	t.Helper()
	logRepo := &fakeModerationLogRepo{}
	checker := &riskcontrol.Checker{
		Config: riskcontrol.NewConfigService(newFakeSettingRepo(cfg)),
		Logs:   riskcontrol.NewLogService(logRepo),
		Events: riskcontrol.NewEventService(fakeRiskEventRepo{}),
	}
	return checker, logRepo
}

func openAIChatRequest(text string) *Request {
	body, _ := json.Marshal(map[string]any{
		"model": "gpt-test",
		"messages": []map[string]any{
			{"role": "system", "content": "you are a bot"},
			{"role": "user", "content": text},
		},
	})
	return &Request{
		Envelope:       &RequestEnvelope{ClientBody: body},
		ClientProtocol: domain.ProtocolOpenAIChat,
		CapabilityType: domain.CapabilityChat,
		ModelCode:      "gpt-test",
		Subject:        &coreidentity.Subject{TenantID: "tenant-1", UserID: "user-1", APIKeyID: "key-1"},
	}
}

func waitForLogs(t *testing.T, repo *fakeModerationLogRepo, n int) []domain.ContentModerationLog {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if logs := repo.snapshot(); len(logs) >= n {
			return logs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d async moderation log(s), got %d", n, len(repo.snapshot()))
	return nil
}

// ---- tests ----

func TestContentModerationStep_NilCheckerIsNoop(t *testing.T) {
	step := &ContentModerationStep{}
	req := openAIChatRequest("anything")
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("expected no-op, got error: %v", err)
	}
}

func TestContentModerationStep_DisabledConfigIsNoop(t *testing.T) {
	checker, logRepo := newTestChecker(t, domain.RiskControlConfig{Enabled: false, Mode: domain.RiskControlModeObserve})
	step := &ContentModerationStep{Checker: checker}
	req := openAIChatRequest("badword")
	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("expected no-op when disabled, got error: %v", err)
	}
	if logs := logRepo.snapshot(); len(logs) != 0 {
		t.Fatalf("expected no logs when disabled, got %d", len(logs))
	}
}

func TestContentModerationStep_PreBlockKeywordHit(t *testing.T) {
	checker, logRepo := newTestChecker(t, domain.RiskControlConfig{
		Enabled: true,
		Mode:    domain.RiskControlModePreBlock,
		Keyword: domain.KeywordConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{{Word: "badword", Level: domain.KeywordLevelBlock}},
		},
		BlockStatusCode: 451,
		BlockMessage:    "blocked by policy",
	})
	step := &ContentModerationStep{Checker: checker}
	req := openAIChatRequest("this message contains badword right here")

	err := step.Execute(context.Background(), req)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %#v", err)
	}
	if apiErr.Status != 451 || apiErr.Message != "blocked by policy" {
		t.Fatalf("unexpected api error: %#v", apiErr)
	}

	logs := logRepo.snapshot()
	if len(logs) != 1 || logs[0].Action != domain.RiskControlActionKeywordBlock || !logs[0].Flagged {
		t.Fatalf("unexpected log state: %#v", logs)
	}
}

func TestContentModerationStep_PreBlockAllowsCleanText(t *testing.T) {
	checker, logRepo := newTestChecker(t, domain.RiskControlConfig{
		Enabled: true,
		Mode:    domain.RiskControlModePreBlock,
		Keyword: domain.KeywordConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{{Word: "badword", Level: domain.KeywordLevelBlock}},
		},
	})
	step := &ContentModerationStep{Checker: checker}
	req := openAIChatRequest("perfectly fine message")

	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("expected clean text to pass, got error: %v", err)
	}
	// RecordNonHits defaults to false, so a clean result should not be logged.
	if logs := logRepo.snapshot(); len(logs) != 0 {
		t.Fatalf("expected no log for allowed request, got %d", len(logs))
	}
}

func TestContentModerationStep_PreBlockAPIHit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"flagged":true,"category_scores":{"violence":0.95}}]}`))
	}))
	defer server.Close()

	checker, logRepo := newTestChecker(t, domain.RiskControlConfig{
		Enabled: true,
		Mode:    domain.RiskControlModePreBlock,
		Keyword: domain.KeywordConfig{Enabled: false},
		Provider: domain.RiskControlProviderConfig{
			BaseURL:          server.URL,
			Model:            "test-model",
			APIKeyCiphertext: "plain:test-key",
			TimeoutMs:        2000,
		},
		Thresholds:      map[string]float64{"violence": 0.8},
		SampleRate:      1,
		BlockStatusCode: 451,
		BlockMessage:    "blocked by moderation api",
	})
	step := &ContentModerationStep{Checker: checker}
	req := openAIChatRequest("some violent text")

	err := step.Execute(context.Background(), req)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %#v", err)
	}
	if apiErr.Status != 451 {
		t.Fatalf("unexpected status: %d", apiErr.Status)
	}

	logs := logRepo.snapshot()
	if len(logs) != 1 || logs[0].HighestCategory != "violence" || logs[0].Action != domain.RiskControlActionBlock {
		t.Fatalf("unexpected log state: %#v", logs)
	}
}

func TestContentModerationStep_ObserveModeDoesNotBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"flagged":true,"category_scores":{"violence":0.95}}]}`))
	}))
	defer server.Close()

	checker, logRepo := newTestChecker(t, domain.RiskControlConfig{
		Enabled: true,
		Mode:    domain.RiskControlModeObserve,
		Keyword: domain.KeywordConfig{Enabled: false},
		Provider: domain.RiskControlProviderConfig{
			BaseURL:          server.URL,
			Model:            "test-model",
			APIKeyCiphertext: "plain:test-key",
			TimeoutMs:        2000,
		},
		Thresholds: map[string]float64{"violence": 0.8},
		SampleRate: 1,
	})
	worker := riskcontrol.NewWorker(checker, nil)
	worker.Start(t.Context(), 1)

	step := &ContentModerationStep{Checker: checker, Worker: worker}
	req := openAIChatRequest("some violent text")

	if err := step.Execute(context.Background(), req); err != nil {
		t.Fatalf("observe mode must never block the request, got error: %v", err)
	}

	logs := waitForLogs(t, logRepo, 1)
	if logs[0].Mode != domain.RiskControlModeObserve || logs[0].Action != domain.RiskControlActionBlock {
		t.Fatalf("unexpected async log state: %#v", logs[0])
	}
}

// openAIImageRequest 造一个生图请求（prompt 而非 messages），可选 multipart 传输。
func openAIImageRequest(prompt string) *Request {
	body, _ := json.Marshal(map[string]any{
		"model": "gpt-image-2", "prompt": prompt, "n": 1, "size": "1024x1024",
	})
	return &Request{
		Envelope:       &RequestEnvelope{ClientBody: body},
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		ModelCode:      "gpt-image-2",
		Subject:        &coreidentity.Subject{TenantID: "tenant-1", UserID: "user-1", APIKeyID: "key-1"},
	}
}

func multipartImageEditRequest(t *testing.T, prompt string) *Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("image", "base.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.WriteField("prompt", prompt)
	_ = w.Close()

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(buf.Bytes()))
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	return &Request{
		Envelope: &RequestEnvelope{
			ClientBody: buf.Bytes(), R: httpReq,
			ClientProtocol: domain.ProtocolOpenAIImages,
		},
		ClientProtocol: domain.ProtocolOpenAIImages,
		CapabilityType: domain.CapabilityImage,
		ModelCode:      "gpt-image-2",
		Subject:        &coreidentity.Subject{TenantID: "tenant-1", UserID: "user-1", APIKeyID: "key-1"},
	}
}

// 生图提示词此前完全绕过审核：extractModerationText 只认 messages，而生图请求把用户文本
// 放在 prompt 里，于是每一次生图/修图的审核都静默 no-op。
func TestContentModerationStep_BlocksImagePrompt(t *testing.T) {
	checker, logRepo := newTestChecker(t, domain.RiskControlConfig{
		Enabled: true,
		Mode:    domain.RiskControlModePreBlock,
		Keyword: domain.KeywordConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{{Word: "badword", Level: domain.KeywordLevelBlock}},
		},
		BlockStatusCode: 451,
		BlockMessage:    "blocked by policy",
	})
	step := &ContentModerationStep{Checker: checker}

	err := step.Execute(context.Background(), openAIImageRequest("画一张 badword 的海报"))
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError for image prompt, got %#v", err)
	}
	if apiErr.Status != 451 {
		t.Fatalf("status = %d, want 451", apiErr.Status)
	}
	if logs := logRepo.snapshot(); len(logs) != 1 || !logs[0].Flagged {
		t.Fatalf("expected one flagged log, got %#v", logs)
	}
}

// multipart 传输的 images.edit 同样不能成为绕过口子。
func TestContentModerationStep_BlocksMultipartImageEditPrompt(t *testing.T) {
	checker, _ := newTestChecker(t, domain.RiskControlConfig{
		Enabled: true,
		Mode:    domain.RiskControlModePreBlock,
		Keyword: domain.KeywordConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{{Word: "badword", Level: domain.KeywordLevelBlock}},
		},
		BlockStatusCode: 451,
		BlockMessage:    "blocked by policy",
	})
	step := &ContentModerationStep{Checker: checker}

	err := step.Execute(context.Background(), multipartImageEditRequest(t, "把它改成 badword"))
	if _, ok := err.(*APIError); !ok {
		t.Fatalf("expected *APIError for multipart image edit, got %#v", err)
	}
}

// 干净的生图提示词必须放行，且留下一条未命中的记录。
func TestContentModerationStep_AllowsCleanImagePrompt(t *testing.T) {
	checker, logRepo := newTestChecker(t, domain.RiskControlConfig{
		Enabled: true,
		Mode:    domain.RiskControlModePreBlock,
		Keyword: domain.KeywordConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{{Word: "badword", Level: domain.KeywordLevelBlock}},
		},
		BlockStatusCode: 451,
	})
	step := &ContentModerationStep{Checker: checker}
	if err := step.Execute(context.Background(), openAIImageRequest("一只白色陶瓷杯，暖色调")); err != nil {
		t.Fatalf("clean image prompt must pass, got %v", err)
	}
	for _, log := range logRepo.snapshot() {
		if log.Flagged {
			t.Fatalf("clean prompt must not be flagged: %#v", log)
		}
	}
}
