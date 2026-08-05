package routing

import "testing"

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if p.ScopeType != ScopeGlobal {
		t.Fatalf("ScopeType = %s, want %s", p.ScopeType, ScopeGlobal)
	}
	if p.ScopeID != "global" {
		t.Fatalf("ScopeID = %s, want global", p.ScopeID)
	}
	if p.Weights.IsZero() {
		t.Fatal("default weights should not be zero")
	}
}
