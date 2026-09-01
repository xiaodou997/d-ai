package observability

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/serving"
)

func TestBuildTraceIncludesGroupPolicyDecision(t *testing.T) {
	req := &serving.Request{
		StickyHit: true,
		Attempts: []serving.AttemptRecord{
			{
				RouteID:         "route-1",
				GroupID:         "group-private",
				RoutePolicy:     "cost",
				GroupRank:       2,
				SelectionReason: "cost",
				Outcome:         serving.ResultSuccess,
				HTTPStatus:      200,
				LatencyMs:       42,
			},
		},
	}

	header, err := EncodeTraceHeader(BuildTrace(req))
	if err != nil {
		t.Fatalf("EncodeTraceHeader: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		t.Fatalf("decode trace header: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal trace payload: %v", err)
	}
	attempts, ok := payload["attempts"].([]any)
	if !ok || len(attempts) != 1 {
		t.Fatalf("attempts = %#v", payload["attempts"])
	}
	attempt := attempts[0].(map[string]any)
	for key, want := range map[string]any{
		"policy":           "cost",
		"group_rank":       float64(2),
		"selection_reason": "cost",
	} {
		if attempt[key] != want {
			t.Fatalf("attempt[%q] = %#v, want %#v", key, attempt[key], want)
		}
	}
	if _, leaked := attempt["group_id"]; leaked {
		t.Fatal("client trace must not expose internal group id")
	}
	if payload["sticky_hit"] != true {
		t.Fatalf("sticky_hit = %#v", payload["sticky_hit"])
	}
}
