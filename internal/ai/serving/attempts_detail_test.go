package serving

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBuildAttemptsDetailWithSkippedBeforeTransport(t *testing.T) {
	attempts := []AttemptRecord{{
		RouteID:         "route-1",
		GroupID:         "g1",
		GroupRank:       0,
		TargetPriority:  10,
		Outcome:         ResultServerError,
		HTTPStatus:      http.StatusBadGateway,
		SelectionReason: "cost",
		LatencyMs:       42,
	}}
	skipped := []AttemptRecord{{
		RouteID:         "route-2",
		GroupID:         "g1",
		GroupRank:       0,
		TargetPriority:  50,
		Outcome:         ResultCircuitOpen,
		ErrorMsg:        "circuit_open",
		SelectionReason: "manual_priority",
	}}
	raw := BuildAttemptsDetailWithSkipped(attempts, skipped)
	if len(raw) == 0 {
		t.Fatal("expected attempts detail")
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (1 skipped + 1 real)", len(rows))
	}
	// Skipped entry comes first, carries outcome=circuit_open, skipped=true, no http_status.
	if rows[0]["route_id"] != "route-2" || rows[0]["outcome"] != "circuit_open" || rows[0]["skipped"] != true {
		t.Fatalf("skipped row = %#v", rows[0])
	}
	if rows[0]["priority"] != float64(50) {
		t.Fatalf("skipped priority = %v, want 50", rows[0]["priority"])
	}
	if v, ok := rows[0]["http_status"].(float64); ok && v != 0 {
		t.Fatalf("skipped http_status = %v, want 0/absent", rows[0]["http_status"])
	}
	// Real attempt carries outcome + priority 10 + http status.
	if rows[1]["route_id"] != "route-1" || rows[1]["skipped"] != false {
		t.Fatalf("attempt row = %#v", rows[1])
	}
	if rows[1]["priority"] != float64(10) {
		t.Fatalf("attempt priority = %v, want 10", rows[1]["priority"])
	}
}

func TestBuildAttemptsDetailWithSkippedReturnsNilWhenBothEmpty(t *testing.T) {
	if raw := BuildAttemptsDetailWithSkipped(nil, nil); raw != nil {
		t.Fatalf("expected nil for both-empty, got %s", raw)
	}
}

func TestBuildAttemptsDetailFullMergesPlanningSkippedAndBreakerSkippedBeforeAttempts(t *testing.T) {
	req := &Request{
		Attempts: []AttemptRecord{{
			RouteID:         "route-real",
			GroupID:         "g1",
			GroupRank:       0,
			TargetPriority:  10,
			Outcome:         ResultServerError,
			HTTPStatus:      http.StatusBadGateway,
			SelectionReason: "cost",
			LatencyMs:       42,
		}},
		PlanningSkipped: []AttemptRecord{{
			RouteID:         "route-plan-rej",
			GroupID:         "g1",
			GroupRank:       0,
			TargetPriority:  50,
			Outcome:         ResultRejected,
			SelectionReason: "rejected:credential_unavailable",
			ErrorMsg:        "credential_unavailable: no live credential",
		}},
		SkippedAttempts: []AttemptRecord{{
			RouteID:         "route-open",
			GroupID:         "g2",
			GroupRank:       1,
			TargetPriority:  100,
			Outcome:         ResultCircuitOpen,
			SelectionReason: "cost",
			ErrorMsg:        "circuit_open",
		}},
	}
	raw := BuildAttemptsDetailFull(req)
	if len(raw) == 0 {
		t.Fatal("expected attempts detail")
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (1 planning-skipped + 1 breaker-skipped + 1 real)", len(rows))
	}
	// Order: planning-skipped (rejected) → breaker-skipped (circuit_open) → real attempt.
	if rows[0]["route_id"] != "route-plan-rej" || rows[0]["outcome"] != "rejected" || rows[0]["skipped"] != true {
		t.Fatalf("rows[0] = %#v", rows[0])
	}
	if rows[0]["priority"] != float64(50) {
		t.Fatalf("rows[0] priority = %v, want 50", rows[0]["priority"])
	}
	if rows[1]["route_id"] != "route-open" || rows[1]["outcome"] != "circuit_open" || rows[1]["skipped"] != true {
		t.Fatalf("rows[1] = %#v", rows[1])
	}
	if rows[1]["priority"] != float64(100) {
		t.Fatalf("rows[1] priority = %v, want 100", rows[1]["priority"])
	}
	if rows[2]["route_id"] != "route-real" || rows[2]["outcome"] != "server_error" || rows[2]["skipped"] != false {
		t.Fatalf("rows[2] = %#v", rows[2])
	}
	if rows[2]["priority"] != float64(10) {
		t.Fatalf("rows[2] priority = %v, want 10", rows[2]["priority"])
	}
}

func TestBuildAttemptsDetailFullReturnsNilOnEmptyReq(t *testing.T) {
	if raw := BuildAttemptsDetailFull(nil); raw != nil {
		t.Fatalf("expected nil for nil req, got %s", raw)
	}
	if raw := BuildAttemptsDetailFull(&Request{}); raw != nil {
		t.Fatalf("expected nil for empty req, got %s", raw)
	}
}
func TestBuildAttemptsDetailPersistsGroupPolicyDecision(t *testing.T) {
	raw := BuildAttemptsDetail([]AttemptRecord{{
		RouteID:         "route-1",
		GroupID:         "group-1",
		RoutePolicy:     "stability",
		GroupRank:       1,
		SelectionReason: "stability",
		Outcome:         ResultTimeout,
	}})
	if len(raw) == 0 {
		t.Fatal("expected attempts detail")
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal attempts detail: %v", err)
	}
	if len(rows) != 1 || rows[0]["group_id"] != "group-1" || rows[0]["route_policy"] != "stability" || rows[0]["selection_reason"] != "stability" {
		t.Fatalf("policy fields = %#v", rows)
	}
}
