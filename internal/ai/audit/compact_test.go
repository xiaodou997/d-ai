package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLargeTextAuditKeepsDiagnostics(t *testing.T) {
	body, _ := json.Marshal(strings.Repeat("x", 5_625_609))
	p := &Payload{RequestID: "incident", CapabilityType: "chat", ClientProtocol: "openai_responses", RequestMessages: body,
		ErrorCode: "provider_terminal_error", InternalErrorDetail: "timed out", FailedStep: "execute", AttemptsDetail: []byte(`[{"upstream_request_id":"upstream-1","outcome":"failed"}]`)}
	store := &fakeStore{}
	if !NewWorker(store, nil, WorkerOptions{}).Submit(p) {
		t.Fatal("large text audit rejected")
	}
	if !p.Compaction.Truncated || p.Compaction.OriginalBytes < 5_625_609 || len(p.RequestMessages) != 0 || len(p.AttemptsDetail) == 0 || p.InternalErrorDetail != "timed out" {
		t.Fatalf("compacted=%+v", p)
	}
	before := p.Compaction.OriginalBytes
	CompactTextPayload(p)
	if p.Compaction.OriginalBytes != before {
		t.Fatal("compaction metadata not idempotent")
	}
	media := &Payload{CapabilityType: "video", ClientProtocol: "openai_responses", RequestMessages: body}
	CompactTextPayload(media)
	if media.Compaction.Truncated {
		t.Fatal("media policy changed")
	}
}
