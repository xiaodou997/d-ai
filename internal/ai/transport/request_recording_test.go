package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/audit"
	"xiaodou/dai/libs/go/server"
)

type recordingSettingsStoreStub struct {
	value     audit.RecordingSettings
	updated   audit.RecordingSettings
	getErr    error
	updateErr error
}

func (s *recordingSettingsStoreStub) Get(context.Context) (audit.RecordingSettings, error) {
	return s.value, s.getErr
}

func (s *recordingSettingsStoreStub) Update(_ context.Context, value audit.RecordingSettings) (audit.RecordingSettings, error) {
	s.updated = value
	if s.updateErr != nil {
		return audit.RecordingSettings{}, s.updateErr
	}
	s.value = value
	return value, nil
}

func TestRequestRecordingRoutesReadAndUpdateSettings(t *testing.T) {
	store := &recordingSettingsStoreStub{value: audit.DefaultRecordingSettings()}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerRecordingSettings(api, store)

	getRecorder := performRecordingSettingsRequest(router, http.MethodGet, "")
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRecorder.Code, getRecorder.Body.String())
	}
	var current audit.RecordingSettings
	if err := json.NewDecoder(getRecorder.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if current.Level != "basic" || len(current.SensitiveHeaders) == 0 {
		t.Fatalf("current settings = %#v", current)
	}

	putRecorder := performRecordingSettingsRequest(router, http.MethodPut, `{"level":"headers","sensitive_headers":["authorization","x-private"]}`)
	if putRecorder.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRecorder.Code, putRecorder.Body.String())
	}
	if store.updated.Level != "headers" || len(store.updated.SensitiveHeaders) != 2 {
		t.Fatalf("updated settings = %#v", store.updated)
	}
}

func TestRequestRecordingRoutesReportDependencyAndStorageFailures(t *testing.T) {
	tests := []struct {
		name   string
		store  RecordingSettingsStore
		method string
		body   string
		want   int
	}{
		{name: "missing dependency", method: http.MethodGet, want: http.StatusServiceUnavailable},
		{name: "read failure", store: &recordingSettingsStoreStub{getErr: errors.New("database unavailable")}, method: http.MethodGet, want: http.StatusInternalServerError},
		{name: "invalid update", store: &recordingSettingsStoreStub{updateErr: audit.ErrInvalidRecordingSettings}, method: http.MethodPut, body: `{"level":"basic","sensitive_headers":[]}`, want: http.StatusBadRequest},
		{name: "write failure", store: &recordingSettingsStoreStub{updateErr: errors.New("database unavailable")}, method: http.MethodPut, body: `{"level":"basic","sensitive_headers":[]}`, want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, api := server.New(server.Options{Title: "test", Version: "test"})
			registerRecordingSettings(api, tt.store)
			recorder := performRecordingSettingsRequest(router, tt.method, tt.body)
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.want, recorder.Body.String())
			}
		})
	}
}

func performRecordingSettingsRequest(handler http.Handler, method, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/v1/admin/modules/request-recording/config", strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
