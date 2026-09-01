package serving

import (
	"encoding/json"
	"testing"
)

func TestBuildAttemptsDetailPersistsGroupPolicyDecision(t *testing.T) {
	raw := BuildAttemptsDetail([]AttemptRecord{{
		RouteID:         "route-1",
		GroupID:         "group-1",
		RouteStrategy:   "adaptive",
		RouteObjective:  "stability",
		GroupRank:       1,
		Priority:        20,
		RoutingWeight:   0.5,
		SelectionReason: "adaptive_fallback",
		Outcome:         ResultTimeout,
	}})
	if len(raw) == 0 {
		t.Fatal("expected attempts detail")
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal attempts detail: %v", err)
	}
	if len(rows) != 1 || rows[0]["group_id"] != "group-1" || rows[0]["route_strategy"] != "adaptive" || rows[0]["selection_reason"] != "adaptive_fallback" {
		t.Fatalf("policy fields = %#v", rows)
	}
}
