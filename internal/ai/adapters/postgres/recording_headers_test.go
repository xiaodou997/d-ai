package postgres

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/audit"
)

func TestRecordingHeadersRedaction(t *testing.T) {
	raw := json.RawMessage(`{"Authorization":["secret"],"COOKIE":["session=secret"],"X-Private":["private"],"Content-Type":["application/json"]}`)
	out := redactHeaders(raw, []string{"authorization", "cookie", "x-private"})
	var headers map[string][]string
	if err := json.Unmarshal(out, &headers); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"Authorization", "COOKIE", "X-Private"} {
		if headers[k][0] != "***REDACTED***" {
			t.Fatalf("header %s leaked", k)
		}
	}
	if headers["Content-Type"][0] != "application/json" {
		t.Fatal("ordinary header lost")
	}
	if redactHeaders([]byte("invalid"), nil) != nil {
		t.Fatal("invalid headers retained")
	}
}

func TestRecordingLevelsControlPersistedPayload(t *testing.T) {
	newPayload := func() *audit.Payload {
		return &audit.Payload{
			RequestMessages:     json.RawMessage(`[{"role":"user","content":"secret"}]`),
			RequestParams:       json.RawMessage(`{"temperature":1}`),
			RequestHeaders:      json.RawMessage(`{"Authorization":["secret"],"Content-Type":["application/json"]}`),
			ResponseMessage:     json.RawMessage(`{"content":"secret"}`),
			ResponseHeaders:     json.RawMessage(`{"Set-Cookie":["secret"],"Content-Type":["application/json"]}`),
			MediaRefs:           json.RawMessage(`[{"id":"media-1"}]`),
			InternalErrorDetail: "private error",
			AttemptsDetail:      json.RawMessage(`[{"account":"private"}]`),
		}
	}

	basic := newPayload()
	if err := applyRecordingSettings(basic, audit.RecordingSettings{Level: "basic", SensitiveHeaders: []string{"authorization", "set-cookie"}}); err != nil {
		t.Fatal(err)
	}
	if basic.RequestMessages != nil || basic.RequestHeaders != nil || basic.ResponseMessage != nil || basic.ResponseHeaders != nil || basic.InternalErrorDetail != "" {
		t.Fatalf("basic level retained private payload: %#v", basic)
	}

	headers := newPayload()
	if err := applyRecordingSettings(headers, audit.RecordingSettings{Level: "headers", SensitiveHeaders: []string{"authorization", "set-cookie"}}); err != nil {
		t.Fatal(err)
	}
	if headers.RequestMessages != nil || headers.ResponseMessage != nil || headers.RequestHeaders == nil || headers.ResponseHeaders == nil {
		t.Fatalf("headers level payload = %#v", headers)
	}
	if string(headers.RequestHeaders) == string(newPayload().RequestHeaders) || string(headers.ResponseHeaders) == string(newPayload().ResponseHeaders) {
		t.Fatal("headers level retained unredacted credentials")
	}

	full := newPayload()
	if err := applyRecordingSettings(full, audit.RecordingSettings{Level: "full", SensitiveHeaders: []string{"authorization", "set-cookie"}}); err != nil {
		t.Fatal(err)
	}
	if full.RequestMessages == nil || full.ResponseMessage == nil || full.RequestHeaders == nil || full.ResponseHeaders == nil {
		t.Fatalf("full level dropped payload: %#v", full)
	}
	if string(full.RequestHeaders) == string(newPayload().RequestHeaders) || string(full.ResponseHeaders) == string(newPayload().ResponseHeaders) {
		t.Fatal("full level retained unredacted credentials")
	}
}
