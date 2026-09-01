package serving

import (
	"encoding/json"
	"testing"
)

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
